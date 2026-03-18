package hub

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/flowgate/flowgate/internal/common"
	"github.com/flowgate/flowgate/internal/panel/db"
)

// NodeConn represents a connected node
type NodeConn struct {
	NodeID int64
	Conn   *websocket.Conn
	Send   chan []byte
}

// Hub manages all node WebSocket connections
type Hub struct {
	mu    sync.RWMutex
	nodes map[int64]*NodeConn
	DB    *db.Database
}

// New creates a new Hub
func New(database *db.Database) *Hub {
	return &Hub{
		nodes: make(map[int64]*NodeConn),
		DB:    database,
	}
}

// Register adds a node connection
func (h *Hub) Register(nodeID int64, conn *websocket.Conn) *NodeConn {
	nc := &NodeConn{
		NodeID: nodeID,
		Conn:   conn,
		Send:   make(chan []byte, 256),
	}

	h.mu.Lock()
	// Close existing connection if any
	if old, ok := h.nodes[nodeID]; ok {
		close(old.Send)
		old.Conn.Close()
	}
	h.nodes[nodeID] = nc
	h.mu.Unlock()

	h.DB.UpdateNodeStatus(nodeID, "online", conn.RemoteAddr().String(), 0, 0, 0)
	h.DB.UpdateNodeRuleStatuses(nodeID, "pending", "节点已连接，等待规则确认")
	log.Printf("[Hub] Node %d registered from %s", nodeID, conn.RemoteAddr())

	return nc
}

// Unregister removes a node connection
func (h *Hub) Unregister(nc *NodeConn) {
	if nc == nil {
		return
	}

	removed := false

	h.mu.Lock()
	if current, ok := h.nodes[nc.NodeID]; ok && current == nc {
		close(nc.Send)
		nc.Conn.Close()
		delete(h.nodes, nc.NodeID)
		removed = true
	}
	h.mu.Unlock()

	if removed {
		h.DB.SetNodeOffline(nc.NodeID)
		h.DB.UpdateNodeRuleStatuses(nc.NodeID, "offline", "节点已离线，等待重新连接")
		log.Printf("[Hub] Node %d unregistered", nc.NodeID)
	}
}

// SendToNode sends a message to a specific node
func (h *Hub) SendToNode(nodeID int64, msg common.WSMessage) error {
	h.mu.RLock()
	nc, ok := h.nodes[nodeID]
	h.mu.RUnlock()

	if !ok {
		return nil // Node not connected, skip
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	select {
	case nc.Send <- data:
	default:
		log.Printf("[Hub] Node %d send buffer full, dropping message", nodeID)
	}
	return nil
}

// SendRuleToNode sends a rule config to a node
func (h *Hub) SendRuleToNode(nodeID int64, action string, rule common.RuleConfig) error {
	msg := common.NewMessage(common.MsgTypeCommand, action, rule)
	return h.SendToNode(nodeID, msg)
}

// SyncRulesToNode sends all rules to a node
func (h *Hub) SyncRulesToNode(nodeID int64) error {
	rules, err := h.DB.ListRules(nodeID)
	if err != nil {
		return err
	}

	var configs []common.RuleConfig
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		configs = append(configs, common.RuleConfig{
			ID:         r.ID,
			Protocol:   r.Protocol,
			ListenPort: r.ListenPort,
			TargetAddr: r.TargetAddr,
			TargetPort: r.TargetPort,
			SpeedLimit: r.SpeedLimit,
			Enabled:    r.Enabled,
		})
	}

	msg := common.NewMessage(common.MsgTypeCommand, common.ActionSyncRules, configs)
	return h.SendToNode(nodeID, msg)
}

// WritePump writes messages to the node WebSocket
func (h *Hub) WritePump(nc *NodeConn) {
	ticker := time.NewTicker(15 * time.Second)
	defer func() {
		ticker.Stop()
		nc.Conn.Close()
		h.Unregister(nc)
	}()

	for {
		select {
		case data, ok := <-nc.Send:
			if !ok {
				nc.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			nc.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := nc.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}

		case <-ticker.C:
			nc.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			msg := common.NewMessage(common.MsgTypeHeartbeat, "", nil)
			data, _ := json.Marshal(msg)
			if err := nc.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	}
}

// ReadPump reads messages from the node WebSocket
func (h *Hub) ReadPump(nc *NodeConn) {
	defer h.Unregister(nc)

	nc.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	nc.Conn.SetPongHandler(func(string) error {
		nc.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, data, err := nc.Conn.ReadMessage()
		if err != nil {
			log.Printf("[Hub] Node %d read error: %v", nc.NodeID, err)
			return
		}
		nc.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		var msg common.WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		h.handleNodeMessage(nc.NodeID, &msg)
	}
}

func (h *Hub) handleNodeMessage(nodeID int64, msg *common.WSMessage) {
	switch msg.Action {
	case common.ActionReportStatus:
		data, _ := json.Marshal(msg.Data)
		var status common.NodeStatus
		json.Unmarshal(data, &status)

		h.mu.RLock()
		nc, ok := h.nodes[nodeID]
		h.mu.RUnlock()

		ipAddr := ""
		if ok {
			ipAddr = nc.Conn.RemoteAddr().String()
		}
		h.DB.UpdateNodeStatus(nodeID, "online", ipAddr, status.CPUUsage, status.MemUsage, status.MemTotal)

	case common.ActionReportStats:
		data, _ := json.Marshal(msg.Data)
		var reports []common.TrafficReport
		json.Unmarshal(data, &reports)

		for _, r := range reports {
			if err := h.DB.UpdateRuleTraffic(r.RuleID, r.TrafficIn, r.TrafficOut); err != nil {
				log.Printf("[Hub] Failed to update rule %d traffic: %v", r.RuleID, err)
			}
			if err := h.DB.InsertTrafficLog(r.RuleID, nodeID, r.TrafficIn, r.TrafficOut); err != nil {
				log.Printf("[Hub] Failed to insert traffic log for rule %d: %v", r.RuleID, err)
			}
		}

	case common.ActionReportRuleStatus:
		data, _ := json.Marshal(msg.Data)
		var report common.RuleStatusReport
		json.Unmarshal(data, &report)

		if err := h.DB.UpdateRuleRuntimeStatus(report.RuleID, report.Status, report.Message); err != nil {
			log.Printf("[Hub] Failed to update rule %d runtime status: %v", report.RuleID, err)
		}
	}
}

// IsNodeOnline checks if a node is connected
func (h *Hub) IsNodeOnline(nodeID int64) bool {
	h.mu.RLock()
	_, ok := h.nodes[nodeID]
	h.mu.RUnlock()
	return ok
}

// GetOnlineNodeIDs returns all online node IDs
func (h *Hub) GetOnlineNodeIDs() []int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ids := make([]int64, 0, len(h.nodes))
	for id := range h.nodes {
		ids = append(ids, id)
	}
	return ids
}
