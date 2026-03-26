package node

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
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
	hopForwarders map[int64]*forwarder.HopChainForwarder
	sniMuxers     map[int]*forwarder.SNIMuxForwarder // keyed by listen_port
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
		hopForwarders: make(map[int64]*forwarder.HopChainForwarder),
		sniMuxers:     make(map[int]*forwarder.SNIMuxForwarder),
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

	a.reportStatus()

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
	case common.ActionTestLatency:
		log.Printf("[Agent] Received test_latency command")
		a.handleTestLatency(msg.Data)
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
	for id, fwd := range a.hopForwarders {
		fwd.Stop()
		a.collector.UnregisterHopChain(id)
	}
	a.tcpForwarders = make(map[int64]*forwarder.TCPForwarder)
	a.udpForwarders = make(map[int64]*forwarder.UDPForwarder)
	a.hopForwarders = make(map[int64]*forwarder.HopChainForwarder)

	// Start new forwarders
	for _, rule := range rules {
		if err := a.startRule(rule); err != nil {
			a.reportRuleStatus(rule.ID, "error", err.Error())
			continue
		}
		a.reportRuleStatus(rule.ID, "running", "规则已生效")
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

	if err := a.startRule(rule); err != nil {
		a.reportRuleStatus(rule.ID, "error", err.Error())
		return
	}
	a.reportRuleStatus(rule.ID, "running", "规则已生效")
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
	a.reportRuleStatus(rule.ID, "stopped", "规则已移除")
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
		if err := a.startRule(rule); err != nil {
			a.reportRuleStatus(rule.ID, "error", err.Error())
			return
		}
		a.reportRuleStatus(rule.ID, "running", "规则已更新并生效")
	} else {
		a.reportRuleStatus(rule.ID, "stopped", "规则已禁用")
	}
	log.Printf("[Agent] Updated rule %d", rule.ID)
}

func (a *Agent) startRule(rule common.RuleConfig) error {
	if err := common.ValidateTunnelSettings(rule.WSEnabled, rule.TLSMode); err != nil {
		return err
	}

	routeMode := common.NormalizedRouteMode(rule.RouteMode)

	// Dispatch hop_chain rules to the hop-chain forwarder.
	if routeMode == common.RouteModeHopChain {
		return a.startHopChainRule(rule)
	}

	// Dispatch port_mux rules to the SNI mux forwarder.
	if routeMode == common.RouteModePortMux {
		return a.startPortMuxRule(rule)
	}

	proto := strings.ToLower(rule.Protocol)
	var errs []string
	startedTCP := false
	startedUDP := false

	if proto == "tcp" || proto == "tcp+udp" {
		fwd := forwarder.NewTCPForwarder(rule)
		if err := fwd.Start(); err != nil {
			log.Printf("[Agent] TCP rule %d start failed: %v", rule.ID, err)
			errs = append(errs, "TCP: "+err.Error())
		} else {
			a.tcpForwarders[rule.ID] = fwd
			a.collector.RegisterTCP(rule.ID, fwd)
			startedTCP = true
		}
	}

	if proto == "udp" || proto == "tcp+udp" {
		fwd := forwarder.NewUDPForwarder(rule.ID, rule.ListenPort, rule.TargetAddr, rule.TargetPort, rule.SpeedLimit)
		if err := fwd.Start(); err != nil {
			log.Printf("[Agent] UDP rule %d start failed: %v", rule.ID, err)
			errs = append(errs, "UDP: "+err.Error())
		} else {
			a.udpForwarders[rule.ID] = fwd
			a.collector.RegisterUDP(rule.ID, fwd)
			startedUDP = true
		}
	}

	if proto != "tcp" && proto != "udp" && proto != "tcp+udp" {
		return fmt.Errorf("unsupported protocol: %s", rule.Protocol)
	}

	if len(errs) == 0 {
		return nil
	}

	if startedTCP {
		if fwd, ok := a.tcpForwarders[rule.ID]; ok {
			fwd.Stop()
			a.collector.UnregisterTCP(rule.ID)
			delete(a.tcpForwarders, rule.ID)
		}
	}
	if startedUDP {
		if fwd, ok := a.udpForwarders[rule.ID]; ok {
			fwd.Stop()
			a.collector.UnregisterUDP(rule.ID)
			delete(a.udpForwarders, rule.ID)
		}
	}

	return fmt.Errorf(strings.Join(errs, "; "))
}

// startHopChainRule starts a hop-chain forwarder for a rule.
// Only TCP is supported; UDP-only hop_chain rules are rejected.
func (a *Agent) startHopChainRule(rule common.RuleConfig) error {
	proto := strings.ToLower(rule.Protocol)
	if proto == "udp" {
		return fmt.Errorf("hop_chain 暂不支持纯 UDP 协议")
	}

	hops, err := common.ParseRouteHops(rule.RouteHops)
	if err != nil {
		return fmt.Errorf("route_hops 解析失败: %w", err)
	}
	if len(hops) == 0 {
		return fmt.Errorf("hop_chain 至少需要一个跳点")
	}

	fwd := forwarder.NewHopChainForwarder(rule, hops)
	if err := fwd.Start(); err != nil {
		return fmt.Errorf("HopChain: %w", err)
	}

	a.hopForwarders[rule.ID] = fwd
	a.collector.RegisterHopChain(rule.ID, fwd)

	log.Printf("[Agent] HopChain rule %d: :%d -> %d hops started", rule.ID, rule.ListenPort, len(hops))
	return nil
}

// startPortMuxRule adds SNI routes to a shared port mux forwarder.
func (a *Agent) startPortMuxRule(rule common.RuleConfig) error {
	hosts, err := forwarder.ParseSNIHosts(rule.SNIHosts)
	if err != nil {
		return fmt.Errorf("sni_hosts 解析失败: %w", err)
	}
	if len(hosts) == 0 {
		return fmt.Errorf("port_mux 规则必须配置至少一个 SNI 主机名")
	}

	mux, ok := a.sniMuxers[rule.ListenPort]
	if !ok {
		mux = forwarder.NewSNIMuxForwarder(rule.ListenPort)
		a.sniMuxers[rule.ListenPort] = mux
	}

	for _, host := range hosts {
		route := mux.AddRoute(host, rule.ID, rule.TargetAddr, rule.TargetPort, rule.SpeedLimit)
		a.collector.RegisterSNIRoute(rule.ID, route)
	}

	if !mux.IsRunning() {
		if err := mux.Start(); err != nil {
			// Clean up routes we just added
			mux.RemoveRuleRoutes(rule.ID)
			a.collector.UnregisterSNIRoute(rule.ID)
			if mux.RouteCount() == 0 {
				delete(a.sniMuxers, rule.ListenPort)
			}
			return err
		}
	}

	log.Printf("[Agent] PortMux rule %d: :%d SNI hosts=%v", rule.ID, rule.ListenPort, hosts)
	return nil
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
	if fwd, ok := a.hopForwarders[id]; ok {
		fwd.Stop()
		a.collector.UnregisterHopChain(id)
		delete(a.hopForwarders, id)
	}

	// Remove SNI mux routes for this rule
	for port, mux := range a.sniMuxers {
		empty := mux.RemoveRuleRoutes(id)
		a.collector.UnregisterSNIRoute(id)
		if empty {
			mux.Stop()
			delete(a.sniMuxers, port)
		}
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
	memUsage, memTotal := a.stats.GetMemoryUsage()

	status := common.NodeStatus{
		CPUUsage:    a.stats.GetCPUUsage(),
		MemUsage:    memUsage,
		MemTotal:    memTotal,
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

func (a *Agent) reportRuleStatus(ruleID int64, status, message string) {
	msg := common.NewMessage(common.MsgTypeReport, common.ActionReportRuleStatus, common.RuleStatusReport{
		RuleID:  ruleID,
		Status:  status,
		Message: message,
	})
	if err := a.writeWSMessage(msg); err != nil {
		log.Printf("[Agent] Failed to report rule %d status: %v", ruleID, err)
	}
}

// reportLatency measures TCP connect latency from this node to each rule's target.
func (a *Agent) reportLatency() {
	type target struct {
		ruleID int64
		addr   string
	}

	a.mu.Lock()
	var targets []target
	seen := make(map[string]bool)
	for id, fwd := range a.tcpForwarders {
		addr := fmt.Sprintf("%s:%d", fwd.TargetAddr, fwd.TargetPort)
		targets = append(targets, target{ruleID: id, addr: addr})
		seen[addr] = true
	}
	for id, fwd := range a.udpForwarders {
		addr := fmt.Sprintf("%s:%d", fwd.TargetAddr, fwd.TargetPort)
		if seen[addr] {
			continue // already measured via TCP forwarder for the same rule
		}
		targets = append(targets, target{ruleID: id, addr: addr})
	}
	for id, fwd := range a.hopForwarders {
		hopTargets := fwd.FirstHopTargets()
		if len(hopTargets) > 0 {
			addr := fmt.Sprintf("%s:%d", hopTargets[0].Host, hopTargets[0].Port)
			if !seen[addr] {
				targets = append(targets, target{ruleID: id, addr: addr})
				seen[addr] = true
			}
		}
	}
	a.mu.Unlock()

	if len(targets) == 0 {
		return
	}

	var reports []common.RuleLatencyReport
	for _, t := range targets {
		latency := measureTCPLatency(t.addr)
		reports = append(reports, common.RuleLatencyReport{
			RuleID:  t.ruleID,
			Latency: latency,
		})
	}

	msg := common.NewMessage(common.MsgTypeReport, common.ActionReportLatency, reports)
	if err := a.writeWSMessage(msg); err != nil {
		log.Printf("[Agent] Failed to report latency: %v", err)
	}
}

// getRuleTargetAddr returns the dial address for a given rule ID, or "" if not found.
func (a *Agent) getRuleTargetAddr(ruleID int64) string {
	a.mu.Lock()
	defer a.mu.Unlock()

	if fwd, ok := a.tcpForwarders[ruleID]; ok {
		return fmt.Sprintf("%s:%d", fwd.TargetAddr, fwd.TargetPort)
	}
	if fwd, ok := a.udpForwarders[ruleID]; ok {
		return fmt.Sprintf("%s:%d", fwd.TargetAddr, fwd.TargetPort)
	}
	if fwd, ok := a.hopForwarders[ruleID]; ok {
		if targets := fwd.FirstHopTargets(); len(targets) > 0 {
			return fmt.Sprintf("%s:%d", targets[0].Host, targets[0].Port)
		}
	}
	// Check SNI mux routes
	for _, mux := range a.sniMuxers {
		if route := mux.GetRouteForRule(ruleID); route != nil {
			return fmt.Sprintf("%s:%d", route.TargetAddr, route.TargetPort)
		}
	}
	return ""
}

// handleTestLatency handles an on-demand latency test command from the panel.
func (a *Agent) handleTestLatency(data interface{}) {
	jsonData, _ := json.Marshal(data)
	var req common.TestLatencyRequest
	if err := json.Unmarshal(jsonData, &req); err != nil {
		return
	}

	go func() {
		addr := a.getRuleTargetAddr(req.RuleID)
		log.Printf("[Agent] test_latency: rule=%d addr=%q", req.RuleID, addr)
		if addr == "" {
			log.Printf("[Agent] test_latency: no target addr for rule %d, skipping", req.RuleID)
			return
		}
		latency := measureTCPLatency(addr)
		log.Printf("[Agent] test_latency: rule=%d latency=%.1fms", req.RuleID, latency)
		reports := []common.RuleLatencyReport{{RuleID: req.RuleID, Latency: latency}}
		msg := common.NewMessage(common.MsgTypeReport, common.ActionReportLatency, reports)
		if err := a.writeWSMessage(msg); err != nil {
			log.Printf("[Agent] Failed to report test latency: %v", err)
		}
	}()
}

// measureTCPLatency measures the TCP handshake time to addr. Returns ms or -1 on failure.
func measureTCPLatency(addr string) float64 {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return -1
	}
	elapsed := time.Since(start)
	conn.Close()
	return float64(elapsed.Microseconds()) / 1000.0
}
