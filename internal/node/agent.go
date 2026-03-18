package node

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/flowgate/flowgate/internal/common"
	"github.com/flowgate/flowgate/internal/node/forwarder"
)

// Agent connects to the Panel and manages forwarding rules
type Agent struct {
	panelURL string
	apiKey   string
	useTLS   bool
	conn     *websocket.Conn
	writeMu  sync.Mutex

	tcpForwarders map[int64]*forwarder.TCPForwarder
	udpForwarders map[int64]*forwarder.UDPForwarder
	mu            sync.Mutex

	stats     *SystemStats
	collector *TrafficCollector
	stopCh    chan struct{}
}

// NewAgent creates a new node agent
func NewAgent(panelURL, apiKey string, useTLS bool) *Agent {
	return &Agent{
		panelURL:      panelURL,
		apiKey:        apiKey,
		useTLS:        useTLS,
		tcpForwarders: make(map[int64]*forwarder.TCPForwarder),
		udpForwarders: make(map[int64]*forwarder.UDPForwarder),
		stats:         NewSystemStats(),
		collector:     NewTrafficCollector(),
		stopCh:        make(chan struct{}),
	}
}

// Start connects to the panel and enters the main loop
func (a *Agent) Start() error {
	log.Println("🚀 FlowGate Node starting...")
	log.Printf("   Panel: %s", a.panelURL)

	for {
		select {
		case <-a.stopCh:
			return nil
		default:
		}

		err := a.connectAndRun()
		if err != nil {
			log.Printf("[Agent] Connection lost: %v, reconnecting in 5s...", err)
		}

		select {
		case <-a.stopCh:
			return nil
		case <-time.After(5 * time.Second):
		}
	}
}

func (a *Agent) connectAndRun() error {
	// Build WebSocket URL with API key
	panelURL := a.normalizePanelURL()
	sep := "?"
	if strings.Contains(panelURL, "?") {
		sep = "&"
	}
	url := panelURL + sep + "key=" + a.apiKey

	log.Printf("[Agent] Connecting to %s", panelURL)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		return err
	}

	a.writeMu.Lock()
	a.conn = conn
	a.writeMu.Unlock()

	defer func() {
		a.writeMu.Lock()
		if a.conn == conn {
			a.conn = nil
		}
		a.writeMu.Unlock()
		conn.Close()
	}()

	log.Println("[Agent] Connected to panel!")

	// Start reporting goroutines
	done := make(chan struct{})
	go a.reportLoop(done)

	// Read messages from panel
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			close(done)
			return err
		}

		var msg common.WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		a.handleMessage(&msg)
	}
}

func (a *Agent) normalizePanelURL() string {
	if !a.useTLS {
		return a.panelURL
	}

	switch {
	case strings.HasPrefix(a.panelURL, "ws://"):
		return "wss://" + strings.TrimPrefix(a.panelURL, "ws://")
	case strings.HasPrefix(a.panelURL, "http://"):
		return "https://" + strings.TrimPrefix(a.panelURL, "http://")
	default:
		return a.panelURL
	}
}

func (a *Agent) handleMessage(msg *common.WSMessage) {
	switch msg.Type {
	case common.MsgTypeCommand:
		a.handleCommand(msg)
	case common.MsgTypeHeartbeat:
		// Respond to heartbeat
		resp := common.NewMessage(common.MsgTypeHeartbeat, "", nil)
		if err := a.writeWSMessage(resp); err != nil {
			log.Printf("[Agent] Failed to respond heartbeat: %v", err)
		}
	}
}

func (a *Agent) handleCommand(msg *common.WSMessage) {
	switch msg.Action {
	case common.ActionSyncRules:
		a.handleSyncRules(msg.Data)
	case common.ActionAddRule:
		a.handleAddRule(msg.Data)
	case common.ActionDelRule:
		a.handleDelRule(msg.Data)
	case common.ActionUpdateRule:
		a.handleUpdateRule(msg.Data)
	}
}

func (a *Agent) handleSyncRules(data interface{}) {
	jsonData, _ := json.Marshal(data)
	var rules []common.RuleConfig
	if err := json.Unmarshal(jsonData, &rules); err != nil {
		log.Printf("[Agent] Failed to parse sync rules: %v", err)
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Stop all existing forwarders
	for id, fwd := range a.tcpForwarders {
		fwd.Stop()
		a.collector.UnregisterTCP(id)
	}
	for id, fwd := range a.udpForwarders {
		fwd.Stop()
		a.collector.UnregisterUDP(id)
	}
	a.tcpForwarders = make(map[int64]*forwarder.TCPForwarder)
	a.udpForwarders = make(map[int64]*forwarder.UDPForwarder)

	// Start new forwarders
	for _, rule := range rules {
		a.startRule(rule)
	}

	log.Printf("[Agent] Synced %d rules", len(rules))
}

func (a *Agent) handleAddRule(data interface{}) {
	jsonData, _ := json.Marshal(data)
	var rule common.RuleConfig
	if err := json.Unmarshal(jsonData, &rule); err != nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.startRule(rule)
	log.Printf("[Agent] Added rule %d: :%d -> %s:%d (%s)", rule.ID, rule.ListenPort, rule.TargetAddr, rule.TargetPort, rule.Protocol)
}

func (a *Agent) handleDelRule(data interface{}) {
	jsonData, _ := json.Marshal(data)
	var rule common.RuleConfig
	if err := json.Unmarshal(jsonData, &rule); err != nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.stopRule(rule.ID)
	log.Printf("[Agent] Deleted rule %d", rule.ID)
}

func (a *Agent) handleUpdateRule(data interface{}) {
	jsonData, _ := json.Marshal(data)
	var rule common.RuleConfig
	if err := json.Unmarshal(jsonData, &rule); err != nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Stop old, start new
	a.stopRule(rule.ID)
	if rule.Enabled {
		a.startRule(rule)
	}
	log.Printf("[Agent] Updated rule %d", rule.ID)
}

func (a *Agent) startRule(rule common.RuleConfig) {
	proto := strings.ToLower(rule.Protocol)

	if proto == "tcp" || proto == "tcp+udp" {
		fwd := forwarder.NewTCPForwarder(rule.ID, rule.ListenPort, rule.TargetAddr, rule.TargetPort, rule.SpeedLimit)
		if err := fwd.Start(); err != nil {
			log.Printf("[Agent] TCP rule %d start failed: %v", rule.ID, err)
		} else {
			a.tcpForwarders[rule.ID] = fwd
			a.collector.RegisterTCP(rule.ID, fwd)
		}
	}

	if proto == "udp" || proto == "tcp+udp" {
		fwd := forwarder.NewUDPForwarder(rule.ID, rule.ListenPort, rule.TargetAddr, rule.TargetPort, rule.SpeedLimit)
		if err := fwd.Start(); err != nil {
			log.Printf("[Agent] UDP rule %d start failed: %v", rule.ID, err)
		} else {
			a.udpForwarders[rule.ID] = fwd
			a.collector.RegisterUDP(rule.ID, fwd)
		}
	}
}

func (a *Agent) stopRule(id int64) {
	if fwd, ok := a.tcpForwarders[id]; ok {
		fwd.Stop()
		a.collector.UnregisterTCP(id)
		delete(a.tcpForwarders, id)
	}
	if fwd, ok := a.udpForwarders[id]; ok {
		fwd.Stop()
		a.collector.UnregisterUDP(id)
		delete(a.udpForwarders, id)
	}
}

func (a *Agent) reportLoop(done chan struct{}) {
	statsTicker := time.NewTicker(10 * time.Second)
	statusTicker := time.NewTicker(30 * time.Second)
	defer statsTicker.Stop()
	defer statusTicker.Stop()

	for {
		select {
		case <-done:
			return
		case <-statsTicker.C:
			a.reportTraffic()
		case <-statusTicker.C:
			a.reportStatus()
		}
	}
}

func (a *Agent) reportTraffic() {
	reports := a.collector.Collect()
	if len(reports) == 0 {
		return
	}

	msg := common.NewMessage(common.MsgTypeReport, common.ActionReportStats, reports)
	if err := a.writeWSMessage(msg); err != nil {
		log.Printf("[Agent] Failed to report traffic: %v", err)
	}
}

func (a *Agent) reportStatus() {
	status := common.NodeStatus{
		CPUUsage:    a.stats.GetCPUUsage(),
		MemUsage:    a.stats.GetMemUsage(),
		Uptime:      a.stats.GetUptime(),
		Connections: a.collector.GetTotalConnections(),
		GoRoutines:  a.stats.GetGoRoutines(),
	}

	msg := common.NewMessage(common.MsgTypeReport, common.ActionReportStatus, status)
	if err := a.writeWSMessage(msg); err != nil {
		log.Printf("[Agent] Failed to report status: %v", err)
	}
}

func (a *Agent) writeWSMessage(msg common.WSMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	if a.conn == nil {
		return nil
	}

	return a.conn.WriteMessage(websocket.TextMessage, data)
}
