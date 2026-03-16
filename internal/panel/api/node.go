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
	nodes, err := h.DB.ListNodes()
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name is required"})
		return
	}

	node, err := h.DB.CreateNode(req.Name, req.GroupName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"node": node})
}

// GetNode returns a single node
func (h *NodeHandler) GetNode(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	node, err := h.DB.GetNodeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Node not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"node": node})
}

// DeleteNode deletes a node
func (h *NodeHandler) DeleteNode(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.DB.DeleteNode(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Node deleted"})
}

// HandleNodeWS handles WebSocket connections from nodes
func (h *NodeHandler) HandleNodeWS(c *gin.Context) {
	apiKey := c.Query("key")
	if apiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "API key required"})
		return
	}

	node, err := h.DB.GetNodeByAPIKey(apiKey)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
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
