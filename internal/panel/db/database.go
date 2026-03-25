package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/flowgate/flowgate/internal/common"
	"github.com/flowgate/flowgate/internal/panel/model"
)

// Database wraps the SQLite connection
type Database struct {
	db *sql.DB
}

func scanNode(scanner interface {
	Scan(dest ...interface{}) error
}, n *model.Node) error {
	var lastSeen sql.NullTime
	var createdAt time.Time

	if err := scanner.Scan(
		&n.ID, &n.Name, &n.APIKey, &n.GroupName, &n.Status,
		&n.IPAddr, &n.CPUUsage, &n.MemUsage, &n.MemTotal, &lastSeen, &createdAt,
	); err != nil {
		return err
	}

	if lastSeen.Valid {
		n.LastSeen = lastSeen.Time
	}
	n.CreatedAt = createdAt
	return nil
}

func scanNodeGroup(scanner interface {
	Scan(dest ...interface{}) error
}, g *model.NodeGroup) error {
	return scanner.Scan(&g.ID, &g.Name, &g.Description, &g.NodeCount, &g.CreatedAt)
}

const ruleColumns = "id, node_id, name, protocol, listen_port, target_addr, target_port, speed_limit, traffic_limit, traffic_in, traffic_out, enabled, runtime_status, runtime_message, latency_ms, created_at, proxy_protocol, blocked_protos, pool_size, tls_mode, tls_sni, ws_enabled, ws_path, route_mode, entry_group, relay_groups, exit_group, lb_strategy"

func scanRule(scanner interface {
	Scan(dest ...interface{}) error
}, r *model.Rule) error {
	if err := scanner.Scan(
		&r.ID, &r.NodeID, &r.Name, &r.Protocol, &r.ListenPort, &r.TargetAddr,
		&r.TargetPort, &r.SpeedLimit, &r.TrafficLimit, &r.TrafficIn, &r.TrafficOut, &r.Enabled,
		&r.RuntimeStatus, &r.RuntimeMessage, &r.Latency, &r.CreatedAt,
		&r.ProxyProtocol, &r.BlockedProtos, &r.PoolSize, &r.TLSMode, &r.TLSSni, &r.WSEnabled, &r.WSPath,
		&r.RouteMode, &r.EntryGroup, &r.RelayGroups, &r.ExitGroup, &r.LBStrategy,
	); err != nil {
		return err
	}
	r.RouteMode = common.NormalizedRouteMode(r.RouteMode)
	r.LBStrategy = common.NormalizedLoadBalanceStrategy(r.LBStrategy)
	if r.TLSMode == "" {
		r.TLSMode = "none"
	}
	if r.WSPath == "" {
		r.WSPath = "/ws"
	}
	return nil
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
		mem_total REAL DEFAULT 0,
		last_seen DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS node_groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		description TEXT DEFAULT '',
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
		runtime_status TEXT DEFAULT 'pending',
		runtime_message TEXT DEFAULT '',
		latency_ms REAL DEFAULT -1,
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

	CREATE TABLE IF NOT EXISTS panel_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		category TEXT NOT NULL,
		title TEXT NOT NULL,
		details TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_rules_node_id ON rules(node_id);
	CREATE INDEX IF NOT EXISTS idx_nodes_group_name ON nodes(group_name);
	CREATE INDEX IF NOT EXISTS idx_node_groups_name ON node_groups(name);
	CREATE INDEX IF NOT EXISTS idx_traffic_logs_recorded ON traffic_logs(recorded_at);
	CREATE INDEX IF NOT EXISTS idx_traffic_logs_rule ON traffic_logs(rule_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_traffic_logs_rule_node_hour ON traffic_logs(rule_id, node_id, recorded_at);
	CREATE INDEX IF NOT EXISTS idx_panel_events_created_at ON panel_events(created_at DESC);
	`
	if _, err := d.db.Exec(schema); err != nil {
		return err
	}

	if err := d.ensureColumn("nodes", "mem_total", "REAL DEFAULT 0"); err != nil {
		return err
	}
	if err := d.ensureColumn("rules", "runtime_status", "TEXT DEFAULT 'pending'"); err != nil {
		return err
	}
	if err := d.ensureColumn("rules", "runtime_message", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := d.ensureColumn("rules", "latency_ms", "REAL DEFAULT -1"); err != nil {
		return err
	}
	if err := d.ensureColumn("rules", "traffic_limit", "BIGINT DEFAULT 0"); err != nil {
		return err
	}

	// Phase 1: Tunnel engine columns
	if err := d.ensureColumn("rules", "proxy_protocol", "INTEGER DEFAULT 0"); err != nil {
		return err
	}
	if err := d.ensureColumn("rules", "blocked_protos", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := d.ensureColumn("rules", "pool_size", "INTEGER DEFAULT 0"); err != nil {
		return err
	}
	if err := d.ensureColumn("rules", "tls_mode", "TEXT DEFAULT 'none'"); err != nil {
		return err
	}
	if err := d.ensureColumn("rules", "tls_sni", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := d.ensureColumn("rules", "ws_enabled", "BOOLEAN DEFAULT 0"); err != nil {
		return err
	}
	if err := d.ensureColumn("rules", "ws_path", "TEXT DEFAULT '/ws'"); err != nil {
		return err
	}

	// Phase 2: Route skeleton columns
	if err := d.ensureColumn("rules", "route_mode", "TEXT DEFAULT 'direct'"); err != nil {
		return err
	}
	if err := d.ensureColumn("rules", "entry_group", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := d.ensureColumn("rules", "relay_groups", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := d.ensureColumn("rules", "exit_group", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	return d.ensureColumn("rules", "lb_strategy", "TEXT DEFAULT 'none'")
}

// GenerateAPIKey generates a random API key
func GenerateAPIKey() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (d *Database) ensureColumn(tableName, columnName, definition string) error {
	rows, err := d.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return err
		}
		if name == columnName {
			return nil
		}
	}

	_, err = d.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, definition))
	return err
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

// CreateEvent stores an event for the panel activity feed.
func (d *Database) CreateEvent(category, title, details string) error {
	_, err := d.db.Exec(
		"INSERT INTO panel_events (category, title, details) VALUES (?, ?, ?)",
		category, title, details,
	)
	return err
}

// ListRecentEvents returns the newest panel events first.
func (d *Database) ListRecentEvents(limit int) ([]model.PanelEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := d.db.Query(
		"SELECT id, category, title, details, created_at FROM panel_events ORDER BY id DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []model.PanelEvent
	for rows.Next() {
		var event model.PanelEvent
		if err := rows.Scan(&event.ID, &event.Category, &event.Title, &event.Details, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
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
	err := scanNode(d.db.QueryRow(
		"SELECT id, name, api_key, group_name, status, ip_addr, cpu_usage, mem_usage, mem_total, last_seen, created_at FROM nodes WHERE api_key = ?",
		apiKey,
	), n)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// GetNodeByID retrieves a node by ID
func (d *Database) GetNodeByID(id int64) (*model.Node, error) {
	n := &model.Node{}
	err := scanNode(d.db.QueryRow(
		"SELECT id, name, api_key, group_name, status, ip_addr, cpu_usage, mem_usage, mem_total, last_seen, created_at FROM nodes WHERE id = ?",
		id,
	), n)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// ListNodes returns all nodes
func (d *Database) ListNodes() ([]model.Node, error) {
	rows, err := d.db.Query(
		"SELECT id, name, api_key, group_name, status, ip_addr, cpu_usage, mem_usage, mem_total, last_seen, created_at FROM nodes ORDER BY id",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []model.Node
	for rows.Next() {
		var n model.Node
		if err := scanNode(rows, &n); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// UpdateNodeStatus updates a node's online status and stats
func (d *Database) UpdateNodeStatus(id int64, status, ipAddr string, cpu, memUsage, memTotal float64) error {
	_, err := d.db.Exec(
		"UPDATE nodes SET status = ?, ip_addr = ?, cpu_usage = ?, mem_usage = ?, mem_total = ?, last_seen = ? WHERE id = ?",
		status, ipAddr, cpu, memUsage, memTotal, time.Now(), id,
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

	if _, err := tx.Exec("DELETE FROM traffic_logs WHERE node_id = ?", id); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete traffic_logs: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM rules WHERE node_id = ?", id); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete rules: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM nodes WHERE id = ?", id); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete node: %w", err)
	}

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

// CreateNodeGroup creates a new reusable node group.
func (d *Database) CreateNodeGroup(name, description string) (*model.NodeGroup, error) {
	res, err := d.db.Exec(
		"INSERT INTO node_groups (name, description) VALUES (?, ?)",
		name, description,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &model.NodeGroup{
		ID:          id,
		Name:        name,
		Description: description,
		NodeCount:   0,
		CreatedAt:   time.Now(),
	}, nil
}

// GetNodeGroupByID retrieves a node group by ID with a live node count.
func (d *Database) GetNodeGroupByID(id int64) (*model.NodeGroup, error) {
	group := &model.NodeGroup{}
	err := scanNodeGroup(d.db.QueryRow(
		`SELECT ng.id, ng.name, ng.description,
		        (SELECT COUNT(*) FROM nodes WHERE group_name = ng.name) AS node_count,
		        ng.created_at
		   FROM node_groups ng
		  WHERE ng.id = ?`,
		id,
	), group)
	if err != nil {
		return nil, err
	}
	return group, nil
}

// ListNodeGroups returns all configured node groups with current node counts.
func (d *Database) ListNodeGroups() ([]model.NodeGroup, error) {
	rows, err := d.db.Query(
		`SELECT ng.id, ng.name, ng.description, COUNT(n.id) AS node_count, ng.created_at
		   FROM node_groups ng
		   LEFT JOIN nodes n ON n.group_name = ng.name
		  GROUP BY ng.id, ng.name, ng.description, ng.created_at
		  ORDER BY ng.id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []model.NodeGroup
	for rows.Next() {
		var group model.NodeGroup
		if err := scanNodeGroup(rows, &group); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, nil
}

// DeleteNodeGroup deletes a node group after ensuring it is no longer used by nodes.
func (d *Database) DeleteNodeGroup(id int64) error {
	group, err := d.GetNodeGroupByID(id)
	if err != nil {
		return err
	}
	if group.NodeCount > 0 {
		return fmt.Errorf("node group %s is still used by %d node(s)", group.Name, group.NodeCount)
	}
	_, err = d.db.Exec("DELETE FROM node_groups WHERE id = ?", id)
	return err
}

// ==================== Rule Operations ====================

// CreateRule creates a new forwarding rule
func (d *Database) CreateRule(r *model.CreateRuleRequest) (*model.Rule, error) {
	tlsMode := r.TLSMode
	if tlsMode == "" {
		tlsMode = "none"
	}
	wsPath := r.WSPath
	if wsPath == "" {
		wsPath = "/ws"
	}
	routeMode := common.NormalizedRouteMode(r.RouteMode)
	lbStrategy := common.NormalizedLoadBalanceStrategy(r.LBStrategy)

	res, err := d.db.Exec(
		"INSERT INTO rules (node_id, name, protocol, listen_port, target_addr, target_port, speed_limit, traffic_limit, proxy_protocol, blocked_protos, pool_size, tls_mode, tls_sni, ws_enabled, ws_path, route_mode, entry_group, relay_groups, exit_group, lb_strategy) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		r.NodeID, r.Name, r.Protocol, r.ListenPort, r.TargetAddr, r.TargetPort, r.SpeedLimit, r.TrafficLimit,
		r.ProxyProtocol, r.BlockedProtos, r.PoolSize, tlsMode, r.TLSSni, r.WSEnabled, wsPath,
		routeMode, r.EntryGroup, r.RelayGroups, r.ExitGroup, lbStrategy,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &model.Rule{
		ID:             id,
		NodeID:         r.NodeID,
		Name:           r.Name,
		Protocol:       r.Protocol,
		ListenPort:     r.ListenPort,
		TargetAddr:     r.TargetAddr,
		TargetPort:     r.TargetPort,
		SpeedLimit:     r.SpeedLimit,
		TrafficLimit:   r.TrafficLimit,
		Enabled:        true,
		RuntimeStatus:  "pending",
		RuntimeMessage: "",
		CreatedAt:      time.Now(),
		ProxyProtocol:  r.ProxyProtocol,
		BlockedProtos:  r.BlockedProtos,
		PoolSize:       r.PoolSize,
		TLSMode:        tlsMode,
		TLSSni:         r.TLSSni,
		WSEnabled:      r.WSEnabled,
		WSPath:         wsPath,
		RouteMode:      routeMode,
		EntryGroup:     r.EntryGroup,
		RelayGroups:    r.RelayGroups,
		ExitGroup:      r.ExitGroup,
		LBStrategy:     lbStrategy,
	}, nil
}

// GetRuleByID retrieves a rule by ID
func (d *Database) GetRuleByID(id int64) (*model.Rule, error) {
	r := &model.Rule{}
	err := scanRule(d.db.QueryRow(
		"SELECT "+ruleColumns+" FROM rules WHERE id = ?",
		id,
	), r)
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
			"SELECT "+ruleColumns+" FROM rules WHERE node_id = ? ORDER BY id",
			nodeID,
		)
	} else {
		rows, err = d.db.Query(
			"SELECT "+ruleColumns+" FROM rules ORDER BY id",
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []model.Rule
	for rows.Next() {
		var r model.Rule
		if err := scanRule(rows, &r); err != nil {
			return nil, err
		}
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
	if r.TrafficLimit != nil {
		rule.TrafficLimit = *r.TrafficLimit
	}
	if r.Enabled != nil {
		rule.Enabled = *r.Enabled
	}
	if r.ProxyProtocol != nil {
		rule.ProxyProtocol = *r.ProxyProtocol
	}
	if r.BlockedProtos != nil {
		rule.BlockedProtos = *r.BlockedProtos
	}
	if r.PoolSize != nil {
		rule.PoolSize = *r.PoolSize
	}
	if r.TLSMode != nil {
		rule.TLSMode = *r.TLSMode
	}
	if r.TLSSni != nil {
		rule.TLSSni = *r.TLSSni
	}
	if r.WSEnabled != nil {
		rule.WSEnabled = *r.WSEnabled
	}
	if r.WSPath != nil {
		rule.WSPath = *r.WSPath
	}
	if r.RouteMode != nil {
		rule.RouteMode = common.NormalizedRouteMode(*r.RouteMode)
	}
	if r.EntryGroup != nil {
		rule.EntryGroup = *r.EntryGroup
	}
	if r.RelayGroups != nil {
		rule.RelayGroups = *r.RelayGroups
	}
	if r.ExitGroup != nil {
		rule.ExitGroup = *r.ExitGroup
	}
	if r.LBStrategy != nil {
		rule.LBStrategy = common.NormalizedLoadBalanceStrategy(*r.LBStrategy)
	}

	_, err = d.db.Exec(
		"UPDATE rules SET name=?, protocol=?, listen_port=?, target_addr=?, target_port=?, speed_limit=?, traffic_limit=?, enabled=?, proxy_protocol=?, blocked_protos=?, pool_size=?, tls_mode=?, tls_sni=?, ws_enabled=?, ws_path=?, route_mode=?, entry_group=?, relay_groups=?, exit_group=?, lb_strategy=? WHERE id=?",
		rule.Name, rule.Protocol, rule.ListenPort, rule.TargetAddr, rule.TargetPort, rule.SpeedLimit, rule.TrafficLimit, rule.Enabled,
		rule.ProxyProtocol, rule.BlockedProtos, rule.PoolSize, rule.TLSMode, rule.TLSSni, rule.WSEnabled, rule.WSPath,
		rule.RouteMode, rule.EntryGroup, rule.RelayGroups, rule.ExitGroup, rule.LBStrategy, id,
	)
	return err
}

// UpdateRuleRuntimeStatus updates the runtime status shown in the panel.
func (d *Database) UpdateRuleRuntimeStatus(id int64, status, message string) error {
	_, err := d.db.Exec(
		"UPDATE rules SET runtime_status = ?, runtime_message = ? WHERE id = ?",
		status, message, id,
	)
	return err
}

// UpdateRuleLatency updates the measured latency for a rule.
func (d *Database) UpdateRuleLatency(id int64, latencyMs float64) error {
	_, err := d.db.Exec("UPDATE rules SET latency_ms = ? WHERE id = ?", latencyMs, id)
	return err
}

// UpdateNodeRuleStatuses updates all enabled rule runtime states for a node.
func (d *Database) UpdateNodeRuleStatuses(nodeID int64, status, message string) error {
	_, err := d.db.Exec(
		"UPDATE rules SET runtime_status = ?, runtime_message = ? WHERE node_id = ? AND enabled = 1",
		status, message, nodeID,
	)
	return err
}

// DeleteRule deletes a rule
func (d *Database) DeleteRule(id int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec("DELETE FROM traffic_logs WHERE rule_id = ?", id); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete traffic_logs: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM rules WHERE id = ?", id); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete rule: %w", err)
	}

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

// ResetRuleTraffic resets traffic counters for a rule
func (d *Database) ResetRuleTraffic(ruleID int64) error {
	_, err := d.db.Exec("UPDATE rules SET traffic_in = 0, traffic_out = 0 WHERE id = ?", ruleID)
	return err
}

// CheckTrafficLimitExceeded checks if a rule has exceeded its traffic limit.
// Returns true if exceeded. Always returns false when limit is 0 (unlimited).
func (d *Database) CheckTrafficLimitExceeded(ruleID int64) (bool, error) {
	var trafficIn, trafficOut, trafficLimit int64
	err := d.db.QueryRow(
		"SELECT traffic_in, traffic_out, traffic_limit FROM rules WHERE id = ?", ruleID,
	).Scan(&trafficIn, &trafficOut, &trafficLimit)
	if err != nil {
		return false, err
	}
	if trafficLimit <= 0 {
		return false, nil
	}
	return (trafficIn + trafficOut) >= trafficLimit, nil
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
