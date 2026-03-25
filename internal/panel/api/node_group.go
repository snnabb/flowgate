package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/flowgate/flowgate/internal/panel/db"
	"github.com/flowgate/flowgate/internal/panel/hub"
	"github.com/flowgate/flowgate/internal/panel/model"
)

type NodeGroupHandler struct {
	DB  *db.Database
	Hub *hub.Hub
}

// ListNodeGroups returns all configured node groups.
func (h *NodeGroupHandler) ListNodeGroups(c *gin.Context) {
	groups, err := h.DB.ListNodeGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if groups == nil {
		groups = []model.NodeGroup{}
	}
	c.JSON(http.StatusOK, gin.H{"node_groups": groups})
}

// CreateNodeGroup creates a reusable node group.
func (h *NodeGroupHandler) CreateNodeGroup(c *gin.Context) {
	var req model.CreateNodeGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的分组请求"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分组名称不能为空"})
		return
	}

	group, err := h.DB.CreateNodeGroup(req.Name, req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	actor := c.GetString("username")
	_ = h.DB.CreateEvent("node_group", "节点分组已创建", actor+" 创建了分组 "+group.Name)
	if h.Hub != nil {
		h.Hub.PanelHub.NotifyChange()
	}
	c.JSON(http.StatusOK, gin.H{"node_group": group})
}

// DeleteNodeGroup deletes a node group when it is no longer referenced by nodes.
func (h *NodeGroupHandler) DeleteNodeGroup(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	group, err := h.DB.GetNodeGroupByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "分组不存在"})
		return
	}

	if err := h.DB.DeleteNodeGroup(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	actor := c.GetString("username")
	_ = h.DB.CreateEvent("node_group", "节点分组已删除", actor+" 删除了分组 "+group.Name)
	if h.Hub != nil {
		h.Hub.PanelHub.NotifyChange()
	}
	c.JSON(http.StatusOK, gin.H{"message": "分组已删除"})
}
