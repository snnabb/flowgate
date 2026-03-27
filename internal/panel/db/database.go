package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/flowgate/flowgate/internal/common"
	"github.com/flowgate/flowgate/internal/panel/model"
)

// Database wraps the SQLite connection
type Database struct {
	db *sql.DB
}

func scanUser(scanner interface {
	Scan(dest ...interface{}) error
}, u *model.User) error {
	var expiresAt sql.NullTime

	if err := scanner.Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Enabled, &u.ParentID,
		&u.TrafficQuota, &u.TrafficUsed, &u.Ratio, &expiresAt, &u.MaxRules, &u.BandwidthLimit, &u.CreatedAt,
	); err != nil {
		return err
	}

	if u.Ratio <= 0 {
		u.Ratio = 1
	}
	if expiresAt.Valid {
		t := expiresAt.Time
		u.ExpiresAt = &t
	} else {
		u.ExpiresAt = nil
	}
	return nil
}

func scanUserNodeAccess(scanner interface {
	Scan(dest ...interface{}) error
}, access *model.UserNodeAccess) error {
	return scanner.Scan(
		&access.ID,
		&access.UserID,
		&access.NodeID,
		&access.NodeName,
		&access.TrafficQuota,
		&access.TrafficUsed,
		&access.BandwidthLimit,
		&access.CreatedAt,
	)
}

func scanNode(scanner interface {
	Scan(dest ...interface{}) error
}, n *model.Node) error {
	var lastSeen sql.NullTime
	var createdAt time.Time

	if err := scanner.Scan(
		&n.ID, &n.OwnerUserID, &n.Name, &n.APIKey, &n.GroupName, &n.Status,
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

const userColumns = "id, username, password_hash, role, enabled, parent_id, traffic_quota, traffic_used, ratio, expires_at, max_rules, bandwidth_limit, created_at"
const ruleColumns = "id, owner_user_id, node_id, name, protocol, listen_port, target_addr, target_port, speed_limit, traffic_limit, traffic_in, traffic_out, enabled, runtime_status, runtime_message, latency_ms, created_at, proxy_protocol, blocked_protos, pool_size, tls_mode, tls_sni, ws_enabled, ws_path, route_mode, route_hops, entry_group, relay_groups, exit_group, lb_strategy, parent_rule_id, chain_type, sni_hosts"

func scanRule(scanner interface {
	Scan(dest ...interface{}) error
}, r *model.Rule) error {
	if err := scanner.Scan(
		&r.ID, &r.OwnerUserID, &r.NodeID, &r.Name, &r.Protocol, &r.ListenPort, &r.TargetAddr,
		&r.TargetPort, &r.SpeedLimit, &r.TrafficLimit, &r.TrafficIn, &r.TrafficOut, &r.Enabled,
		&r.RuntimeStatus, &r.RuntimeMessage, &r.Latency, &r.CreatedAt,
		&r.ProxyProtocol, &r.BlockedProtos, &r.PoolSize, &r.TLSMode, &r.TLSSni, &r.WSEnabled, &r.WSPath,
		&r.RouteMode, &r.RouteHops, &r.EntryGroup, &r.RelayGroups, &r.ExitGroup, &r.LBStrategy,
		&r.ParentRuleID, &r.ChainType, &r.SNIHosts,
	); err != nil {
		return err
	}
	r.RouteMode = common.NormalizedRouteMode(r.RouteMode)
	if r.RouteHops == "" {
		r.RouteHops = "[]"
	}
	r.LBStrategy = common.NormalizedLoadBalanceStrategy(r.LBStrategy)
	if r.TLSMode == "" {
		r.TLSMode = "none"
	}
	if r.WSPath == "" {
		r.WSPath = "/ws"
	}
	if r.ChainType == "" {
		r.ChainType = "custom"
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
		enabled BOOLEAN DEFAULT 1,
		parent_id INTEGER DEFAULT 0,
		traffic_quota BIGINT DEFAULT 0,
		traffic_used BIGINT DEFAULT 0,
		ratio REAL DEFAULT 1,
		expires_at DATETIME,
		max_rules INTEGER DEFAULT 0,
		bandwidth_limit INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		owner_user_id INTEGER DEFAULT 0,
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
		owner_user_id INTEGER DEFAULT 0,
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

	CREATE TABLE IF NOT EXISTS user_node_access (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		node_id INTEGER NOT NULL,
		traffic_quota BIGINT DEFAULT 0,
		traffic_used BIGINT DEFAULT 0,
		bandwidth_limit INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, node_id),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_rules_node_id ON rules(node_id);
	CREATE INDEX IF NOT EXISTS idx_nodes_group_name ON nodes(group_name);
	CREATE INDEX IF NOT EXISTS idx_node_groups_name ON node_groups(name);
	CREATE INDEX IF NOT EXISTS idx_user_node_access_user_id ON user_node_access(user_id);
	CREATE INDEX IF NOT EXISTS idx_user_node_access_node_id ON user_node_access(node_id);
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
	if err := d.ensureColumn("users", "parent_id", "INTEGER DEFAULT 0"); err != nil {
		return err
	}
	if err := d.ensureColumn("users", "enabled", "BOOLEAN DEFAULT 1"); err != nil {
		return err
	}
	if err := d.ensureColumn("users", "ratio", "REAL DEFAULT 1"); err != nil {
		return err
	}
	if err := d.ensureColumn("users", "expires_at", "DATETIME"); err != nil {
		return err
	}
	if err := d.ensureColumn("users", "max_rules", "INTEGER DEFAULT 0"); err != nil {
		return err
	}
	if err := d.ensureColumn("users", "bandwidth_limit", "INTEGER DEFAULT 0"); err != nil {
		return err
	}
	if err := d.ensureColumn("nodes", "owner_user_id", "INTEGER DEFAULT 0"); err != nil {
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
	if err := d.ensureColumn("rules", "owner_user_id", "INTEGER DEFAULT 0"); err != nil {
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
	if err := d.ensureColumn("rules", "route_hops", "TEXT DEFAULT '[]'"); err != nil {
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
	if err := d.ensureColumn("rules", "lb_strategy", "TEXT DEFAULT 'none'"); err != nil {
		return err
	}

	// Phase 2: Managed chain columns
	if err := d.ensureColumn("rules", "parent_rule_id", "INTEGER DEFAULT 0"); err != nil {
		return err
	}
	if err := d.ensureColumn("rules", "chain_type", "TEXT DEFAULT 'custom'"); err != nil {
		return err
	}

	// Port multiplexing
	return d.ensureColumn("rules", "sni_hosts", "TEXT DEFAULT ''")
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

func visibleUserIDsClause(actor *model.User) (string, []interface{}, bool) {
	if actor == nil || actor.Role == "admin" {
		return "", nil, true
	}
	return "id = ?", []interface{}{actor.ID}, false
}

func ownerIDsClause(column string, ids []int64) (string, []interface{}) {
	if len(ids) == 0 {
		return "1 = 0", nil
	}

	placeholders := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ",")), args
}

// ==================== User Operations ====================

// GetUserByUsername retrieves a user by username
func (d *Database) GetUserByUsername(username string) (*model.User, error) {
	u := &model.User{}
	err := scanUser(d.db.QueryRow(
		"SELECT "+userColumns+" FROM users WHERE username = ?",
		username,
	), u)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// GetUserByID retrieves a user by ID.
func (d *Database) GetUserByID(id int64) (*model.User, error) {
	u := &model.User{}
	err := scanUser(d.db.QueryRow(
		"SELECT "+userColumns+" FROM users WHERE id = ?",
		id,
	), u)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// CreateUser creates a new user
func (d *Database) CreateUser(username, passwordHash, role string) (*model.User, error) {
	return d.CreateUserWithOptions(&model.CreateUserRequest{
		Username: username,
		Role:     role,
	}, passwordHash)
}

// CreateUserWithOptions creates a user with Phase 3 metadata.
func (d *Database) CreateUserWithOptions(req *model.CreateUserRequest, passwordHash string) (*model.User, error) {
	if req == nil {
		return nil, fmt.Errorf("nil create user request")
	}

	role := req.Role
	if role == "" {
		role = "user"
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	ratio := req.Ratio
	if ratio <= 0 {
		ratio = 1
	}
	var parentID int64
	if req.ParentID != nil {
		parentID = *req.ParentID
	}

	res, err := d.db.Exec(
		"INSERT INTO users (username, password_hash, role, enabled, parent_id, traffic_quota, traffic_used, ratio, expires_at, max_rules, bandwidth_limit) VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?)",
		req.Username, passwordHash, role, enabled, parentID, req.TrafficQuota, ratio, req.ExpiresAt, req.MaxRules, req.BandwidthLimit,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &model.User{
		ID:             id,
		Username:       req.Username,
		Role:           role,
		Enabled:        enabled,
		ParentID:       parentID,
		TrafficQuota:   req.TrafficQuota,
		TrafficUsed:    0,
		Ratio:          ratio,
		ExpiresAt:      req.ExpiresAt,
		MaxRules:       req.MaxRules,
		BandwidthLimit: req.BandwidthLimit,
		CreatedAt:      time.Now(),
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
	rows, err := d.db.Query("SELECT " + userColumns + " FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		if err := scanUser(rows, &u); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// ListUsersVisibleTo returns users visible to the actor based on role hierarchy.
func (d *Database) ListUsersVisibleTo(actor *model.User) ([]model.User, error) {
	where, args, all := visibleUserIDsClause(actor)
	query := "SELECT " + userColumns + " FROM users"
	if !all {
		query += " WHERE " + where
	}
	query += " ORDER BY id"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		if err := scanUser(rows, &u); err != nil {
			return nil, err
		}
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

// UpdateUser updates editable user account fields.
func (d *Database) UpdateUser(id int64, req *model.UpdateUserRequest) error {
	if req == nil {
		return nil
	}

	parts := make([]string, 0, 1)
	args := make([]interface{}, 0, 2)
	if req.Enabled != nil {
		parts = append(parts, "enabled = ?")
		args = append(args, *req.Enabled)
	}
	if len(parts) == 0 {
		return nil
	}

	args = append(args, id)
	_, err := d.db.Exec("UPDATE users SET "+strings.Join(parts, ", ")+" WHERE id = ?", args...)
	return err
}

// ListUserNodeAccess returns all node assignments for a user.
func (d *Database) ListUserNodeAccess(userID int64) ([]model.UserNodeAccess, error) {
	rows, err := d.db.Query(
		`SELECT una.id, una.user_id, una.node_id, n.name, una.traffic_quota, una.traffic_used, una.bandwidth_limit, una.created_at
		   FROM user_node_access una
		   JOIN nodes n ON n.id = una.node_id
		  WHERE una.user_id = ?
		  ORDER BY una.id`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var access []model.UserNodeAccess
	for rows.Next() {
		var item model.UserNodeAccess
		if err := scanUserNodeAccess(rows, &item); err != nil {
			return nil, err
		}
		access = append(access, item)
	}
	return access, nil
}

// GetUserNodeAccess returns a single node assignment for a user.
func (d *Database) GetUserNodeAccess(userID, nodeID int64) (*model.UserNodeAccess, error) {
	item := &model.UserNodeAccess{}
	err := scanUserNodeAccess(d.db.QueryRow(
		`SELECT una.id, una.user_id, una.node_id, n.name, una.traffic_quota, una.traffic_used, una.bandwidth_limit, una.created_at
		   FROM user_node_access una
		   JOIN nodes n ON n.id = una.node_id
		  WHERE una.user_id = ? AND una.node_id = ?`,
		userID, nodeID,
	), item)
	if err != nil {
		return nil, err
	}
	return item, nil
}

// ReplaceUserNodeAccess replaces the complete node assignment set for a user.
func (d *Database) ReplaceUserNodeAccess(userID int64, access []model.UserNodeAccessInput) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}

	existingUsed := make(map[int64]int64, len(access))
	rows, err := tx.Query("SELECT node_id, traffic_used FROM user_node_access WHERE user_id = ?", userID)
	if err != nil {
		tx.Rollback()
		return err
	}
	for rows.Next() {
		var nodeID, trafficUsed int64
		if err := rows.Scan(&nodeID, &trafficUsed); err != nil {
			rows.Close()
			tx.Rollback()
			return err
		}
		existingUsed[nodeID] = trafficUsed
	}
	rows.Close()

	if _, err := tx.Exec("DELETE FROM user_node_access WHERE user_id = ?", userID); err != nil {
		tx.Rollback()
		return err
	}

	for _, item := range access {
		if item.NodeID <= 0 {
			tx.Rollback()
			return fmt.Errorf("invalid node id")
		}
		var nodeExists int
		if err := tx.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = ?", item.NodeID).Scan(&nodeExists); err != nil {
			tx.Rollback()
			return err
		}
		if nodeExists == 0 {
			tx.Rollback()
			return fmt.Errorf("node %d not found", item.NodeID)
		}
		if _, err := tx.Exec(
			"INSERT INTO user_node_access (user_id, node_id, traffic_quota, traffic_used, bandwidth_limit) VALUES (?, ?, ?, ?, ?)",
			userID, item.NodeID, item.TrafficQuota, existingUsed[item.NodeID], item.BandwidthLimit,
		); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
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
	return d.CreateNodeWithOwner(&model.CreateNodeRequest{
		Name:      name,
		GroupName: groupName,
	}, 0)
}

// CreateNodeWithOwner creates a new node owned by a user.
func (d *Database) CreateNodeWithOwner(req *model.CreateNodeRequest, ownerUserID int64) (*model.Node, error) {
	if req == nil {
		return nil, fmt.Errorf("nil create node request")
	}

	apiKey := GenerateAPIKey()
	res, err := d.db.Exec(
		"INSERT INTO nodes (owner_user_id, name, api_key, group_name) VALUES (?, ?, ?, ?)",
		ownerUserID, req.Name, apiKey, req.GroupName,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &model.Node{
		ID:          id,
		OwnerUserID: ownerUserID,
		Name:        req.Name,
		APIKey:      apiKey,
		GroupName:   req.GroupName,
		Status:      "offline",
		CreatedAt:   time.Now(),
	}, nil
}

// GetNodeByAPIKey retrieves a node by its API key
func (d *Database) GetNodeByAPIKey(apiKey string) (*model.Node, error) {
	n := &model.Node{}
	err := scanNode(d.db.QueryRow(
		"SELECT id, owner_user_id, name, api_key, group_name, status, ip_addr, cpu_usage, mem_usage, mem_total, last_seen, created_at FROM nodes WHERE api_key = ?",
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
		"SELECT id, owner_user_id, name, api_key, group_name, status, ip_addr, cpu_usage, mem_usage, mem_total, last_seen, created_at FROM nodes WHERE id = ?",
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
		"SELECT id, owner_user_id, name, api_key, group_name, status, ip_addr, cpu_usage, mem_usage, mem_total, last_seen, created_at FROM nodes ORDER BY id",
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

// ListNodesVisibleTo returns nodes visible to the actor based on ownership.
func (d *Database) ListNodesVisibleTo(actor *model.User) ([]model.Node, error) {
	if actor == nil || actor.Role == "admin" {
		return d.ListNodes()
	}

	rows, err := d.db.Query(
		`SELECT n.id, n.owner_user_id, n.name, n.api_key, n.group_name, n.status, n.ip_addr, n.cpu_usage, n.mem_usage, n.mem_total, n.last_seen, n.created_at
		   FROM nodes n
		   JOIN user_node_access una ON una.node_id = n.id
		  WHERE una.user_id = ?
		  ORDER BY n.id`,
		actor.ID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []model.Node
	for rows.Next() {
		var node model.Node
		if err := scanNode(rows, &node); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
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
	if _, err := tx.Exec("DELETE FROM user_node_access WHERE node_id = ?", id); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete user_node_access: %w", err)
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

// GetNodeCountVisibleTo returns node counts for the actor scope.
func (d *Database) GetNodeCountVisibleTo(actor *model.User) (total, online int, err error) {
	if actor == nil || actor.Role == "admin" {
		return d.GetNodeCount()
	}

	err = d.db.QueryRow(
		"SELECT COUNT(*) FROM user_node_access WHERE user_id = ?",
		actor.ID,
	).Scan(&total)
	if err != nil {
		return
	}
	err = d.db.QueryRow(
		`SELECT COUNT(*)
		   FROM nodes n
		   JOIN user_node_access una ON una.node_id = n.id
		  WHERE una.user_id = ? AND n.status = 'online'`,
		actor.ID,
	).Scan(&online)
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
	return d.CreateRuleWithOwner(r, 0)
}

// CreateRuleWithOwner creates a new forwarding rule owned by a user.
func (d *Database) CreateRuleWithOwner(r *model.CreateRuleRequest, ownerUserID int64) (*model.Rule, error) {
	if r == nil {
		return nil, fmt.Errorf("nil create rule request")
	}

	tlsMode := r.TLSMode
	if tlsMode == "" {
		tlsMode = "none"
	}
	wsPath := r.WSPath
	if wsPath == "" {
		wsPath = "/ws"
	}
	routeMode := common.NormalizedRouteMode(r.RouteMode)
	routeHops, err := common.CanonicalRouteHops(r.RouteHops)
	if err != nil {
		return nil, err
	}
	lbStrategy := common.NormalizedLoadBalanceStrategy(r.LBStrategy)
	chainType := r.ChainType
	if chainType == "" {
		chainType = "custom"
	}

	res, err := d.db.Exec(
		"INSERT INTO rules (owner_user_id, node_id, name, protocol, listen_port, target_addr, target_port, speed_limit, traffic_limit, proxy_protocol, blocked_protos, pool_size, tls_mode, tls_sni, ws_enabled, ws_path, route_mode, route_hops, entry_group, relay_groups, exit_group, lb_strategy, parent_rule_id, chain_type, sni_hosts) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		ownerUserID, r.NodeID, r.Name, r.Protocol, r.ListenPort, r.TargetAddr, r.TargetPort, r.SpeedLimit, r.TrafficLimit,
		r.ProxyProtocol, r.BlockedProtos, r.PoolSize, tlsMode, r.TLSSni, r.WSEnabled, wsPath,
		routeMode, routeHops, r.EntryGroup, r.RelayGroups, r.ExitGroup, lbStrategy,
		r.ParentRuleID, chainType, r.SNIHosts,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &model.Rule{
		ID:             id,
		OwnerUserID:    ownerUserID,
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
		RouteHops:      routeHops,
		EntryGroup:     r.EntryGroup,
		RelayGroups:    r.RelayGroups,
		ExitGroup:      r.ExitGroup,
		LBStrategy:     lbStrategy,
		ParentRuleID:   r.ParentRuleID,
		ChainType:      chainType,
		SNIHosts:       r.SNIHosts,
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

// ListRules returns top-level rules (parent_rule_id = 0), optionally filtered by node.
// Child rules created for managed chains are hidden from the UI.
func (d *Database) ListRules(nodeID int64) ([]model.Rule, error) {
	var rows *sql.Rows
	var err error
	if nodeID > 0 {
		rows, err = d.db.Query(
			"SELECT "+ruleColumns+" FROM rules WHERE node_id = ? AND parent_rule_id = 0 ORDER BY id",
			nodeID,
		)
	} else {
		rows, err = d.db.Query(
			"SELECT "+ruleColumns+" FROM rules WHERE parent_rule_id = 0 ORDER BY id",
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

// ListRulesVisibleTo returns visible top-level rules for the actor.
func (d *Database) ListRulesVisibleTo(actor *model.User, nodeID int64) ([]model.Rule, error) {
	if actor == nil || actor.Role == "admin" {
		return d.ListRules(nodeID)
	}

	args := []interface{}{actor.ID}
	query := "SELECT " + ruleColumns + " FROM rules WHERE parent_rule_id = 0 AND owner_user_id = ?"
	if nodeID > 0 {
		query += " AND node_id = ?"
		args = append(args, nodeID)
	}
	query += " ORDER BY id"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []model.Rule
	for rows.Next() {
		var rule model.Rule
		if err := scanRule(rows, &rule); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
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
	if r.SpeedLimit != nil {
		rule.SpeedLimit = *r.SpeedLimit
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
	if r.RouteHops != nil {
		rule.RouteHops = *r.RouteHops
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
	if r.ChainType != nil {
		rule.ChainType = *r.ChainType
	}
	if r.SNIHosts != nil {
		rule.SNIHosts = *r.SNIHosts
	}
	routeHops, err := common.CanonicalRouteHops(rule.RouteHops)
	if err != nil {
		return err
	}
	rule.RouteHops = routeHops

	_, err = d.db.Exec(
		"UPDATE rules SET name=?, protocol=?, listen_port=?, target_addr=?, target_port=?, speed_limit=?, traffic_limit=?, enabled=?, proxy_protocol=?, blocked_protos=?, pool_size=?, tls_mode=?, tls_sni=?, ws_enabled=?, ws_path=?, route_mode=?, route_hops=?, entry_group=?, relay_groups=?, exit_group=?, lb_strategy=?, chain_type=?, sni_hosts=? WHERE id=?",
		rule.Name, rule.Protocol, rule.ListenPort, rule.TargetAddr, rule.TargetPort, rule.SpeedLimit, rule.TrafficLimit, rule.Enabled,
		rule.ProxyProtocol, rule.BlockedProtos, rule.PoolSize, rule.TLSMode, rule.TLSSni, rule.WSEnabled, rule.WSPath,
		rule.RouteMode, rule.RouteHops, rule.EntryGroup, rule.RelayGroups, rule.ExitGroup, rule.LBStrategy, rule.ChainType, rule.SNIHosts, id,
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

// DeleteRule deletes a rule and its child rules (for managed chains).
func (d *Database) DeleteRule(id int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}

	// Delete child rules' traffic logs and child rules first
	if _, err := tx.Exec(
		"DELETE FROM traffic_logs WHERE rule_id IN (SELECT id FROM rules WHERE parent_rule_id = ?)", id,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete child traffic_logs: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM rules WHERE parent_rule_id = ?", id); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete child rules: %w", err)
	}

	// Delete the rule itself
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

// ListRulesForSync returns ALL rules for a node (including child rules).
// Used by hub.SyncRulesToNode so relay rules are pushed to nodes.
func (d *Database) ListRulesForSync(nodeID int64) ([]model.Rule, error) {
	rows, err := d.db.Query(
		"SELECT "+ruleColumns+" FROM rules WHERE node_id = ? ORDER BY id",
		nodeID,
	)
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
		active, err := d.IsUserActiveByID(r.OwnerUserID)
		if err != nil {
			return nil, err
		}
		if !active {
			continue
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// ListChildRules returns all child rules belonging to a parent managed chain.
func (d *Database) ListChildRules(parentID int64) ([]model.Rule, error) {
	rows, err := d.db.Query(
		"SELECT "+ruleColumns+" FROM rules WHERE parent_rule_id = ? ORDER BY id",
		parentID,
	)
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

// DeleteChildRules deletes all child rules (and their traffic logs) for a parent chain.
func (d *Database) DeleteChildRules(parentID int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(
		"DELETE FROM traffic_logs WHERE rule_id IN (SELECT id FROM rules WHERE parent_rule_id = ?)",
		parentID,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete child traffic_logs: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM rules WHERE parent_rule_id = ?", parentID); err != nil {
		tx.Rollback()
		return fmt.Errorf("delete child rules: %w", err)
	}

	return tx.Commit()
}

// UpdateRuleTraffic updates rule traffic counters
func (d *Database) UpdateRuleTraffic(ruleID int64, trafficIn, trafficOut int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(
		"UPDATE rules SET traffic_in = traffic_in + ?, traffic_out = traffic_out + ? WHERE id = ?",
		trafficIn, trafficOut, ruleID,
	); err != nil {
		tx.Rollback()
		return err
	}

	var ownerUserID, nodeID int64
	if err := tx.QueryRow("SELECT owner_user_id, node_id FROM rules WHERE id = ?", ruleID).Scan(&ownerUserID, &nodeID); err != nil {
		tx.Rollback()
		return err
	}

	usageDelta := trafficIn + trafficOut
	if ownerUserID > 0 && nodeID > 0 && usageDelta > 0 {
		if _, err := tx.Exec(
			"UPDATE user_node_access SET traffic_used = traffic_used + ? WHERE user_id = ? AND node_id = ?",
			usageDelta, ownerUserID, nodeID,
		); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
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

// CountTopLevelRulesByOwner returns the number of top-level rules owned by a user.
func (d *Database) CountTopLevelRulesByOwner(ownerUserID int64) (int, error) {
	var count int
	err := d.db.QueryRow(
		"SELECT COUNT(*) FROM rules WHERE owner_user_id = ? AND parent_rule_id = 0",
		ownerUserID,
	).Scan(&count)
	return count, err
}

// CheckUserNodeTrafficQuotaExceeded checks whether the user's node assignment exhausted its quota.
func (d *Database) CheckUserNodeTrafficQuotaExceeded(userID, nodeID int64) (bool, error) {
	var quota, used int64
	if err := d.db.QueryRow(
		"SELECT traffic_quota, traffic_used FROM user_node_access WHERE user_id = ? AND node_id = ?",
		userID, nodeID,
	).Scan(&quota, &used); err != nil {
		return false, err
	}
	if quota <= 0 {
		return false, nil
	}
	return used >= quota, nil
}

// CheckUserTrafficQuotaExceeded aggregates assigned-node quotas for compatibility.
func (d *Database) CheckUserTrafficQuotaExceeded(userID int64) (bool, error) {
	var quota, used int64
	if err := d.db.QueryRow(
		"SELECT COALESCE(SUM(traffic_quota), 0), COALESCE(SUM(traffic_used), 0) FROM user_node_access WHERE user_id = ?",
		userID,
	).Scan(&quota, &used); err != nil {
		return false, err
	}
	if quota <= 0 {
		return false, nil
	}
	return used >= quota, nil
}

// DisableRulesByOwner disables all enabled rules for an owner and returns affected rules.
func (d *Database) DisableRulesByOwner(ownerUserID int64) ([]model.Rule, error) {
	rows, err := d.db.Query(
		"SELECT "+ruleColumns+" FROM rules WHERE owner_user_id = ? AND enabled = 1 ORDER BY id",
		ownerUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []model.Rule
	for rows.Next() {
		var rule model.Rule
		if err := scanRule(rows, &rule); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	if len(rules) == 0 {
		return rules, nil
	}

	if _, err := d.db.Exec("UPDATE rules SET enabled = 0 WHERE owner_user_id = ? AND enabled = 1", ownerUserID); err != nil {
		return nil, err
	}
	return rules, nil
}

// DisableRulesByUserNode disables all enabled rules for a user on one node and returns affected rules.
func (d *Database) DisableRulesByUserNode(userID, nodeID int64) ([]model.Rule, error) {
	rows, err := d.db.Query(
		"SELECT "+ruleColumns+" FROM rules WHERE owner_user_id = ? AND node_id = ? AND enabled = 1 ORDER BY id",
		userID, nodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []model.Rule
	for rows.Next() {
		var rule model.Rule
		if err := scanRule(rows, &rule); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	if len(rules) == 0 {
		return rules, nil
	}

	if _, err := d.db.Exec("UPDATE rules SET enabled = 0 WHERE owner_user_id = ? AND node_id = ? AND enabled = 1", userID, nodeID); err != nil {
		return nil, err
	}
	return rules, nil
}

// IsUserExpiredByID returns whether the user's account is expired.
func (d *Database) IsUserExpiredByID(userID int64) (bool, error) {
	var expiresAt sql.NullTime
	if err := d.db.QueryRow("SELECT expires_at FROM users WHERE id = ?", userID).Scan(&expiresAt); err != nil {
		return false, err
	}
	return expiresAt.Valid && !expiresAt.Time.After(time.Now()), nil
}

// IsUserActiveByID returns whether the user's account is currently active.
func (d *Database) IsUserActiveByID(userID int64) (bool, error) {
	if userID <= 0 {
		return true, nil
	}
	user, err := d.GetUserByID(userID)
	if err != nil {
		return false, err
	}
	if !user.Enabled {
		return false, nil
	}
	return user.ExpiresAt == nil || user.ExpiresAt.After(time.Now()), nil
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

// GetRuleCountVisibleTo returns top-level rule counts for the actor scope.
func (d *Database) GetRuleCountVisibleTo(actor *model.User) (total, active int, err error) {
	if actor == nil || actor.Role == "admin" {
		err = d.db.QueryRow("SELECT COUNT(*) FROM rules WHERE parent_rule_id = 0").Scan(&total)
		if err != nil {
			return
		}
		err = d.db.QueryRow("SELECT COUNT(*) FROM rules WHERE parent_rule_id = 0 AND enabled = 1").Scan(&active)
		return
	}

	err = d.db.QueryRow(
		"SELECT COUNT(*) FROM rules WHERE parent_rule_id = 0 AND owner_user_id = ?",
		actor.ID,
	).Scan(&total)
	if err != nil {
		return
	}
	err = d.db.QueryRow(
		"SELECT COUNT(*) FROM rules WHERE parent_rule_id = 0 AND owner_user_id = ? AND enabled = 1",
		actor.ID,
	).Scan(&active)
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

// GetAggregateTrafficLogs returns hourly aggregate traffic across all rules.
func (d *Database) GetAggregateTrafficLogs(hours int) ([]model.TrafficLog, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	rows, err := d.db.Query(
		`SELECT 0, 0, 0, COALESCE(SUM(traffic_in),0), COALESCE(SUM(traffic_out),0), recorded_at
		 FROM traffic_logs WHERE recorded_at >= ?
		 GROUP BY recorded_at ORDER BY recorded_at`,
		since,
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

// GetAggregateTrafficLogsVisibleTo returns hourly aggregate traffic scoped to the actor.
func (d *Database) GetAggregateTrafficLogsVisibleTo(actor *model.User, hours int) ([]model.TrafficLog, error) {
	if actor == nil || actor.Role == "admin" {
		return d.GetAggregateTrafficLogs(hours)
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	rows, err := d.db.Query(
		`SELECT 0, 0, 0, COALESCE(SUM(tl.traffic_in),0), COALESCE(SUM(tl.traffic_out),0), tl.recorded_at
		   FROM traffic_logs tl
		   JOIN rules r ON r.id = tl.rule_id
		  WHERE tl.recorded_at >= ? AND r.owner_user_id = ? AND r.parent_rule_id = 0
		  GROUP BY tl.recorded_at
		  ORDER BY tl.recorded_at`,
		since, actor.ID,
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

// GetTotalTrafficVisibleTo returns scoped top-level rule traffic totals.
func (d *Database) GetTotalTrafficVisibleTo(actor *model.User) (totalIn, totalOut int64, err error) {
	if actor == nil || actor.Role == "admin" {
		err = d.db.QueryRow(
			"SELECT COALESCE(SUM(traffic_in),0), COALESCE(SUM(traffic_out),0) FROM rules WHERE parent_rule_id = 0",
		).Scan(&totalIn, &totalOut)
		return
	}
	err = d.db.QueryRow(
		"SELECT COALESCE(SUM(traffic_in),0), COALESCE(SUM(traffic_out),0) FROM rules WHERE parent_rule_id = 0 AND owner_user_id = ?",
		actor.ID,
	).Scan(&totalIn, &totalOut)
	return
}

// CountAssignedNodes returns the number of node assignments for a user.
func (d *Database) CountAssignedNodes(userID int64) (int, error) {
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM user_node_access WHERE user_id = ?", userID).Scan(&count)
	return count, err
}

// GetRemainingTrafficForUser returns the total remaining quota across assigned nodes.
func (d *Database) GetRemainingTrafficForUser(userID int64) (int64, error) {
	var remaining int64
	err := d.db.QueryRow(
		`SELECT COALESCE(SUM(CASE
			WHEN traffic_quota <= 0 THEN 0
			WHEN traffic_used >= traffic_quota THEN 0
			ELSE traffic_quota - traffic_used
		END), 0)
		FROM user_node_access
		WHERE user_id = ?`,
		userID,
	).Scan(&remaining)
	return remaining, err
}

// GetDashboardStats returns the dashboard overview stats
func (d *Database) GetDashboardStats(actor *model.User) (*model.DashboardStats, error) {
	stats := &model.DashboardStats{}
	var err error

	stats.TotalNodes, stats.OnlineNodes, err = d.GetNodeCountVisibleTo(actor)
	if err != nil {
		return nil, err
	}

	stats.TotalRules, stats.ActiveRules, err = d.GetRuleCountVisibleTo(actor)
	if err != nil {
		return nil, err
	}

	stats.TotalTrafficIn, stats.TotalTrafficOut, err = d.GetTotalTrafficVisibleTo(actor)
	if err != nil {
		return nil, err
	}

	if actor == nil || actor.Role == "admin" {
		stats.TotalUsers, err = d.GetUserCount()
		if err != nil {
			return nil, err
		}
	} else {
		stats.TotalUsers = 1
		stats.AssignedNodes, err = d.CountAssignedNodes(actor.ID)
		if err != nil {
			return nil, err
		}
		stats.RemainingTraffic, err = d.GetRemainingTrafficForUser(actor.ID)
		if err != nil {
			return nil, err
		}
	}

	return stats, nil
}

// GetDashboardStatsAll is retained for older callers that expect global stats.
func (d *Database) GetDashboardStatsAll() (*model.DashboardStats, error) {
	return d.GetDashboardStats(nil)
}
