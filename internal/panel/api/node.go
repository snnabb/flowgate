package api

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/flowgate/flowgate/internal/panel/db"
	"github.com/flowgate/flowgate/internal/panel/hub"
	"github.com/flowgate/flowgate/internal/panel/model"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type NodeHandler struct {
	DB  *db.Database
	Hub *hub.Hub
}

// ListNodes returns all nodes
func (h *NodeHandler) ListNodes(c *gin.Context) {
	nodes, err := h.DB.ListNodesVisibleTo(currentUser(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if nodes == nil {
		nodes = []model.Node{}
	}
	c.JSON(http.StatusOK, gin.H{"nodes": nodes})
}

// CreateNode creates a new node
func (h *NodeHandler) CreateNode(c *gin.Context) {
	var req model.CreateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "鑺傜偣鍚嶇О涓嶈兘涓虹┖"})
		return
	}

	owner, err := resolvedOwnerUser(h.DB, currentUser(c), req.OwnerUserID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	node, err := h.DB.CreateNodeWithOwner(&req, owner.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	actor := c.GetString("username")
	details := actor + " 娣诲姞浜嗚妭鐐?" + node.Name
	if node.GroupName != "" {
		details += " 鍒板垎缁?" + node.GroupName
	}
	_ = h.DB.CreateEvent("node", "鑺傜偣宸插垱寤?", details)
	h.Hub.PanelHub.NotifyChange()
	c.JSON(http.StatusOK, gin.H{"node": node})
}

// GetNode returns a single node
func (h *NodeHandler) GetNode(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	node, err := h.DB.GetNodeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "鑺傜偣涓嶅瓨鍦?"})
		return
	}

	allowed, err := canAccessOwner(h.DB, currentUser(c), node.OwnerUserID)
	if err != nil || !allowed {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"node": node})
}

// DeleteNode deletes a node
func (h *NodeHandler) DeleteNode(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	node, err := h.DB.GetNodeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "鑺傜偣涓嶅瓨鍦?"})
		return
	}

	allowed, err := canAccessOwner(h.DB, currentUser(c), node.OwnerUserID)
	if err != nil || !allowed {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	// Disconnect the node's WebSocket before deleting
	h.Hub.DisconnectNode(id)

	if err := h.DB.DeleteNode(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	actor := c.GetString("username")
	_ = h.DB.CreateEvent("node", "鑺傜偣宸插垹闄?", actor+" 鍒犻櫎浜嗚妭鐐?"+node.Name)
	h.Hub.PanelHub.NotifyChange()
	c.JSON(http.StatusOK, gin.H{"message": "鑺傜偣宸插垹闄?"})
}

// HandleNodeWS handles WebSocket connections from nodes
func (h *NodeHandler) HandleNodeWS(c *gin.Context) {
	apiKey := c.Query("key")
	if apiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "闇€瑕?API 瀵嗛挜"})
		return
	}

	node, err := h.DB.GetNodeByAPIKey(apiKey)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "鏃犳晥鐨?API 瀵嗛挜"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WS] Upgrade failed: %v", err)
		return
	}

	nc := h.Hub.Register(node.ID, conn)

	// Sync all rules to the newly connected node
	h.Hub.SyncRulesToNode(node.ID)

	go h.Hub.WritePump(nc)
	h.Hub.ReadPump(nc) // blocks until disconnect
}
