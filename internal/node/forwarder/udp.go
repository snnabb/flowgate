package forwarder

import (
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// UDPForwarder handles UDP port forwarding
type UDPForwarder struct {
	ID         int64
	ListenPort int
	TargetAddr string
	TargetPort int
	SpeedLimit int

	conn       *net.UDPConn
	trafficIn  int64
	trafficOut int64
	stopCh     chan struct{}
	running    bool
	mu         sync.Mutex
	inLimiter  *udpRateLimiter
	outLimiter *udpRateLimiter

	// NAT table: maps client addr -> upstream conn
	natTable   map[string]*udpNATEntry
	natMu      sync.RWMutex
}

type udpNATEntry struct {
	clientAddr *net.UDPAddr
	upstream   *net.UDPConn
	lastActive time.Time
}

type udpRateLimiter struct {
	bytesPerSec int64
	nextAllowed time.Time
	mu          sync.Mutex
}

// NewUDPForwarder creates a new UDP forwarder
func NewUDPForwarder(id int64, listenPort int, targetAddr string, targetPort int, speedLimit int) *UDPForwarder {
	return &UDPForwarder{
		ID:         id,
		ListenPort: listenPort,
		TargetAddr: targetAddr,
		TargetPort: targetPort,
		SpeedLimit: speedLimit,
		inLimiter:  newUDPRateLimiter(speedLimit),
		outLimiter: newUDPRateLimiter(speedLimit),
		natTable:   make(map[string]*udpNATEntry),
		stopCh:     make(chan struct{}),
	}
}

// Start begins listening and forwarding UDP packets
func (f *UDPForwarder) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.running {
		return nil
	}

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("0.0.0.0:%d", f.ListenPort))
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("udp listen :%d: %w", f.ListenPort, err)
	}

	f.conn = conn
	f.running = true
	f.stopCh = make(chan struct{})

	go f.readLoop()
	go f.cleanupLoop()

	log.Printf("[UDP] Rule %d: :%d -> %s:%d started", f.ID, f.ListenPort, f.TargetAddr, f.TargetPort)
	return nil
}

// Stop stops the forwarder
func (f *UDPForwarder) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.running {
		return
	}

	f.running = false
	close(f.stopCh)
	if f.conn != nil {
		f.conn.Close()
	}

	// Close all NAT entries
	f.natMu.Lock()
	for _, entry := range f.natTable {
		entry.upstream.Close()
	}
	f.natTable = make(map[string]*udpNATEntry)
	f.natMu.Unlock()

	log.Printf("[UDP] Rule %d stopped", f.ID)
}

// GetAndResetTraffic returns and resets traffic counters
func (f *UDPForwarder) GetAndResetTraffic() (in, out int64) {
	in = atomic.SwapInt64(&f.trafficIn, 0)
	out = atomic.SwapInt64(&f.trafficOut, 0)
	return
}

func newUDPRateLimiter(speedLimitKB int) *udpRateLimiter {
	if speedLimitKB <= 0 {
		return nil
	}
	return &udpRateLimiter{
		bytesPerSec: int64(speedLimitKB) * 1024,
	}
}

func (l *udpRateLimiter) Wait(size int) {
	if l == nil || size <= 0 {
		return
	}

	delay := time.Second * time.Duration(size) / time.Duration(l.bytesPerSec)

	l.mu.Lock()
	now := time.Now()
	if l.nextAllowed.Before(now) {
		l.nextAllowed = now
	}
	l.nextAllowed = l.nextAllowed.Add(delay)
	sleepFor := l.nextAllowed.Sub(now)
	l.mu.Unlock()

	if sleepFor > 0 {
		time.Sleep(sleepFor)
	}
}

func (f *UDPForwarder) readLoop() {
	buf := make([]byte, 65535)
	for {
		select {
		case <-f.stopCh:
			return
		default:
		}

		f.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, clientAddr, err := f.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-f.stopCh:
				return
			default:
				continue
			}
		}

		atomic.AddInt64(&f.trafficIn, int64(n))

		key := clientAddr.String()
		f.natMu.RLock()
		entry, exists := f.natTable[key]
		f.natMu.RUnlock()

		if !exists {
			entry, err = f.createNATEntry(clientAddr)
			if err != nil {
				log.Printf("[UDP] Rule %d: failed to create NAT for %s: %v", f.ID, key, err)
				continue
			}
		}

		entry.lastActive = time.Now()
		f.inLimiter.Wait(n)
		entry.upstream.Write(buf[:n])
	}
}

func (f *UDPForwarder) createNATEntry(clientAddr *net.UDPAddr) (*udpNATEntry, error) {
	targetUDP, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", f.TargetAddr, f.TargetPort))
	if err != nil {
		return nil, err
	}

	upstream, err := net.DialUDP("udp", nil, targetUDP)
	if err != nil {
		return nil, err
	}

	entry := &udpNATEntry{
		clientAddr: clientAddr,
		upstream:   upstream,
		lastActive: time.Now(),
	}

	key := clientAddr.String()
	f.natMu.Lock()
	f.natTable[key] = entry
	f.natMu.Unlock()

	// Start reverse read goroutine (target -> client)
	go f.reverseRead(key, entry)

	return entry, nil
}

func (f *UDPForwarder) reverseRead(key string, entry *udpNATEntry) {
	buf := make([]byte, 65535)
	for {
		select {
		case <-f.stopCh:
			return
		default:
		}

		entry.upstream.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := entry.upstream.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Check if entry is still active
				if time.Since(entry.lastActive) > 2*time.Minute {
					f.natMu.Lock()
					delete(f.natTable, key)
					f.natMu.Unlock()
					entry.upstream.Close()
					return
				}
				continue
			}
			return
		}

		entry.lastActive = time.Now()
		atomic.AddInt64(&f.trafficOut, int64(n))
		f.outLimiter.Wait(n)
		f.conn.WriteToUDP(buf[:n], entry.clientAddr)
	}
}

func (f *UDPForwarder) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-f.stopCh:
			return
		case <-ticker.C:
			f.natMu.Lock()
			for key, entry := range f.natTable {
				if time.Since(entry.lastActive) > 2*time.Minute {
					entry.upstream.Close()
					delete(f.natTable, key)
				}
			}
			f.natMu.Unlock()
		}
	}
}
