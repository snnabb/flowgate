package hub

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// PanelClient represents a connected browser client
type PanelClient struct {
	Conn *websocket.Conn
	Send chan []byte
}

// PanelHub manages browser WebSocket connections for real-time push
type PanelHub struct {
	mu      sync.RWMutex
	clients map[*PanelClient]bool

	// Debounce: coalesce rapid changes into a single broadcast
	notifyCh chan struct{}
}

// NewPanelHub creates a new PanelHub
func NewPanelHub() *PanelHub {
	ph := &PanelHub{
		clients:  make(map[*PanelClient]bool),
		notifyCh: make(chan struct{}, 1),
	}
	go ph.debounceLoop()
	return ph
}

// Register adds a browser client
func (h *PanelHub) Register(conn *websocket.Conn) *PanelClient {
	client := &PanelClient{
		Conn: conn,
		Send: make(chan []byte, 32),
	}
	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()
	log.Printf("[PanelHub] Client connected (%d total)", h.ClientCount())
	return client
}

// Unregister removes a browser client
func (h *PanelHub) Unregister(client *PanelClient) {
	h.mu.Lock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.Send)
	}
	h.mu.Unlock()
}

// ClientCount returns the number of connected clients
func (h *PanelHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// NotifyChange signals that dashboard data has changed.
// Debounced: multiple rapid calls result in a single broadcast.
func (h *PanelHub) NotifyChange() {
	select {
	case h.notifyCh <- struct{}{}:
	default:
		// Already pending
	}
}

func (h *PanelHub) debounceLoop() {
	for range h.notifyCh {
		// Wait briefly to coalesce rapid changes
		time.Sleep(500 * time.Millisecond)
		// Drain any extra signals that arrived during the wait
		select {
		case <-h.notifyCh:
		default:
		}
		h.broadcast(map[string]interface{}{"type": "reload"})
	}
}

func (h *PanelHub) broadcast(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		select {
		case client.Send <- data:
		default:
			// Buffer full, skip
		}
	}
}

// WritePump sends messages to a browser client
func (h *PanelHub) WritePump(client *PanelClient) {
	ticker := time.NewTicker(20 * time.Second)
	defer func() {
		ticker.Stop()
		client.Conn.Close()
		h.Unregister(client)
	}()

	for {
		select {
		case data, ok := <-client.Send:
			if !ok {
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			client.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ReadPump reads messages from a browser client (just keeps connection alive)
func (h *PanelHub) ReadPump(client *PanelClient) {
	defer h.Unregister(client)
	client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.Conn.SetPongHandler(func(string) error {
		client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := client.Conn.ReadMessage()
		if err != nil {
			return
		}
		client.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	}
}
