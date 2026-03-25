package forwarder

import (
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// WSListener implements net.Listener over WebSocket.
// Clients connect via HTTP(S) and upgrade to WebSocket; each upgraded connection
// is wrapped as a net.Conn and returned from Accept().
type WSListener struct {
	addr     net.Addr
	connCh   chan net.Conn
	server   *http.Server
	listener net.Listener // underlying TCP listener, kept for Addr()
	closeCh  chan struct{}
	once     sync.Once
}

// NewWSListener creates a WebSocket listener on listenAddr (e.g. "0.0.0.0:8080")
// accepting WebSocket upgrades on the given path (e.g. "/ws").
func NewWSListener(listenAddr, path string) (*WSListener, error) {
	tcpLn, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	wsl := &WSListener{
		addr:     tcpLn.Addr(),
		connCh:   make(chan net.Conn, 256),
		listener: tcpLn,
		closeCh:  make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc(path, wsl.handleUpgrade)

	wsl.server = &http.Server{
		Handler:     mux,
		ReadTimeout: 30 * time.Second,
	}

	go func() {
		if err := wsl.server.Serve(tcpLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[WSListener] Serve error: %v", err)
		}
	}()

	return wsl, nil
}

func (wsl *WSListener) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	wsConn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WSListener] Upgrade failed: %v", err)
		return
	}

	conn := newWSConn(wsConn)

	select {
	case wsl.connCh <- conn:
	case <-wsl.closeCh:
		conn.Close()
	}
}

// Accept waits for and returns the next WebSocket connection.
func (wsl *WSListener) Accept() (net.Conn, error) {
	select {
	case conn := <-wsl.connCh:
		return conn, nil
	case <-wsl.closeCh:
		return nil, errors.New("listener closed")
	}
}

// Close shuts down the WebSocket listener.
func (wsl *WSListener) Close() error {
	wsl.once.Do(func() {
		close(wsl.closeCh)
		wsl.server.Close()
	})
	return nil
}

// Addr returns the listener's network address.
func (wsl *WSListener) Addr() net.Addr {
	return wsl.addr
}

// wsConn adapts a *websocket.Conn to the net.Conn interface.
type wsConn struct {
	ws     *websocket.Conn
	reader io.Reader // current message reader for partial reads
	mu     sync.Mutex
}

func newWSConn(ws *websocket.Conn) *wsConn {
	return &wsConn{ws: ws}
}

func (c *wsConn) Read(p []byte) (int, error) {
	// If we have an ongoing message reader, continue reading from it
	if c.reader != nil {
		n, err := c.reader.Read(p)
		if err == io.EOF {
			c.reader = nil
			if n > 0 {
				return n, nil
			}
			// Fall through to get next message
		} else {
			return n, err
		}
	}

	// Get next message
	_, reader, err := c.ws.NextReader()
	if err != nil {
		return 0, err
	}
	c.reader = reader
	return c.reader.Read(p)
}

func (c *wsConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.ws.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *wsConn) Close() error {
	return c.ws.Close()
}

func (c *wsConn) LocalAddr() net.Addr {
	return c.ws.LocalAddr()
}

func (c *wsConn) RemoteAddr() net.Addr {
	return c.ws.RemoteAddr()
}

func (c *wsConn) SetDeadline(t time.Time) error {
	if err := c.ws.SetReadDeadline(t); err != nil {
		return err
	}
	return c.ws.SetWriteDeadline(t)
}

func (c *wsConn) SetReadDeadline(t time.Time) error {
	return c.ws.SetReadDeadline(t)
}

func (c *wsConn) SetWriteDeadline(t time.Time) error {
	return c.ws.SetWriteDeadline(t)
}
