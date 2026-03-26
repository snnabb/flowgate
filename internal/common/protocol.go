package common

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Message types for Panel <-> Node communication
const (
	MsgTypeCommand   = "command"
	MsgTypeReport    = "report"
	MsgTypeHeartbeat = "heartbeat"
)

// Phase 2 route modes.
const (
	RouteModeDirect     = "direct"
	RouteModeHopChain   = "hop_chain"
	RouteModeGroupChain = "group_chain" // legacy alias kept for compatibility
)

// Phase 2 load-balancing strategies.
const (
	LBStrategyNone               = "none"
	LBStrategyRoundRobin         = "round_robin"
	LBStrategyWeightedRoundRobin = "weighted_round_robin"
	LBStrategyLeastConnections   = "least_connections"
	LBStrategyLeastLatency       = "least_latency"
	LBStrategyIPHash             = "ip_hash"
	LBStrategyFailover           = "failover"
)

// Command actions (Panel -> Node)
const (
	ActionAddRule    = "add_rule"
	ActionDelRule    = "del_rule"
	ActionUpdateRule = "update_rule"
	ActionSyncRules  = "sync_rules"
)

// Report actions (Node -> Panel)
const (
	ActionReportStats      = "report_stats"
	ActionReportStatus     = "report_status"
	ActionReportRuleStatus = "report_rule_status"
	ActionReportLatency    = "report_latency"
)

// WSMessage is the WebSocket message envelope
type WSMessage struct {
	Type      string      `json:"type"`
	Action    string      `json:"action"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// NewMessage creates a new WSMessage with current timestamp
func NewMessage(msgType, action string, data interface{}) WSMessage {
	return WSMessage{
		Type:      msgType,
		Action:    action,
		Data:      data,
		Timestamp: time.Now().Unix(),
	}
}

// RuleConfig is the forwarding rule sent to nodes
type RuleConfig struct {
	ID         int64  `json:"id"`
	Protocol   string `json:"protocol"`    // tcp, udp, tcp+udp
	ListenPort int    `json:"listen_port"`
	TargetAddr string `json:"target_addr"`
	TargetPort int    `json:"target_port"`
	SpeedLimit int    `json:"speed_limit"` // KB/s, 0 = unlimited
	Enabled    bool   `json:"enabled"`

	// Tunnel engine fields (Phase 1)
	ProxyProtocol int    `json:"proxy_protocol"`  // 0=off, 1=v1, 2=v2
	BlockedProtos string `json:"blocked_protos"`   // comma-separated: "socks,http"
	PoolSize      int    `json:"pool_size"`        // 0=disabled, >0=pre-connect pool
	TLSMode       string `json:"tls_mode"`         // none/client/server/both
	TLSSni        string `json:"tls_sni"`          // SNI for outbound TLS
	WSEnabled     bool   `json:"ws_enabled"`       // accept connections over WebSocket
	WSPath        string `json:"ws_path"`          // WebSocket path, default "/ws"

	// Phase 2 route skeleton fields
	RouteMode   string `json:"route_mode"`   // direct/hop_chain
	RouteHops   string `json:"route_hops"`   // JSON-encoded ordered hops
	EntryGroup  string `json:"entry_group"`  // legacy field, no longer used by new UI
	RelayGroups string `json:"relay_groups"` // legacy field, no longer used by new UI
	ExitGroup   string `json:"exit_group"`   // legacy field, no longer used by new UI
	LBStrategy  string `json:"lb_strategy"`  // reserved top-level strategy field
}

// RouteTarget represents one dialable target inside a hop.
type RouteTarget struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Weight int    `json:"weight,omitempty"`
}

// RouteHop represents one ordered hop in a Phase 2 chain.
type RouteHop struct {
	Order      int           `json:"order"`
	Targets    []RouteTarget `json:"targets"`
	LBStrategy string        `json:"lb_strategy,omitempty"`
}

// NormalizedTLSMode returns the persisted/default TLS mode for a rule.
func NormalizedTLSMode(mode string) string {
	if mode == "" {
		return "none"
	}
	return strings.ToLower(mode)
}

// ValidateTunnelSettings rejects unsupported tunnel combinations for Phase 1.
func ValidateTunnelSettings(wsEnabled bool, tlsMode string) error {
	mode := NormalizedTLSMode(tlsMode)
	if wsEnabled && (mode == "client" || mode == "both") {
		return fmt.Errorf("WebSocket 隧道暂不支持与入站 TLS 同时开启")
	}
	return nil
}

// NormalizedRouteMode returns the persisted/default route mode for a rule.
func NormalizedRouteMode(mode string) string {
	if mode == "" {
		return RouteModeDirect
	}
	switch strings.ToLower(mode) {
	case RouteModeGroupChain:
		return RouteModeHopChain
	default:
		return strings.ToLower(mode)
	}
}

// NormalizedLoadBalanceStrategy returns the persisted/default load-balance strategy.
func NormalizedLoadBalanceStrategy(strategy string) string {
	if strategy == "" {
		return LBStrategyNone
	}
	return strings.ToLower(strategy)
}

// RouteModeUsesNodeRuntime reports whether the current node runtime can apply the rule directly.
func RouteModeUsesNodeRuntime(mode string) bool {
	m := NormalizedRouteMode(mode)
	return m == RouteModeDirect || m == RouteModeHopChain
}

// ValidateRouteSettings validates the reserved Phase 2 route fields.
func ValidateRouteSettings(routeMode, routeHops, lbStrategy string) error {
	mode := NormalizedRouteMode(routeMode)
	switch mode {
	case RouteModeDirect, RouteModeHopChain:
	default:
		return fmt.Errorf("unsupported route mode: %s", routeMode)
	}

	strategy := NormalizedLoadBalanceStrategy(lbStrategy)
	switch strategy {
	case LBStrategyNone, LBStrategyRoundRobin, LBStrategyWeightedRoundRobin, LBStrategyLeastConnections, LBStrategyLeastLatency, LBStrategyIPHash, LBStrategyFailover:
	default:
		return fmt.Errorf("unsupported load-balance strategy: %s", lbStrategy)
	}

	if mode == RouteModeDirect {
		if normalized := strings.TrimSpace(routeHops); normalized != "" && normalized != "[]" {
			return fmt.Errorf("direct route mode cannot include route_hops")
		}
		return nil
	}

	hops, err := ParseRouteHops(routeHops)
	if err != nil {
		return err
	}
	if len(hops) == 0 {
		return fmt.Errorf("hop_chain route mode requires at least one hop")
	}
	return nil
}

// ParseRouteHops validates and decodes a JSON hop list.
func ParseRouteHops(raw string) ([]RouteHop, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var hops []RouteHop
	if err := json.Unmarshal([]byte(raw), &hops); err != nil {
		return nil, fmt.Errorf("invalid route_hops json: %w", err)
	}

	orders := make(map[int]struct{}, len(hops))
	for i := range hops {
		hop := &hops[i]
		if hop.Order <= 0 {
			return nil, fmt.Errorf("hop order must be greater than 0")
		}
		if _, exists := orders[hop.Order]; exists {
			return nil, fmt.Errorf("duplicate hop order: %d", hop.Order)
		}
		orders[hop.Order] = struct{}{}

		hop.LBStrategy = NormalizedLoadBalanceStrategy(hop.LBStrategy)
		switch hop.LBStrategy {
		case LBStrategyNone, LBStrategyRoundRobin, LBStrategyWeightedRoundRobin, LBStrategyLeastConnections, LBStrategyLeastLatency, LBStrategyIPHash, LBStrategyFailover:
		default:
			return nil, fmt.Errorf("unsupported hop load-balance strategy: %s", hop.LBStrategy)
		}

		if len(hop.Targets) == 0 {
			return nil, fmt.Errorf("hop %d requires at least one target", hop.Order)
		}
		for _, target := range hop.Targets {
			if strings.TrimSpace(target.Host) == "" {
				return nil, fmt.Errorf("hop %d target host is required", hop.Order)
			}
			if target.Port <= 0 || target.Port > 65535 {
				return nil, fmt.Errorf("hop %d target port must be between 1 and 65535", hop.Order)
			}
			if target.Weight < 0 {
				return nil, fmt.Errorf("hop %d target weight must be non-negative", hop.Order)
			}
		}
	}

	return hops, nil
}

// CanonicalRouteHops returns a normalized JSON string for storage.
func CanonicalRouteHops(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "[]", nil
	}

	hops, err := ParseRouteHops(raw)
	if err != nil {
		return "", err
	}
	if len(hops) == 0 {
		return "[]", nil
	}

	data, err := json.Marshal(hops)
	if err != nil {
		return "", fmt.Errorf("marshal route_hops: %w", err)
	}
	return string(data), nil
}

// NodeStatus is the status report from a node
type NodeStatus struct {
	CPUUsage    float64 `json:"cpu_usage"`
	MemUsage    float64 `json:"mem_usage"`
	MemTotal    float64 `json:"mem_total"`
	Uptime      int64   `json:"uptime"`
	Connections int     `json:"connections"`
	GoRoutines  int     `json:"goroutines"`
}

// TrafficReport is the traffic stats report from a node
type TrafficReport struct {
	RuleID    int64 `json:"rule_id"`
	TrafficIn int64 `json:"traffic_in"`  // bytes
	TrafficOut int64 `json:"traffic_out"` // bytes
}

// RuleStatusReport is the runtime apply result reported by a node.
type RuleStatusReport struct {
	RuleID  int64  `json:"rule_id"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// RuleLatencyReport is the latency measurement from node to target.
type RuleLatencyReport struct {
	RuleID  int64   `json:"rule_id"`
	Latency float64 `json:"latency_ms"` // milliseconds, -1 = unreachable
}
