package forwarder

import (
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// pooledConn wraps a net.Conn with the time it was put into the pool.
type pooledConn struct {
	net.Conn
	idleSince time.Time
}

// ConnPool maintains a pool of pre-established connections to a target.
type ConnPool struct {
	maxSize     int
	idleTimeout time.Duration
	dialFunc    func() (net.Conn, error)

	pool   chan *pooledConn
	stopCh chan struct{}
	once   sync.Once
}

// NewConnPool creates a connection pool. dialFunc is called to establish new connections.
func NewConnPool(maxSize int, idleTimeout time.Duration, dialFunc func() (net.Conn, error)) *ConnPool {
	if idleTimeout <= 0 {
		idleTimeout = 60 * time.Second
	}
	return &ConnPool{
		maxSize:     maxSize,
		idleTimeout: idleTimeout,
		dialFunc:    dialFunc,
		pool:        make(chan *pooledConn, maxSize),
		stopCh:      make(chan struct{}),
	}
}

// Start pre-fills the pool and starts the health check loop.
func (p *ConnPool) Start() {
	// Pre-fill in background
	go func() {
		for i := 0; i < p.maxSize; i++ {
			select {
			case <-p.stopCh:
				return
			default:
			}
			conn, err := p.dialFunc()
			if err != nil {
				log.Printf("[ConnPool] Pre-fill dial failed: %v", err)
				continue
			}
			pc := &pooledConn{Conn: conn, idleSince: time.Now()}
			select {
			case p.pool <- pc:
			default:
				conn.Close()
			}
		}
		log.Printf("[ConnPool] Pre-filled %d/%d connections", len(p.pool), p.maxSize)
	}()

	// Health check / refill loop
	go p.maintainLoop()
}

// Get returns a connection from the pool or dials a new one.
func (p *ConnPool) Get() (net.Conn, error) {
	for {
		select {
		case pc := <-p.pool:
			if pc == nil {
				continue
			}
			// Check idle timeout
			if time.Since(pc.idleSince) > p.idleTimeout {
				pc.Close()
				continue
			}
			// Probe pooled connection health. This may consume a pending byte.
			_ = pc.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
			var probe [1]byte
			_, probeErr := pc.Read(probe[:])
			_ = pc.SetReadDeadline(time.Time{})
			if probeErr != nil {
				if ne, ok := probeErr.(net.Error); ok && ne.Timeout() {
					return pc.Conn, nil
				}
				if probeErr == io.EOF {
					pc.Close()
					continue
				}
				pc.Close()
				continue
			}
			pc.Close()
			continue
		default:
			// Pool empty, dial new
			return p.dialFunc()
		}
	}
}

// Stop drains and closes all pooled connections.
func (p *ConnPool) Stop() {
	p.once.Do(func() {
		close(p.stopCh)
		// Drain pool
		for {
			select {
			case pc := <-p.pool:
				pc.Close()
			default:
				return
			}
		}
	})
}

func (p *ConnPool) maintainLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.evictStale()
			p.refill()
		}
	}
}

func (p *ConnPool) evictStale() {
	n := len(p.pool)
	for i := 0; i < n; i++ {
		select {
		case pc := <-p.pool:
			if time.Since(pc.idleSince) > p.idleTimeout {
				pc.Close()
				continue
			}
			// Put back
			select {
			case p.pool <- pc:
			default:
				pc.Close()
			}
		default:
			return
		}
	}
}

func (p *ConnPool) refill() {
	deficit := p.maxSize - len(p.pool)
	for i := 0; i < deficit; i++ {
		select {
		case <-p.stopCh:
			return
		default:
		}
		conn, err := p.dialFunc()
		if err != nil {
			return // stop refilling on first error
		}
		pc := &pooledConn{Conn: conn, idleSince: time.Now()}
		select {
		case p.pool <- pc:
		default:
			conn.Close()
			return
		}
	}
}
