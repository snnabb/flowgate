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

// ListNodes returns nodes visible to the current actor.
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

// CreateNode creates a shared resource-pool node.
func (h *NodeHandler) CreateNode(c *gin.Context) {
	var req model.CreateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	req.OwnerUserID = nil
	node, err := h.DB.CreateNodeWithOwner(&req, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	actor := c.GetString("username")
	_ = h.DB.CreateEvent("node", "Node created", actor+" created "+node.Name)
	h.Hub.PanelHub.NotifyChange()
	c.JSON(http.StatusOK, gin.H{"node": node})
}

// GetNode returns a single node if it is visible to the current actor.
func (h *NodeHandler) GetNode(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	node, err := h.DB.GetNodeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	allowed, _, err := canUseNode(h.DB, currentUser(c), node.ID)
	if err != nil || !allowed {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"node": node})
}

// DeleteNode deletes a node.
func (h *NodeHandler) DeleteNode(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	node, err := h.DB.GetNodeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	h.Hub.DisconnectNode(id)
	if err := h.DB.DeleteNode(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	actor := c.GetString("username")
	_ = h.DB.CreateEvent("node", "Node deleted", actor+" deleted "+node.Name)
	h.Hub.PanelHub.NotifyChange()
	c.JSON(http.StatusOK, gin.H{"message": "node deleted"})
}

// HandleNodeWS handles WebSocket connections from nodes.
func (h *NodeHandler) HandleNodeWS(c *gin.Context) {
	apiKey := c.Query("key")
	if apiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing API key"})
		return
	}

	node, err := h.DB.GetNodeByAPIKey(apiKey)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WS] Upgrade failed: %v", err)
		return
	}

	nc := h.Hub.Register(node.ID, conn)
	h.Hub.SyncRulesToNode(node.ID)

	go h.Hub.WritePump(nc)
	h.Hub.ReadPump(nc)
}
