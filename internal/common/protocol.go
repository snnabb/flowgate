package common

import (
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
