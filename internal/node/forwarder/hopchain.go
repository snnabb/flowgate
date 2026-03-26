package forwarder

import (
	"fmt"
	"log"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flowgate/flowgate/internal/common"
)

// HopChainForwarder handles TCP forwarding with hop-chain routing and
// per-hop load balancing.  For the first version the entry node forwards
// to the first hop's targets (selected by the hop's LB strategy).
// Subsequent hops are expected to be separate direct-mode rules on
// intermediate nodes configured by the panel.
type HopChainForwarder struct {
	ID         int64
	ListenPort int
	SpeedLimit int // KB/s, 0 = unlimited

	// Tunnel engine (same feature set as TCPForwarder)
	ProxyProtocol int
	BlockedProtos string
	TLSMode       string
	TLSSni        string
	WSEnabled     bool
	WSPath        string

	// Resolved hops (sorted by order)
	hops []resolvedHop

	listener    net.Listener
	trafficIn   int64
	trafficOut  int64
	connections int32
	stopCh      chan struct{}
	running     bool
	mu          sync.Mutex
}

type resolvedHop struct {
	order    int
	targets  []common.RouteTarget
	balancer Balancer
}

// NewHopChainForwarder creates a forwarder from a RuleConfig and parsed hops.
func NewHopChainForwarder(cfg common.RuleConfig, hops []common.RouteHop) *HopChainForwarder {
	tlsMode := cfg.TLSMode
	if tlsMode == "" {
		tlsMode = "none"
	}
	wsPath := cfg.WSPath
	if wsPath == "" {
		wsPath = "/ws"
	}

	// Sort hops by order and create per-hop balancers
	sort.Slice(hops, func(i, j int) bool { return hops[i].Order < hops[j].Order })

	resolved := make([]resolvedHop, len(hops))
	for i, h := range hops {
		resolved[i] = resolvedHop{
			order:    h.Order,
			targets:  h.Targets,
			balancer: NewBalancer(h.LBStrategy, h.Targets),
		}
	}

	return &HopChainForwarder{
		ID:            cfg.ID,
		ListenPort:    cfg.ListenPort,
		SpeedLimit:    cfg.SpeedLimit,
		ProxyProtocol: cfg.ProxyProtocol,
		BlockedProtos: cfg.BlockedProtos,
		TLSMode:       tlsMode,
		TLSSni:        cfg.TLSSni,
		WSEnabled:     cfg.WSEnabled,
		WSPath:        wsPath,
		hops:          resolved,
		stopCh:        make(chan struct{}),
	}
}

// Start begins listening and forwarding.
func (f *HopChainForwarder) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.running {
		return nil
	}
	if len(f.hops) == 0 {
		return fmt.Errorf("hop chain 没有配置跳点")
	}
	if err := common.ValidateTunnelSettings(f.WSEnabled, f.TLSMode); err != nil {
		return err
	}

	listenAddr := fmt.Sprintf("0.0.0.0:%d", f.ListenPort)

	var listener net.Listener
	var err error

	if f.WSEnabled {
		listener, err = NewWSListener(listenAddr, f.WSPath)
		if err != nil {
			return fmt.Errorf("ws listen %s: %w", listenAddr, err)
		}
		log.Printf("[HopChain] Rule %d: WebSocket listener on %s%s", f.ID, listenAddr, f.WSPath)
	} else {
		listener, err = net.Listen("tcp", listenAddr)
		if err != nil {
			return fmt.Errorf("tcp listen %s: %w", listenAddr, err)
		}
	}

	// Inbound TLS termination
	if f.TLSMode == "client" || f.TLSMode == "both" {
		listener, err = NewTLSListener(listener)
		if err != nil {
			listener.Close()
			return fmt.Errorf("tls listener: %w", err)
		}
		log.Printf("[HopChain] Rule %d: TLS termination enabled", f.ID)
	}

	f.listener = listener
	f.running = true
	f.stopCh = make(chan struct{})

	go f.acceptLoop()

	first := f.hops[0]
	log.Printf("[HopChain] Rule %d: :%d -> %d targets (%d hops, lb=%s) started",
		f.ID, f.ListenPort, len(first.targets), len(f.hops),
		describeHopLB(first))
	return nil
}

// Stop stops the forwarder.
func (f *HopChainForwarder) Stop() {
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
	log.Printf("[HopChain] Rule %d stopped", f.ID)
}

// GetAndResetTraffic returns and resets traffic counters.
func (f *HopChainForwarder) GetAndResetTraffic() (in, out int64) {
	in = atomic.SwapInt64(&f.trafficIn, 0)
	out = atomic.SwapInt64(&f.trafficOut, 0)
	return
}

// GetConnections returns active connection count.
func (f *HopChainForwarder) GetConnections() int {
	return int(atomic.LoadInt32(&f.connections))
}

// FirstHopTargets returns the first hop's targets (for latency measurement).
func (f *HopChainForwarder) FirstHopTargets() []common.RouteTarget {
	if len(f.hops) == 0 {
		return nil
	}
	return f.hops[0].targets
}

// -------------------------------------------------------------------------
// internal
// -------------------------------------------------------------------------

func (f *HopChainForwarder) acceptLoop() {
	for {
		select {
		case <-f.stopCh:
			return
		default:
		}

		conn, err := f.listener.Accept()
		if err != nil {
			select {
			case <-f.stopCh:
				return
			default:
				log.Printf("[HopChain] Rule %d accept error: %v", f.ID, err)
				continue
			}
		}

		go f.handleConn(conn)
	}
}

func (f *HopChainForwarder) handleConn(clientConn net.Conn) {
	atomic.AddInt32(&f.connections, 1)
	defer func() {
		atomic.AddInt32(&f.connections, -1)
		clientConn.Close()
	}()

	// --- Protocol detection & blocking ---
	var relayConn net.Conn = clientConn
	if f.BlockedProtos != "" {
		pc, peeked, err := PeekBytes(clientConn, 8)
		if err != nil {
			log.Printf("[HopChain] Rule %d peek failed: %v", f.ID, err)
			return
		}
		proto := DetectProtocol(peeked)
		if IsBlocked(proto, f.BlockedProtos) {
			log.Printf("[HopChain] Rule %d blocked %s from %s", f.ID, proto, clientConn.RemoteAddr())
			return
		}
		relayConn = pc
	}

	// --- Select target from first hop via LB ---
	firstHop := f.hops[0]
	target := firstHop.balancer.Pick(clientConn.RemoteAddr())
	key := TargetKey(target)

	firstHop.balancer.OnConnect(key)
	defer firstHop.balancer.OnDisconnect(key)

	// --- Dial target ---
	start := time.Now()
	serverConn, err := f.dialTarget(target.Host, target.Port)
	if err != nil {
		firstHop.balancer.OnDialError(key)
		log.Printf("[HopChain] Rule %d dial %s failed: %v", f.ID, key, err)
		return
	}
	firstHop.balancer.OnDialSuccess(key, time.Since(start))
	defer serverConn.Close()

	// --- PROXY Protocol header ---
	if f.ProxyProtocol == 1 {
		if err := WriteProxyV1(serverConn, clientConn.RemoteAddr(), clientConn.LocalAddr()); err != nil {
			log.Printf("[HopChain] Rule %d PROXY v1 failed: %v", f.ID, err)
			return
		}
	} else if f.ProxyProtocol == 2 {
		if err := WriteProxyV2(serverConn, clientConn.RemoteAddr(), clientConn.LocalAddr()); err != nil {
			log.Printf("[HopChain] Rule %d PROXY v2 failed: %v", f.ID, err)
			return
		}
	}

	// --- Bidirectional relay ---
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		f.copyWithCounter(serverConn, relayConn, &f.trafficIn)
	}()

	go func() {
		defer wg.Done()
		f.copyWithCounter(relayConn, serverConn, &f.trafficOut)
	}()

	wg.Wait()
}

func (f *HopChainForwarder) dialTarget(host string, port int) (net.Conn, error) {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	if f.TLSMode == "server" || f.TLSMode == "both" {
		return TLSDial(addr, 10*time.Second, f.TLSSni)
	}

	return net.DialTimeout("tcp", addr, 10*time.Second)
}

func (f *HopChainForwarder) copyWithCounter(dst, src net.Conn, counter *int64) int64 {
	if f.SpeedLimit > 0 {
		return f.copyWithSpeedLimit(dst, src, counter)
	}

	buf := make([]byte, 32*1024)
	var total int64
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				total += int64(nw)
				atomic.AddInt64(counter, int64(nw))
			}
			if ew != nil {
				break
			}
		}
		if er != nil {
			break
		}
	}
	return total
}

func (f *HopChainForwarder) copyWithSpeedLimit(dst, src net.Conn, counter *int64) int64 {
	limitBytes := int64(f.SpeedLimit) * 1024
	buf := make([]byte, 32*1024)
	var total int64
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var bytesThisTick int64
	maxPerTick := limitBytes / 10

	for {
		if bytesThisTick >= maxPerTick {
			<-ticker.C
			bytesThisTick = 0
		}

		readSize := int(maxPerTick - bytesThisTick)
		if readSize > len(buf) {
			readSize = len(buf)
		}
		if readSize <= 0 {
			<-ticker.C
			bytesThisTick = 0
			continue
		}

		nr, er := src.Read(buf[:readSize])
		if nr > 0 {
			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				total += int64(nw)
				bytesThisTick += int64(nw)
				atomic.AddInt64(counter, int64(nw))
			}
			if ew != nil {
				break
			}
		}
		if er != nil {
			break
		}
	}
	return total
}

func describeHopLB(h resolvedHop) string {
	if len(h.targets) <= 1 {
		return "single"
	}
	// Report the first hop's LB strategy for logging
	for _, t := range h.targets {
		_ = t // just need balancer type
	}
	return fmt.Sprintf("%T", h.balancer)
}
