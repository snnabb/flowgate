package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/flowgate/flowgate/internal/panel/model"
)

// Database wraps the SQLite connection
type Database struct {
	db *sql.DB
}

// New creates a new Database and initializes tables
func New(path string) (*Database, error) {
	sqlDB, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB.SetMaxOpenConns(1) // SQLite single writer
	sqlDB.SetMaxIdleConns(2)

	d := &Database{db: sqlDB}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return d, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

func (d *Database) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT DEFAULT 'user',
		traffic_quota BIGINT DEFAULT 0,
		traffic_used BIGINT DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		api_key TEXT UNIQUE NOT NULL,
		group_name TEXT DEFAULT '',
		status TEXT DEFAULT 'offline',
		ip_addr TEXT DEFAULT '',
		cpu_usage REAL DEFAULT 0,
		mem_usage REAL DEFAULT 0,
		last_seen DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		node_id INTEGER NOT NULL,
		name TEXT DEFAULT '',
		protocol TEXT DEFAULT 'tcp',
		listen_port INTEGER NOT NULL,
		target_addr TEXT NOT NULL,
		target_port INTEGER NOT NULL,
		speed_limit INTEGER DEFAULT 0,
		traffic_in BIGINT DEFAULT 0,
		traffic_out BIGINT DEFAULT 0,
		enabled BOOLEAN DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS traffic_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		rule_id INTEGER NOT NULL,
		node_id INTEGER NOT NULL,
		traffic_in BIGINT DEFAULT 0,
		traffic_out BIGINT DEFAULT 0,
		recorded_at DATETIME NOT NULL,
		FOREIGN KEY (rule_id) REFERENCES rules(id) ON DELETE CASCADE,
		FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_rules_node_id ON rules(node_id);
	CREATE INDEX IF NOT EXISTS idx_traffic_logs_recorded ON traffic_logs(recorded_at);
	CREATE INDEX IF NOT EXISTS idx_traffic_logs_rule ON traffic_logs(rule_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_traffic_logs_rule_node_hour ON traffic_logs(rule_id, node_id, recorded_at);
	`
	_, err := d.db.Exec(schema)
	return err
}

// GenerateAPIKey generates a random API key
func GenerateAPIKey() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ==================== User Operations ====================

// GetUserByUsername retrieves a user by username
func (d *Database) GetUserByUsername(username string) (*model.User, error) {
	u := &model.User{}
	err := d.db.QueryRow(
		"SELECT id, username, password_hash, role, traffic_quota, traffic_used, created_at FROM users WHERE username = ?",
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.TrafficQuota, &u.TrafficUsed, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// CreateUser creates a new user
func (d *Database) CreateUser(username, passwordHash, role string) (*model.User, error) {
	res, err := d.db.Exec(
		"INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)",
		username, passwordHash, role,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &model.User{
		ID:       id,
		Username: username,
		Role:     role,
		CreatedAt: time.Now(),
	}, nil
}

// GetUserCount returns the total number of users
func (d *Database) GetUserCount() (int, error) {
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

// ListUsers returns all users
func (d *Database) ListUsers() ([]model.User, error) {
	rows, err := d.db.Query("SELECT id, username, role, traffic_quota, traffic_used, created_at FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		rows.Scan(&u.ID, &u.Username, &u.Role, &u.TrafficQuota, &u.TrafficUsed, &u.CreatedAt)
		users = append(users, u)
	}
	return users, nil
}

// DeleteUser deletes a user by ID
func (d *Database) DeleteUser(id int64) error {
	_, err := d.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

// UpdateUserPassword updates a user's password hash
func (d *Database) UpdateUserPassword(id int64, passwordHash string) error {
	_, err := d.db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", passwordHash, id)
	return err
}

// ==================== Node Operations ====================

// CreateNode creates a new node
func (d *Database) CreateNode(name, groupName string) (*model.Node, error) {
	apiKey := GenerateAPIKey()
	res, err := d.db.Exec(
		"INSERT INTO nodes (name, api_key, group_name) VALUES (?, ?, ?)",
		name, apiKey, groupName,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &model.Node{
		ID:        id,
		Name:      name,
		APIKey:    apiKey,
		GroupName: groupName,
		Status:    "offline",
		CreatedAt: time.Now(),
	}, nil
}

// GetNodeByAPIKey retrieves a node by its API key
func (d *Database) GetNodeByAPIKey(apiKey string) (*model.Node, error) {
	n := &model.Node{}
	err := d.db.QueryRow(
		"SELECT id, name, api_key, group_name, status, ip_addr, cpu_usage, mem_usage, last_seen, created_at FROM nodes WHERE api_key = ?",
		apiKey,
	).Scan(&n.ID, &n.Name, &n.APIKey, &n.GroupName, &n.Status, &n.IPAddr, &n.CPUUsage, &n.MemUsage, &n.LastSeen, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// GetNodeByID retrieves a node by ID
func (d *Database) GetNodeByID(id int64) (*model.Node, error) {
	n := &model.Node{}
	err := d.db.QueryRow(
		"SELECT id, name, api_key, group_name, status, ip_addr, cpu_usage, mem_usage, last_seen, created_at FROM nodes WHERE id = ?",
		id,
	).Scan(&n.ID, &n.Name, &n.APIKey, &n.GroupName, &n.Status, &n.IPAddr, &n.CPUUsage, &n.MemUsage, &n.LastSeen, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// ListNodes returns all nodes
func (d *Database) ListNodes() ([]model.Node, error) {
	rows, err := d.db.Query(
		"SELECT id, name, api_key, group_name, status, ip_addr, cpu_usage, mem_usage, last_seen, created_at FROM nodes ORDER BY id",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []model.Node
	for rows.Next() {
		var n model.Node
		rows.Scan(&n.ID, &n.Name, &n.APIKey, &n.GroupName, &n.Status, &n.IPAddr, &n.CPUUsage, &n.MemUsage, &n.LastSeen, &n.CreatedAt)
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// UpdateNodeStatus updates a node's online status and stats
func (d *Database) UpdateNodeStatus(id int64, status, ipAddr string, cpu, mem float64) error {
	_, err := d.db.Exec(
		"UPDATE nodes SET status = ?, ip_addr = ?, cpu_usage = ?, mem_usage = ?, last_seen = ? WHERE id = ?",
		status, ipAddr, cpu, mem, time.Now(), id,
	)
	return err
}

// SetNodeOffline sets a node offline
func (d *Database) SetNodeOffline(id int64) error {
	_, err := d.db.Exec("UPDATE nodes SET status = 'offline' WHERE id = ?", id)
	return err
}

// DeleteNode deletes a node and its rules
func (d *Database) DeleteNode(id int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	tx.Exec("DELETE FROM traffic_logs WHERE node_id = ?", id)
	tx.Exec("DELETE FROM rules WHERE node_id = ?", id)
	tx.Exec("DELETE FROM nodes WHERE id = ?", id)
	return tx.Commit()
}

// GetNodeCount returns node statistics
func (d *Database) GetNodeCount() (total, online int, err error) {
	err = d.db.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&total)
	if err != nil {
		return
	}
	err = d.db.QueryRow("SELECT COUNT(*) FROM nodes WHERE status = 'online'").Scan(&online)
	return
}

// ==================== Rule Operations ====================

// CreateRule creates a new forwarding rule
func (d *Database) CreateRule(r *model.CreateRuleRequest) (*model.Rule, error) {
	res, err := d.db.Exec(
		"INSERT INTO rules (node_id, name, protocol, listen_port, target_addr, target_port, speed_limit) VALUES (?, ?, ?, ?, ?, ?, ?)",
		r.NodeID, r.Name, r.Protocol, r.ListenPort, r.TargetAddr, r.TargetPort, r.SpeedLimit,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &model.Rule{
		ID:         id,
		NodeID:     r.NodeID,
		Name:       r.Name,
		Protocol:   r.Protocol,
		ListenPort: r.ListenPort,
		TargetAddr: r.TargetAddr,
		TargetPort: r.TargetPort,
		SpeedLimit: r.SpeedLimit,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}, nil
}

// GetRuleByID retrieves a rule by ID
func (d *Database) GetRuleByID(id int64) (*model.Rule, error) {
	r := &model.Rule{}
	err := d.db.QueryRow(
		"SELECT id, node_id, name, protocol, listen_port, target_addr, target_port, speed_limit, traffic_in, traffic_out, enabled, created_at FROM rules WHERE id = ?",
		id,
	).Scan(&r.ID, &r.NodeID, &r.Name, &r.Protocol, &r.ListenPort, &r.TargetAddr, &r.TargetPort, &r.SpeedLimit, &r.TrafficIn, &r.TrafficOut, &r.Enabled, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// ListRules returns all rules, optionally filtered by node
func (d *Database) ListRules(nodeID int64) ([]model.Rule, error) {
	var rows *sql.Rows
	var err error
	if nodeID > 0 {
		rows, err = d.db.Query(
			"SELECT id, node_id, name, protocol, listen_port, target_addr, target_port, speed_limit, traffic_in, traffic_out, enabled, created_at FROM rules WHERE node_id = ? ORDER BY id",
			nodeID,
		)
	} else {
		rows, err = d.db.Query(
			"SELECT id, node_id, name, protocol, listen_port, target_addr, target_port, speed_limit, traffic_in, traffic_out, enabled, created_at FROM rules ORDER BY id",
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []model.Rule
	for rows.Next() {
		var r model.Rule
		rows.Scan(&r.ID, &r.NodeID, &r.Name, &r.Protocol, &r.ListenPort, &r.TargetAddr, &r.TargetPort, &r.SpeedLimit, &r.TrafficIn, &r.TrafficOut, &r.Enabled, &r.CreatedAt)
		rules = append(rules, r)
	}
	return rules, nil
}

// UpdateRule updates a forwarding rule
func (d *Database) UpdateRule(id int64, r *model.UpdateRuleRequest) error {
	rule, err := d.GetRuleByID(id)
	if err != nil {
		return err
	}

	if r.Name != "" {
		rule.Name = r.Name
	}
	if r.Protocol != "" {
		rule.Protocol = r.Protocol
	}
	if r.ListenPort > 0 {
		rule.ListenPort = r.ListenPort
	}
	if r.TargetAddr != "" {
		rule.TargetAddr = r.TargetAddr
	}
	if r.TargetPort > 0 {
		rule.TargetPort = r.TargetPort
	}
	if r.SpeedLimit >= 0 {
		rule.SpeedLimit = r.SpeedLimit
	}
	if r.Enabled != nil {
		rule.Enabled = *r.Enabled
	}

	_, err = d.db.Exec(
		"UPDATE rules SET name=?, protocol=?, listen_port=?, target_addr=?, target_port=?, speed_limit=?, enabled=? WHERE id=?",
		rule.Name, rule.Protocol, rule.ListenPort, rule.TargetAddr, rule.TargetPort, rule.SpeedLimit, rule.Enabled, id,
	)
	return err
}

// DeleteRule deletes a rule
func (d *Database) DeleteRule(id int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	tx.Exec("DELETE FROM traffic_logs WHERE rule_id = ?", id)
	tx.Exec("DELETE FROM rules WHERE id = ?", id)
	return tx.Commit()
}

// UpdateRuleTraffic updates rule traffic counters
func (d *Database) UpdateRuleTraffic(ruleID int64, trafficIn, trafficOut int64) error {
	_, err := d.db.Exec(
		"UPDATE rules SET traffic_in = traffic_in + ?, traffic_out = traffic_out + ? WHERE id = ?",
		trafficIn, trafficOut, ruleID,
	)
	return err
}

// GetRuleCount returns rule statistics
func (d *Database) GetRuleCount() (total, active int, err error) {
	err = d.db.QueryRow("SELECT COUNT(*) FROM rules").Scan(&total)
	if err != nil {
		return
	}
	err = d.db.QueryRow("SELECT COUNT(*) FROM rules WHERE enabled = 1").Scan(&active)
	return
}

// ==================== Traffic Log Operations ====================

// InsertTrafficLog records hourly traffic data
func (d *Database) InsertTrafficLog(ruleID, nodeID, trafficIn, trafficOut int64) error {
	recordedAt := time.Now().Truncate(time.Hour)
	_, err := d.db.Exec(
		`INSERT INTO traffic_logs (rule_id, node_id, traffic_in, traffic_out, recorded_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(rule_id, node_id, recorded_at)
		 DO UPDATE SET
		 	traffic_in = traffic_in + excluded.traffic_in,
		 	traffic_out = traffic_out + excluded.traffic_out`,
		ruleID, nodeID, trafficIn, trafficOut, recordedAt,
	)
	return err
}

// GetTrafficLogs gets traffic logs for a time range
func (d *Database) GetTrafficLogs(ruleID int64, hours int) ([]model.TrafficLog, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	rows, err := d.db.Query(
		"SELECT id, rule_id, node_id, traffic_in, traffic_out, recorded_at FROM traffic_logs WHERE rule_id = ? AND recorded_at >= ? ORDER BY recorded_at",
		ruleID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []model.TrafficLog
	for rows.Next() {
		var l model.TrafficLog
		rows.Scan(&l.ID, &l.RuleID, &l.NodeID, &l.TrafficIn, &l.TrafficOut, &l.RecordedAt)
		logs = append(logs, l)
	}
	return logs, nil
}

// GetTotalTraffic returns total traffic across all rules
func (d *Database) GetTotalTraffic() (totalIn, totalOut int64, err error) {
	err = d.db.QueryRow("SELECT COALESCE(SUM(traffic_in),0), COALESCE(SUM(traffic_out),0) FROM rules").Scan(&totalIn, &totalOut)
	return
}

// GetDashboardStats returns the dashboard overview stats
func (d *Database) GetDashboardStats() (*model.DashboardStats, error) {
	stats := &model.DashboardStats{}
	var err error

	stats.TotalNodes, stats.OnlineNodes, err = d.GetNodeCount()
	if err != nil {
		return nil, err
	}

	stats.TotalRules, stats.ActiveRules, err = d.GetRuleCount()
	if err != nil {
		return nil, err
	}

	stats.TotalTrafficIn, stats.TotalTrafficOut, err = d.GetTotalTraffic()
	if err != nil {
		return nil, err
	}

	stats.TotalUsers, err = d.GetUserCount()
	if err != nil {
		return nil, err
	}

	return stats, nil
}
