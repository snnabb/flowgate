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
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求: " + err.Error()})
		return
	}
	if err := validateCreateRuleTunnelSettings(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateCreateRuleRouteSettings(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify node exists
	_, err := h.DB.GetNodeByID(req.NodeID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "节点不存在"})
		return
	}

	rule, err := h.DB.CreateRule(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.setRuleRuntimeState(rule)

	if h.Hub.IsNodeOnline(rule.NodeID) && common.RouteModeUsesNodeRuntime(rule.RouteMode) {
		h.Hub.SendRuleToNode(rule.NodeID, common.ActionAddRule, ruleToConfig(rule))
	}

	rule, _ = h.DB.GetRuleByID(rule.ID)
	actor := c.GetString("username")
	_ = h.DB.CreateEvent("rule", "规则已创建", actor+" 创建了 "+describeRule(rule))
	h.Hub.PanelHub.NotifyChange()

	c.JSON(http.StatusOK, gin.H{"rule": rule})
}

// GetRule returns a single rule
func (h *RuleHandler) GetRule(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	rule, err := h.DB.GetRuleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "规则不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rule": rule})
}

// UpdateRule updates a forwarding rule
func (h *RuleHandler) UpdateRule(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	var req model.UpdateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求"})
		return
	}

	existing, err := h.DB.GetRuleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "规则不存在"})
		return
	}
	if err := validateUpdateRuleTunnelSettings(existing, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateUpdateRuleRouteSettings(existing, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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

		if h.Hub.IsNodeOnline(rule.NodeID) && common.RouteModeUsesNodeRuntime(rule.RouteMode) {
			h.Hub.SendRuleToNode(rule.NodeID, common.ActionUpdateRule, ruleToConfig(rule))
		}
		actor := c.GetString("username")
		_ = h.DB.CreateEvent("rule", "规则已更新", actor+" 更新了 "+describeRule(rule))
		h.Hub.PanelHub.NotifyChange()
	}

	c.JSON(http.StatusOK, gin.H{"message": "规则已更新"})
}

// DeleteRule deletes a forwarding rule
func (h *RuleHandler) DeleteRule(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	// Get rule before deletion for node notification
	rule, err := h.DB.GetRuleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "规则不存在"})
		return
	}

	if err := h.DB.DeleteRule(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Notify node to remove rule
	if common.RouteModeUsesNodeRuntime(rule.RouteMode) {
		h.Hub.SendRuleToNode(rule.NodeID, common.ActionDelRule, common.RuleConfig{
			ID: rule.ID,
		})
	}
	actor := c.GetString("username")
	_ = h.DB.CreateEvent("rule", "规则已删除", actor+" 删除了 "+describeRule(rule))
	h.Hub.PanelHub.NotifyChange()

	c.JSON(http.StatusOK, gin.H{"message": "规则已删除"})
}

// ToggleRule enables/disables a rule
func (h *RuleHandler) ToggleRule(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	rule, err := h.DB.GetRuleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "规则不存在"})
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

	if h.Hub.IsNodeOnline(rule.NodeID) && common.RouteModeUsesNodeRuntime(rule.RouteMode) {
		cfg := ruleToConfig(rule)
		cfg.Enabled = newEnabled
		h.Hub.SendRuleToNode(rule.NodeID, action, cfg)
	}
	actor := c.GetString("username")
	title := "规则已禁用"
	if newEnabled {
		title = "规则已启用"
	}
	_ = h.DB.CreateEvent("rule", title, actor+" 切换了 "+describeRule(rule))
	h.Hub.PanelHub.NotifyChange()

	c.JSON(http.StatusOK, gin.H{"enabled": newEnabled})
}

// ResetTraffic resets traffic counters for a rule
func (h *RuleHandler) ResetTraffic(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	rule, err := h.DB.GetRuleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "规则不存在"})
		return
	}

	if err := h.DB.ResetRuleTraffic(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	actor := c.GetString("username")
	_ = h.DB.CreateEvent("rule", "流量已重置", actor+" 重置了 "+describeRule(rule)+" 的流量")
	h.Hub.PanelHub.NotifyChange()

	c.JSON(http.StatusOK, gin.H{"message": "流量计数已重置"})
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
	} else if !common.RouteModeUsesNodeRuntime(rule.RouteMode) {
		status = "pending"
		message = "节点分组链路已保存，等待 Phase 2 运行时接入"
	} else if !h.Hub.IsNodeOnline(rule.NodeID) {
		status = "offline"
		message = "节点离线，连接后会自动同步"
	}

	rule.RuntimeStatus = status
	rule.RuntimeMessage = message
	_ = h.DB.UpdateRuleRuntimeStatus(rule.ID, status, message)
}

func ruleToConfig(r *model.Rule) common.RuleConfig {
	return common.RuleConfig{
		ID:            r.ID,
		Protocol:      r.Protocol,
		ListenPort:    r.ListenPort,
		TargetAddr:    r.TargetAddr,
		TargetPort:    r.TargetPort,
		SpeedLimit:    r.SpeedLimit,
		Enabled:       r.Enabled,
		ProxyProtocol: r.ProxyProtocol,
		BlockedProtos: r.BlockedProtos,
		PoolSize:      r.PoolSize,
		TLSMode:       r.TLSMode,
		TLSSni:        r.TLSSni,
		WSEnabled:     r.WSEnabled,
		WSPath:        r.WSPath,
		RouteMode:     r.RouteMode,
		EntryGroup:    r.EntryGroup,
		RelayGroups:   r.RelayGroups,
		ExitGroup:     r.ExitGroup,
		LBStrategy:    r.LBStrategy,
	}
}

func describeRule(rule *model.Rule) string {
	if rule == nil {
		return "规则"
	}

	name := rule.Name
	if name == "" {
		name = "规则 #" + strconv.FormatInt(rule.ID, 10)
	}

	return name + " (" + rule.Protocol + " :" + strconv.Itoa(rule.ListenPort) + " -> " + rule.TargetAddr + ":" + strconv.Itoa(rule.TargetPort) + ")"
}
