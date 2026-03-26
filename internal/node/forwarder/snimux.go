package forwarder

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SNIRoute represents a single SNI-based forwarding target within a port mux.
type SNIRoute struct {
	RuleID     int64
	TargetAddr string
	TargetPort int
	SpeedLimit int

	trafficIn  int64
	trafficOut int64
	connections int32
}

// GetAndResetTraffic returns accumulated traffic and resets counters.
func (r *SNIRoute) GetAndResetTraffic() (in, out int64) {
	in = atomic.SwapInt64(&r.trafficIn, 0)
	out = atomic.SwapInt64(&r.trafficOut, 0)
	return
}

// GetConnections returns current active connections.
func (r *SNIRoute) GetConnections() int {
	return int(atomic.LoadInt32(&r.connections))
}

// SNIMuxForwarder manages a single TCP listener that routes connections
// based on TLS SNI hostname to different backend targets.
type SNIMuxForwarder struct {
	ListenPort int

	mu       sync.RWMutex
	routes   map[string]*SNIRoute // hostname -> route
	fallback *SNIRoute            // route for non-TLS or unmatched SNI (hostname "*")
	listener net.Listener
	running  bool
	stopCh   chan struct{}
}

// NewSNIMuxForwarder creates a new SNI multiplexer on the given port.
func NewSNIMuxForwarder(port int) *SNIMuxForwarder {
	return &SNIMuxForwarder{
		ListenPort: port,
		routes:     make(map[string]*SNIRoute),
		stopCh:     make(chan struct{}),
	}
}

// AddRoute registers an SNI hostname -> backend target mapping.
// If hostname is "*", it becomes the fallback route.
func (f *SNIMuxForwarder) AddRoute(hostname string, ruleID int64, targetAddr string, targetPort int, speedLimit int) *SNIRoute {
	f.mu.Lock()
	defer f.mu.Unlock()

	route := &SNIRoute{
		RuleID:     ruleID,
		TargetAddr: targetAddr,
		TargetPort: targetPort,
		SpeedLimit: speedLimit,
	}

	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if hostname == "*" {
		f.fallback = route
	} else {
		f.routes[hostname] = route
	}
	return route
}

// RemoveRoute removes an SNI route. Returns true if the mux has no routes left.
func (f *SNIMuxForwarder) RemoveRoute(hostname string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if hostname == "*" {
		f.fallback = nil
	} else {
		delete(f.routes, hostname)
	}

	return len(f.routes) == 0 && f.fallback == nil
}

// RemoveRuleRoutes removes all routes belonging to a specific rule ID.
// Returns true if the mux has no routes left.
func (f *SNIMuxForwarder) RemoveRuleRoutes(ruleID int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	for host, route := range f.routes {
		if route.RuleID == ruleID {
			delete(f.routes, host)
		}
	}
	if f.fallback != nil && f.fallback.RuleID == ruleID {
		f.fallback = nil
	}

	return len(f.routes) == 0 && f.fallback == nil
}

// RouteCount returns the total number of routes.
func (f *SNIMuxForwarder) RouteCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	n := len(f.routes)
	if f.fallback != nil {
		n++
	}
	return n
}

// GetRouteForRule returns the SNIRoute for a given rule ID, or nil.
func (f *SNIMuxForwarder) GetRouteForRule(ruleID int64) *SNIRoute {
	f.mu.RLock()
	defer f.mu.RUnlock()

	for _, route := range f.routes {
		if route.RuleID == ruleID {
			return route
		}
	}
	if f.fallback != nil && f.fallback.RuleID == ruleID {
		return f.fallback
	}
	return nil
}

// AllRoutes returns all routes (for traffic collection).
func (f *SNIMuxForwarder) AllRoutes() []*SNIRoute {
	f.mu.RLock()
	defer f.mu.RUnlock()

	routes := make([]*SNIRoute, 0, len(f.routes)+1)
	for _, r := range f.routes {
		routes = append(routes, r)
	}
	if f.fallback != nil {
		routes = append(routes, f.fallback)
	}
	return routes
}

// Start begins listening on the configured port.
func (f *SNIMuxForwarder) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.running {
		return nil
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", f.ListenPort))
	if err != nil {
		return fmt.Errorf("snimux listen :%d: %w", f.ListenPort, err)
	}

	f.listener = ln
	f.running = true
	f.stopCh = make(chan struct{})

	go f.acceptLoop()
	log.Printf("[snimux] listening on :%d", f.ListenPort)
	return nil
}

// Stop closes the listener and signals all connections to drain.
func (f *SNIMuxForwarder) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.running {
		return
	}
	f.running = false
	close(f.stopCh)
	if f.listener != nil {
		f.listener.Close()
	}
	log.Printf("[snimux] stopped :%d", f.ListenPort)
}

// IsRunning returns whether the mux is actively listening.
func (f *SNIMuxForwarder) IsRunning() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.running
}

func (f *SNIMuxForwarder) acceptLoop() {
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			select {
			case <-f.stopCh:
				return
			default:
				log.Printf("[snimux] accept error: %v", err)
				continue
			}
		}
		go f.handleConn(conn)
	}
}

func (f *SNIMuxForwarder) handleConn(conn net.Conn) {
	defer conn.Close()

	// Peek up to 1024 bytes for TLS ClientHello
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	peek := make([]byte, 1024)
	n, err := conn.Read(peek)
	conn.SetReadDeadline(time.Time{})
	if err != nil || n == 0 {
		return
	}
	peek = peek[:n]

	// Create a PeekConn to replay the peeked data
	pconn := &peekConn{Conn: conn, buf: peek}

	// Try to extract SNI hostname
	sni := parseTLSSNI(peek)

	f.mu.RLock()
	var route *SNIRoute
	if sni != "" {
		route = f.routes[strings.ToLower(sni)]
	}
	if route == nil {
		route = f.fallback
	}
	f.mu.RUnlock()

	if route == nil {
		return
	}

	// Dial backend
	target := net.JoinHostPort(route.TargetAddr, fmt.Sprintf("%d", route.TargetPort))
	backend, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		return
	}
	defer backend.Close()

	atomic.AddInt32(&route.connections, 1)
	defer atomic.AddInt32(&route.connections, -1)

	// Bidirectional relay with traffic accounting
	done := make(chan struct{})
	go func() {
		n, _ := io.Copy(backend, pconn)
		atomic.AddInt64(&route.trafficIn, n)
		done <- struct{}{}
	}()

	out, _ := io.Copy(pconn, backend)
	atomic.AddInt64(&route.trafficOut, out)
	<-done
}

// peekConn replays buffered bytes then delegates to the underlying connection.
type peekConn struct {
	net.Conn
	buf    []byte
	offset int
}

func (c *peekConn) Read(p []byte) (int, error) {
	if c.offset < len(c.buf) {
		n := copy(p, c.buf[c.offset:])
		c.offset += n
		return n, nil
	}
	return c.Conn.Read(p)
}

// parseTLSSNI extracts the SNI hostname from a TLS ClientHello message.
// Returns empty string if the data is not a valid ClientHello or has no SNI.
func parseTLSSNI(data []byte) string {
	// Minimum: 5 (record) + 4 (handshake header) + 2 (version) + 32 (random) = 43
	if len(data) < 43 {
		return ""
	}

	// TLS record: ContentType(1) + Version(2) + Length(2)
	if data[0] != 0x16 { // Not a handshake record
		return ""
	}

	recordLen := int(data[3])<<8 | int(data[4])
	handshake := data[5:]
	if len(handshake) < recordLen {
		handshake = handshake[:len(handshake)]
	} else {
		handshake = handshake[:recordLen]
	}

	// Handshake: Type(1) + Length(3)
	if len(handshake) < 4 || handshake[0] != 0x01 { // Not ClientHello
		return ""
	}
	hsLen := int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
	body := handshake[4:]
	if len(body) < hsLen {
		// partial read, use what we have
	}

	// ClientHello body: Version(2) + Random(32) = 34
	if len(body) < 34 {
		return ""
	}
	pos := 34

	// Session ID: Length(1) + data
	if pos >= len(body) {
		return ""
	}
	sidLen := int(body[pos])
	pos += 1 + sidLen

	// Cipher Suites: Length(2) + data
	if pos+2 > len(body) {
		return ""
	}
	csLen := int(body[pos])<<8 | int(body[pos+1])
	pos += 2 + csLen

	// Compression Methods: Length(1) + data
	if pos >= len(body) {
		return ""
	}
	cmLen := int(body[pos])
	pos += 1 + cmLen

	// Extensions: Length(2)
	if pos+2 > len(body) {
		return ""
	}
	extLen := int(body[pos])<<8 | int(body[pos+1])
	pos += 2

	extEnd := pos + extLen
	if extEnd > len(body) {
		extEnd = len(body)
	}

	// Walk extensions
	for pos+4 <= extEnd {
		extType := int(body[pos])<<8 | int(body[pos+1])
		extDataLen := int(body[pos+2])<<8 | int(body[pos+3])
		pos += 4

		if extType == 0x0000 { // server_name extension
			return parseServerNameExt(body[pos:min(pos+extDataLen, extEnd)])
		}
		pos += extDataLen
	}

	return ""
}

func parseServerNameExt(data []byte) string {
	// ServerNameList: Length(2) + entries
	if len(data) < 2 {
		return ""
	}
	listLen := int(data[0])<<8 | int(data[1])
	data = data[2:]
	if len(data) < listLen {
		listLen = len(data)
	}

	pos := 0
	for pos+3 <= listLen {
		nameType := data[pos]
		nameLen := int(data[pos+1])<<8 | int(data[pos+2])
		pos += 3
		if nameType == 0x00 && nameLen > 0 && pos+nameLen <= listLen { // host_name
			return string(data[pos : pos+nameLen])
		}
		pos += nameLen
	}
	return ""
}

// ParseSNIHosts parses a JSON array of hostname strings.
func ParseSNIHosts(raw string) ([]string, error) {
	if raw == "" || raw == "[]" {
		return nil, nil
	}
	var hosts []string
	if err := json.Unmarshal([]byte(raw), &hosts); err != nil {
		return nil, fmt.Errorf("invalid sni_hosts JSON: %w", err)
	}
	for i, h := range hosts {
		hosts[i] = strings.ToLower(strings.TrimSpace(h))
	}
	return hosts, nil
}
