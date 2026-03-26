package api

import (
	"fmt"
	"log"
	"net"
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

	// Managed chain: validate hops have node_id + listen_port
	if req.ChainType == "managed" {
		hops, err := common.ParseRouteHops(req.RouteHops)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := validateManagedChainHops(hops); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
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

	// For managed chains, create child relay rules on each hop node
	if rule.ChainType == "managed" {
		if err := h.createManagedChainRelays(rule); err != nil {
			log.Printf("[rule] failed to create managed chain relays for rule %d: %v", rule.ID, err)
		}
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

	// If switching to managed chain or updating hops, validate
	chainType := existing.ChainType
	if req.ChainType != nil {
		chainType = *req.ChainType
	}
	if chainType == "managed" {
		hopsRaw := existing.RouteHops
		if req.RouteHops != nil {
			hopsRaw = *req.RouteHops
		}
		hops, parseErr := common.ParseRouteHops(hopsRaw)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": parseErr.Error()})
			return
		}
		if err := validateManagedChainHops(hops); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
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

		// Rebuild managed chain relay rules if chain type or hops changed
		if rule.ChainType == "managed" {
			h.deleteManagedChainRelays(rule.ID)
			if err := h.createManagedChainRelays(rule); err != nil {
				log.Printf("[rule] failed to rebuild managed chain relays for rule %d: %v", rule.ID, err)
			}
		} else if existing.ChainType == "managed" && rule.ChainType != "managed" {
			// Switched from managed to custom — remove old child rules
			h.deleteManagedChainRelays(rule.ID)
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

	// For managed chains, notify relay nodes and delete child rules
	if rule.ChainType == "managed" {
		h.deleteManagedChainRelays(rule.ID)
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

// TestLatency sends an on-demand latency test command to the node.
// For managed chains, it also triggers tests on all relay nodes.
func (h *RuleHandler) TestLatency(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	rule, err := h.DB.GetRuleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "规则不存在"})
		return
	}

	if !h.Hub.IsNodeOnline(rule.NodeID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "节点不在线"})
		return
	}

	// Test the main rule's node
	msg := common.NewMessage(common.MsgTypeCommand, common.ActionTestLatency, common.TestLatencyRequest{
		RuleID: rule.ID,
	})
	h.Hub.SendToNode(rule.NodeID, msg)

	// For managed chains, also test each child relay rule
	if rule.ChainType == "managed" {
		children, err := h.DB.ListChildRules(rule.ID)
		if err == nil {
			for _, child := range children {
				if h.Hub.IsNodeOnline(child.NodeID) {
					childMsg := common.NewMessage(common.MsgTypeCommand, common.ActionTestLatency, common.TestLatencyRequest{
						RuleID: child.ID,
					})
					h.Hub.SendToNode(child.NodeID, childMsg)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "延迟测试已发起"})
}

// GetChainLatency returns per-hop latency for a managed chain rule.
func (h *RuleHandler) GetChainLatency(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	rule, err := h.DB.GetRuleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "规则不存在"})
		return
	}

	type HopLatency struct {
		Order   int     `json:"order"`
		NodeID  int64   `json:"node_id"`
		RuleID  int64   `json:"rule_id"`
		Latency float64 `json:"latency_ms"`
	}

	result := struct {
		EntryLatency float64      `json:"entry_latency_ms"`
		Hops         []HopLatency `json:"hops"`
		TotalLatency float64      `json:"total_latency_ms"`
	}{
		EntryLatency: rule.Latency,
		Hops:         []HopLatency{},
	}

	total := rule.Latency
	if total < 0 {
		total = 0
	}

	if rule.ChainType == "managed" {
		children, err := h.DB.ListChildRules(rule.ID)
		if err == nil {
			hops, _ := common.ParseRouteHops(rule.RouteHops)
			hopOrderMap := make(map[int64]int) // nodeID -> order
			for _, hop := range hops {
				hopOrderMap[hop.NodeID] = hop.Order
			}

			for _, child := range children {
				order := hopOrderMap[child.NodeID]
				result.Hops = append(result.Hops, HopLatency{
					Order:   order,
					NodeID:  child.NodeID,
					RuleID:  child.ID,
					Latency: child.Latency,
				})
				if child.Latency > 0 {
					total += child.Latency
				}
			}
		}
	}

	result.TotalLatency = total
	c.JSON(http.StatusOK, result)
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
		RouteHops:     r.RouteHops,
		EntryGroup:    r.EntryGroup,
		RelayGroups:   r.RelayGroups,
		ExitGroup:     r.ExitGroup,
		LBStrategy:    r.LBStrategy,
	}
}

// createManagedChainRelays auto-creates direct relay rules for each hop in a managed chain.
// Chain topology: entry_node -> hop1_node:port -> hop2_node:port -> ... -> target_addr:target_port
// Each hop gets a direct rule where its target is the NEXT hop's node IP:port (or final target for last hop).
func (h *RuleHandler) createManagedChainRelays(rule *model.Rule) error {
	hops, err := common.ParseRouteHops(rule.RouteHops)
	if err != nil {
		return err
	}
	if len(hops) == 0 {
		return nil
	}

	// Resolve node IPs for each hop
	type resolvedHop struct {
		hop    common.RouteHop
		nodeIP string
	}
	resolved := make([]resolvedHop, 0, len(hops))
	for _, hop := range hops {
		node, err := h.DB.GetNodeByID(hop.NodeID)
		if err != nil {
			return fmt.Errorf("跳点 %d 的节点 %d 不存在", hop.Order, hop.NodeID)
		}
		host, _, err := net.SplitHostPort(node.IPAddr)
		if err != nil {
			// IPAddr might be plain IP without port
			host = node.IPAddr
		}
		if host == "" {
			return fmt.Errorf("节点 %s (ID %d) 尚未上报 IP 地址，请确保节点已上线", node.Name, node.ID)
		}
		resolved = append(resolved, resolvedHop{hop: hop, nodeIP: host})
	}

	// Build child rules: each hop's target = next hop's IP:ListenPort, last hop's target = final target
	for i, rh := range resolved {
		var targetAddr string
		var targetPort int

		if i+1 < len(resolved) {
			// Target is the next hop
			next := resolved[i+1]
			targetAddr = next.nodeIP
			targetPort = next.hop.ListenPort
		} else {
			// Last hop targets the final destination
			targetAddr = rule.TargetAddr
			targetPort = rule.TargetPort
		}

		childReq := &model.CreateRuleRequest{
			NodeID:       rh.hop.NodeID,
			Name:         fmt.Sprintf("%s [中转%d]", rule.Name, rh.hop.Order),
			Protocol:     rule.Protocol,
			ListenPort:   rh.hop.ListenPort,
			TargetAddr:   targetAddr,
			TargetPort:   targetPort,
			SpeedLimit:   rule.SpeedLimit,
			RouteMode:    common.RouteModeDirect,
			ChainType:    "managed",
			ParentRuleID: rule.ID,
		}

		child, err := h.DB.CreateRule(childReq)
		if err != nil {
			return fmt.Errorf("创建跳点 %d 的中转规则失败: %w", rh.hop.Order, err)
		}

		// Set runtime state and push to node if online
		h.setRuleRuntimeState(child)
		if h.Hub.IsNodeOnline(child.NodeID) {
			h.Hub.SendRuleToNode(child.NodeID, common.ActionAddRule, ruleToConfig(child))
		}
	}

	return nil
}

// deleteManagedChainRelays removes child relay rules and notifies their nodes.
func (h *RuleHandler) deleteManagedChainRelays(parentID int64) {
	children, err := h.DB.ListChildRules(parentID)
	if err != nil {
		log.Printf("[rule] failed to list child rules for parent %d: %v", parentID, err)
		return
	}

	// Notify each relay node to remove the rule
	for _, child := range children {
		h.Hub.SendRuleToNode(child.NodeID, common.ActionDelRule, common.RuleConfig{ID: child.ID})
	}

	if err := h.DB.DeleteChildRules(parentID); err != nil {
		log.Printf("[rule] failed to delete child rules for parent %d: %v", parentID, err)
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
