package forwarder

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// TCPForwarder handles TCP port forwarding
type TCPForwarder struct {
	ID         int64
	ListenPort int
	TargetAddr string
	TargetPort int
	SpeedLimit int // KB/s, 0 = unlimited

	listener    net.Listener
	trafficIn   int64
	trafficOut  int64
	connections int32
	stopCh      chan struct{}
	running     bool
	mu          sync.Mutex
}

// NewTCPForwarder creates a new TCP forwarder
func NewTCPForwarder(id int64, listenPort int, targetAddr string, targetPort int, speedLimit int) *TCPForwarder {
	return &TCPForwarder{
		ID:         id,
		ListenPort: listenPort,
		TargetAddr: targetAddr,
		TargetPort: targetPort,
		SpeedLimit: speedLimit,
		stopCh:     make(chan struct{}),
	}
}

// Start begins listening and forwarding
func (f *TCPForwarder) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.running {
		return nil
	}

	listenAddr := fmt.Sprintf("0.0.0.0:%d", f.ListenPort)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("tcp listen %s: %w", listenAddr, err)
	}

	f.listener = listener
	f.running = true
	f.stopCh = make(chan struct{})

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

	targetAddr := fmt.Sprintf("%s:%d", f.TargetAddr, f.TargetPort)
	serverConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		log.Printf("[TCP] Rule %d connect to %s failed: %v", f.ID, targetAddr, err)
		return
	}
	defer serverConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Server (upload/in)
	go func() {
		defer wg.Done()
		n := f.copyWithCounter(serverConn, clientConn, &f.trafficIn)
		_ = n
	}()

	// Server -> Client (download/out)
	go func() {
		defer wg.Done()
		n := f.copyWithCounter(clientConn, serverConn, &f.trafficOut)
		_ = n
	}()

	wg.Wait()
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
