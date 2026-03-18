package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/flowgate/flowgate/internal/common"
	"github.com/flowgate/flowgate/internal/panel/db"
	"github.com/flowgate/flowgate/internal/panel/hub"
	"github.com/flowgate/flowgate/internal/panel/model"
)

type RuleHandler struct {
	DB  *db.Database
	Hub *hub.Hub
}

// ListRules returns all rules, optionally filtered by node_id
func (h *RuleHandler) ListRules(c *gin.Context) {
	nodeID, _ := strconv.ParseInt(c.Query("node_id"), 10, 64)
	rules, err := h.DB.ListRules(nodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rules == nil {
		rules = []model.Rule{}
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

// CreateRule creates a new forwarding rule
func (h *RuleHandler) CreateRule(c *gin.Context) {
	var req model.CreateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	// Verify node exists
	_, err := h.DB.GetNodeByID(req.NodeID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Node not found"})
		return
	}

	rule, err := h.DB.CreateRule(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.setRuleRuntimeState(rule)

	if h.Hub.IsNodeOnline(rule.NodeID) {
		h.Hub.SendRuleToNode(rule.NodeID, common.ActionAddRule, common.RuleConfig{
			ID:         rule.ID,
			Protocol:   rule.Protocol,
			ListenPort: rule.ListenPort,
			TargetAddr: rule.TargetAddr,
			TargetPort: rule.TargetPort,
			SpeedLimit: rule.SpeedLimit,
			Enabled:    rule.Enabled,
		})
	}

	rule, _ = h.DB.GetRuleByID(rule.ID)

	c.JSON(http.StatusOK, gin.H{"rule": rule})
}

// GetRule returns a single rule
func (h *RuleHandler) GetRule(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	rule, err := h.DB.GetRuleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rule": rule})
}

// UpdateRule updates a forwarding rule
func (h *RuleHandler) UpdateRule(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	var req model.UpdateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if err := h.DB.UpdateRule(id, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Re-fetch updated rule and push to node
	rule, err := h.DB.GetRuleByID(id)
	if err == nil {
		h.setRuleRuntimeState(rule)

		if h.Hub.IsNodeOnline(rule.NodeID) {
			h.Hub.SendRuleToNode(rule.NodeID, common.ActionUpdateRule, common.RuleConfig{
				ID:         rule.ID,
				Protocol:   rule.Protocol,
				ListenPort: rule.ListenPort,
				TargetAddr: rule.TargetAddr,
				TargetPort: rule.TargetPort,
				SpeedLimit: rule.SpeedLimit,
				Enabled:    rule.Enabled,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Rule updated"})
}

// DeleteRule deletes a forwarding rule
func (h *RuleHandler) DeleteRule(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	// Get rule before deletion for node notification
	rule, err := h.DB.GetRuleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	if err := h.DB.DeleteRule(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Notify node to remove rule
	h.Hub.SendRuleToNode(rule.NodeID, common.ActionDelRule, common.RuleConfig{
		ID: rule.ID,
	})

	c.JSON(http.StatusOK, gin.H{"message": "Rule deleted"})
}

// ToggleRule enables/disables a rule
func (h *RuleHandler) ToggleRule(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	rule, err := h.DB.GetRuleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	newEnabled := !rule.Enabled
	req := &model.UpdateRuleRequest{Enabled: &newEnabled}
	h.DB.UpdateRule(id, req)

	rule.Enabled = newEnabled
	h.setRuleRuntimeState(rule)

	action := common.ActionAddRule
	if !newEnabled {
		action = common.ActionDelRule
	}

	if h.Hub.IsNodeOnline(rule.NodeID) {
		h.Hub.SendRuleToNode(rule.NodeID, action, common.RuleConfig{
			ID:         rule.ID,
			Protocol:   rule.Protocol,
			ListenPort: rule.ListenPort,
			TargetAddr: rule.TargetAddr,
			TargetPort: rule.TargetPort,
			SpeedLimit: rule.SpeedLimit,
			Enabled:    newEnabled,
		})
	}

	c.JSON(http.StatusOK, gin.H{"enabled": newEnabled})
}

func (h *RuleHandler) setRuleRuntimeState(rule *model.Rule) {
	if rule == nil {
		return
	}

	status := "pending"
	message := "已下发到节点，等待确认"

	if !rule.Enabled {
		status = "stopped"
		message = "规则已禁用"
	} else if !h.Hub.IsNodeOnline(rule.NodeID) {
		status = "offline"
		message = "节点离线，连接后会自动同步"
	}

	rule.RuntimeStatus = status
	rule.RuntimeMessage = message
	_ = h.DB.UpdateRuleRuntimeStatus(rule.ID, status, message)
}
