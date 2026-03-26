package model

import "time"

// User represents a panel user
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"` // admin, reseller, user
	ParentID     int64     `json:"parent_id"`
	TrafficQuota int64     `json:"traffic_quota"`
	TrafficUsed  int64     `json:"traffic_used"`
	Ratio        float64   `json:"ratio"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	MaxRules     int       `json:"max_rules"`
	BandwidthLimit int     `json:"bandwidth_limit"`
	CreatedAt    time.Time `json:"created_at"`
}

// Node represents a forwarding node/agent
type Node struct {
	ID        int64     `json:"id"`
	OwnerUserID int64   `json:"owner_user_id"`
	Name      string    `json:"name"`
	APIKey    string    `json:"api_key,omitempty"`
	GroupName string    `json:"group_name"`
	Status    string    `json:"status"` // online, offline
	IPAddr    string    `json:"ip_addr"`
	CPUUsage  float64   `json:"cpu_usage"`
	MemUsage  float64   `json:"mem_usage"`
	MemTotal  float64   `json:"mem_total"`
	LastSeen  time.Time `json:"last_seen"`
	CreatedAt time.Time `json:"created_at"`
}

// NodeGroup represents a reusable node grouping primitive for Phase 2 routing.
type NodeGroup struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	NodeCount   int       `json:"node_count"`
	CreatedAt   time.Time `json:"created_at"`
}

// Rule represents a forwarding rule
type Rule struct {
	ID           int64     `json:"id"`
	OwnerUserID  int64     `json:"owner_user_id"`
	NodeID       int64     `json:"node_id"`
	Name         string    `json:"name"`
	Protocol     string    `json:"protocol"` // tcp, udp, tcp+udp
	ListenPort   int       `json:"listen_port"`
	TargetAddr   string    `json:"target_addr"`
	TargetPort   int       `json:"target_port"`
	SpeedLimit   int       `json:"speed_limit"`   // KB/s, 0 = unlimited
	TrafficLimit int64     `json:"traffic_limit"`  // bytes, 0 = unlimited
	TrafficIn    int64     `json:"traffic_in"`
	TrafficOut   int64     `json:"traffic_out"`
	Enabled      bool      `json:"enabled"`
	RuntimeStatus  string    `json:"runtime_status"`
	RuntimeMessage string    `json:"runtime_message"`
	Latency        float64   `json:"latency_ms"`
	CreatedAt      time.Time `json:"created_at"`

	// Tunnel engine fields
	ProxyProtocol int    `json:"proxy_protocol"`
	BlockedProtos string `json:"blocked_protos"`
	PoolSize      int    `json:"pool_size"`
	TLSMode       string `json:"tls_mode"`
	TLSSni        string `json:"tls_sni"`
	WSEnabled     bool   `json:"ws_enabled"`
	WSPath        string `json:"ws_path"`

	// Phase 2 route fields
	RouteMode   string `json:"route_mode"`
	RouteHops   string `json:"route_hops"`
	EntryGroup  string `json:"entry_group"`
	RelayGroups string `json:"relay_groups"`
	ExitGroup   string `json:"exit_group"`
	LBStrategy  string `json:"lb_strategy"`

	// Managed chain fields
	ParentRuleID int64  `json:"parent_rule_id"`
	ChainType    string `json:"chain_type"` // "custom" or "managed"

	// Port multiplexing
	SNIHosts string `json:"sni_hosts"` // JSON array of hostnames
}

// TrafficLog represents hourly aggregated traffic
type TrafficLog struct {
	ID         int64     `json:"id"`
	RuleID     int64     `json:"rule_id"`
	NodeID     int64     `json:"node_id"`
	TrafficIn  int64     `json:"traffic_in"`
	TrafficOut int64     `json:"traffic_out"`
	RecordedAt time.Time `json:"recorded_at"`
}

// PanelEvent represents an operational event shown in the panel.
type PanelEvent struct {
	ID        int64     `json:"id"`
	Category  string    `json:"category"`
	Title     string    `json:"title"`
	Details   string    `json:"details"`
	CreatedAt time.Time `json:"created_at"`
}

// LoginRequest is the request body for login
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse is the response body for login
type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// CreateNodeRequest is the request body for creating a node
type CreateNodeRequest struct {
	Name      string `json:"name" binding:"required"`
	GroupName string `json:"group_name"`
	OwnerUserID *int64 `json:"owner_user_id,omitempty"`
}

// CreateNodeGroupRequest is the request body for creating a node group.
type CreateNodeGroupRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// CreateUserRequest is the request body for admin-created users.
type CreateUserRequest struct {
	Username       string     `json:"username" binding:"required"`
	Password       string     `json:"password" binding:"required"`
	Role           string     `json:"role"`
	ParentID       *int64     `json:"parent_id,omitempty"`
	TrafficQuota   int64      `json:"traffic_quota"`
	Ratio          float64    `json:"ratio"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	MaxRules       int        `json:"max_rules"`
	BandwidthLimit int        `json:"bandwidth_limit"`
}

// CreateRuleRequest is the request body for creating a rule
type CreateRuleRequest struct {
	OwnerUserID  *int64 `json:"owner_user_id,omitempty"`
	NodeID       int64  `json:"node_id" binding:"required"`
	Name         string `json:"name"`
	Protocol     string `json:"protocol" binding:"required"`
	ListenPort   int    `json:"listen_port" binding:"required"`
	TargetAddr   string `json:"target_addr" binding:"required"`
	TargetPort   int    `json:"target_port" binding:"required"`
	SpeedLimit   int    `json:"speed_limit"`
	TrafficLimit int64  `json:"traffic_limit"`

	// Tunnel engine fields
	ProxyProtocol int    `json:"proxy_protocol"`
	BlockedProtos string `json:"blocked_protos"`
	PoolSize      int    `json:"pool_size"`
	TLSMode       string `json:"tls_mode"`
	TLSSni        string `json:"tls_sni"`
	WSEnabled     bool   `json:"ws_enabled"`
	WSPath        string `json:"ws_path"`

	// Phase 2 route fields
	RouteMode   string `json:"route_mode"`
	RouteHops   string `json:"route_hops"`
	EntryGroup  string `json:"entry_group"`
	RelayGroups string `json:"relay_groups"`
	ExitGroup   string `json:"exit_group"`
	LBStrategy  string `json:"lb_strategy"`

	// Managed chain fields
	ChainType    string `json:"chain_type"`
	ParentRuleID int64  `json:"parent_rule_id"`

	// Port multiplexing
	SNIHosts string `json:"sni_hosts"`
}

// UpdateRuleRequest is the request body for updating a rule
type UpdateRuleRequest struct {
	Name         string `json:"name"`
	Protocol     string `json:"protocol"`
	ListenPort   int    `json:"listen_port"`
	TargetAddr   string `json:"target_addr"`
	TargetPort   int    `json:"target_port"`
	SpeedLimit   int    `json:"speed_limit"`
	TrafficLimit *int64 `json:"traffic_limit"`
	Enabled      *bool  `json:"enabled"`

	// Tunnel engine fields (pointer types for optional update)
	ProxyProtocol *int    `json:"proxy_protocol"`
	BlockedProtos *string `json:"blocked_protos"`
	PoolSize      *int    `json:"pool_size"`
	TLSMode       *string `json:"tls_mode"`
	TLSSni        *string `json:"tls_sni"`
	WSEnabled     *bool   `json:"ws_enabled"`
	WSPath        *string `json:"ws_path"`

	// Phase 2 route fields
	RouteMode   *string `json:"route_mode"`
	RouteHops   *string `json:"route_hops"`
	EntryGroup  *string `json:"entry_group"`
	RelayGroups *string `json:"relay_groups"`
	ExitGroup   *string `json:"exit_group"`
	LBStrategy  *string `json:"lb_strategy"`

	// Managed chain fields
	ChainType *string `json:"chain_type"`

	// Port multiplexing
	SNIHosts *string `json:"sni_hosts"`
}

// DashboardStats is the overview statistics for the dashboard
type DashboardStats struct {
	TotalNodes    int   `json:"total_nodes"`
	OnlineNodes   int   `json:"online_nodes"`
	TotalRules    int   `json:"total_rules"`
	ActiveRules   int   `json:"active_rules"`
	TotalTrafficIn  int64 `json:"total_traffic_in"`
	TotalTrafficOut int64 `json:"total_traffic_out"`
	TotalUsers    int   `json:"total_users"`
}
