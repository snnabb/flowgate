package common

import "time"

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
	ActionReportStats  = "report_stats"
	ActionReportStatus = "report_status"
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
