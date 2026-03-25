package forwarder

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flowgate/flowgate/internal/common"
)

// TCPForwarder handles TCP port forwarding with tunnel engine features.
type TCPForwarder struct {
	ID         int64
	ListenPort int
	TargetAddr string
	TargetPort int
	SpeedLimit int // KB/s, 0 = unlimited

	// Tunnel engine
	ProxyProtocol int    // 0=off, 1=v1, 2=v2
	BlockedProtos string // comma-separated blocked protocols
	PoolSize      int    // connection pool size, 0=disabled
	TLSMode       string // none/client/server/both
	TLSSni        string // SNI for outbound TLS
	WSEnabled     bool   // WebSocket listener mode
	WSPath        string // WebSocket path

	listener    net.Listener
	pool        *ConnPool
	trafficIn   int64
	trafficOut  int64
	connections int32
	stopCh      chan struct{}
	running     bool
	mu          sync.Mutex
}

// NewTCPForwarder creates a new TCP forwarder from a RuleConfig.
func NewTCPForwarder(cfg common.RuleConfig) *TCPForwarder {
	tlsMode := cfg.TLSMode
	if tlsMode == "" {
		tlsMode = "none"
	}
	wsPath := cfg.WSPath
	if wsPath == "" {
		wsPath = "/ws"
	}
	return &TCPForwarder{
		ID:            cfg.ID,
		ListenPort:    cfg.ListenPort,
		TargetAddr:    cfg.TargetAddr,
		TargetPort:    cfg.TargetPort,
		SpeedLimit:    cfg.SpeedLimit,
		ProxyProtocol: cfg.ProxyProtocol,
		BlockedProtos: cfg.BlockedProtos,
		PoolSize:      cfg.PoolSize,
		TLSMode:       tlsMode,
		TLSSni:        cfg.TLSSni,
		WSEnabled:     cfg.WSEnabled,
		WSPath:        wsPath,
		stopCh:        make(chan struct{}),
	}
}

// Start begins listening and forwarding
func (f *TCPForwarder) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.running {
		return nil
	}
	if err := common.ValidateTunnelSettings(f.WSEnabled, f.TLSMode); err != nil {
		return err
	}

	listenAddr := fmt.Sprintf("0.0.0.0:%d", f.ListenPort)

	var listener net.Listener
	var err error

	if f.WSEnabled {
		// WebSocket listener mode
		listener, err = NewWSListener(listenAddr, f.WSPath)
		if err != nil {
			return fmt.Errorf("ws listen %s: %w", listenAddr, err)
		}
		log.Printf("[TCP] Rule %d: WebSocket listener on %s%s", f.ID, listenAddr, f.WSPath)
	} else {
		listener, err = net.Listen("tcp", listenAddr)
		if err != nil {
			return fmt.Errorf("tcp listen %s: %w", listenAddr, err)
		}
	}

	// Wrap with TLS if client-side TLS is enabled
	if f.TLSMode == "client" || f.TLSMode == "both" {
		listener, err = NewTLSListener(listener)
		if err != nil {
			listener.Close()
			return fmt.Errorf("tls listener: %w", err)
		}
		log.Printf("[TCP] Rule %d: TLS termination enabled", f.ID)
	}

	f.listener = listener
	f.running = true
	f.stopCh = make(chan struct{})

	// Start connection pool if configured
	if f.PoolSize > 0 {
		dialFunc := f.makeDialFunc()
		f.pool = NewConnPool(f.PoolSize, 60*time.Second, dialFunc)
		f.pool.Start()
		log.Printf("[TCP] Rule %d: connection pool size=%d", f.ID, f.PoolSize)
	}

	go f.acceptLoop()

	log.Printf("[TCP] Rule %d: %s -> %s:%d started", f.ID, listenAddr, f.TargetAddr, f.TargetPort)
	return nil
}

// Stop stops the forwarder
func (f *TCPForwarder) Stop() {
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
	if f.pool != nil {
		f.pool.Stop()
		f.pool = nil
	}

	log.Printf("[TCP] Rule %d stopped", f.ID)
}

// GetAndResetTraffic returns and resets traffic counters
func (f *TCPForwarder) GetAndResetTraffic() (in, out int64) {
	in = atomic.SwapInt64(&f.trafficIn, 0)
	out = atomic.SwapInt64(&f.trafficOut, 0)
	return
}

// GetConnections returns active connection count
func (f *TCPForwarder) GetConnections() int {
	return int(atomic.LoadInt32(&f.connections))
}

func (f *TCPForwarder) acceptLoop() {
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
				log.Printf("[TCP] Rule %d accept error: %v", f.ID, err)
				continue
			}
		}

		go f.handleConn(conn)
	}
}

func (f *TCPForwarder) handleConn(clientConn net.Conn) {
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
			log.Printf("[TCP] Rule %d peek failed: %v", f.ID, err)
			return
		}
		proto := DetectProtocol(peeked)
		if IsBlocked(proto, f.BlockedProtos) {
			log.Printf("[TCP] Rule %d blocked %s connection from %s", f.ID, proto, clientConn.RemoteAddr())
			return
		}
		relayConn = pc // use PeekConn which replays peeked bytes
	}

	// --- Dial target (with pool, TLS) ---
	var serverConn net.Conn
	var err error

	if f.pool != nil {
		serverConn, err = f.pool.Get()
	} else {
		serverConn, err = f.dialTarget()
	}
	if err != nil {
		log.Printf("[TCP] Rule %d connect to %s:%d failed: %v", f.ID, f.TargetAddr, f.TargetPort, err)
		return
	}
	defer serverConn.Close()

	// --- PROXY Protocol header ---
	if f.ProxyProtocol == 1 {
		if err := WriteProxyV1(serverConn, clientConn.RemoteAddr(), clientConn.LocalAddr()); err != nil {
			log.Printf("[TCP] Rule %d PROXY v1 header failed: %v", f.ID, err)
			return
		}
	} else if f.ProxyProtocol == 2 {
		if err := WriteProxyV2(serverConn, clientConn.RemoteAddr(), clientConn.LocalAddr()); err != nil {
			log.Printf("[TCP] Rule %d PROXY v2 header failed: %v", f.ID, err)
			return
		}
	}

	// --- Bidirectional relay ---
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Server (upload/in)
	go func() {
		defer wg.Done()
		f.copyWithCounter(serverConn, relayConn, &f.trafficIn)
	}()

	// Server -> Client (download/out)
	go func() {
		defer wg.Done()
		f.copyWithCounter(relayConn, serverConn, &f.trafficOut)
	}()

	wg.Wait()
}

// makeDialFunc returns a dial function that respects TLS settings.
func (f *TCPForwarder) makeDialFunc() func() (net.Conn, error) {
	return func() (net.Conn, error) {
		return f.dialTarget()
	}
}

// dialTarget connects to the target, optionally over TLS.
func (f *TCPForwarder) dialTarget() (net.Conn, error) {
	targetAddr := net.JoinHostPort(f.TargetAddr, fmt.Sprintf("%d", f.TargetPort))

	if f.TLSMode == "server" || f.TLSMode == "both" {
		return TLSDial(targetAddr, 10*time.Second, f.TLSSni)
	}

	return net.DialTimeout("tcp", targetAddr, 10*time.Second)
}

func (f *TCPForwarder) copyWithCounter(dst, src net.Conn, counter *int64) int64 {
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

func (f *TCPForwarder) copyWithSpeedLimit(dst, src net.Conn, counter *int64) int64 {
	limitBytes := int64(f.SpeedLimit) * 1024 // KB/s -> bytes/s
	buf := make([]byte, 32*1024)
	var total int64
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var bytesThisTick int64
	maxPerTick := limitBytes / 10 // 100ms intervals

	for {
		// Apply speed limit
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
			if er != io.EOF {
				// Network error
			}
			break
		}
	}
	return total
}
