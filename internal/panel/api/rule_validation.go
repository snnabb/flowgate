package api

import (
	"github.com/flowgate/flowgate/internal/common"
	"github.com/flowgate/flowgate/internal/panel/model"
)

func validateCreateRuleTunnelSettings(req *model.CreateRuleRequest) error {
	if req == nil {
		return nil
	}
	return common.ValidateTunnelSettings(req.WSEnabled, req.TLSMode)
}

func validateCreateRuleRouteSettings(req *model.CreateRuleRequest) error {
	if req == nil {
		return nil
	}
	return common.ValidateRouteSettings(req.RouteMode, req.EntryGroup, req.RelayGroups, req.ExitGroup, req.LBStrategy)
}

func validateUpdateRuleTunnelSettings(existing *model.Rule, req *model.UpdateRuleRequest) error {
	if req == nil {
		return nil
	}

	wsEnabled := false
	tlsMode := "none"
	if existing != nil {
		wsEnabled = existing.WSEnabled
		tlsMode = existing.TLSMode
	}
	if req.WSEnabled != nil {
		wsEnabled = *req.WSEnabled
	}
	if req.TLSMode != nil {
		tlsMode = *req.TLSMode
	}

	return common.ValidateTunnelSettings(wsEnabled, tlsMode)
}

func validateUpdateRuleRouteSettings(existing *model.Rule, req *model.UpdateRuleRequest) error {
	if req == nil {
		return nil
	}

	routeMode := common.RouteModeDirect
	entryGroup := ""
	relayGroups := ""
	exitGroup := ""
	lbStrategy := common.LBStrategyNone
	if existing != nil {
		routeMode = existing.RouteMode
		entryGroup = existing.EntryGroup
		relayGroups = existing.RelayGroups
		exitGroup = existing.ExitGroup
		lbStrategy = existing.LBStrategy
	}
	if req.RouteMode != nil {
		routeMode = *req.RouteMode
	}
	if req.EntryGroup != nil {
		entryGroup = *req.EntryGroup
	}
	if req.RelayGroups != nil {
		relayGroups = *req.RelayGroups
	}
	if req.ExitGroup != nil {
		exitGroup = *req.ExitGroup
	}
	if req.LBStrategy != nil {
		lbStrategy = *req.LBStrategy
	}

	return common.ValidateRouteSettings(routeMode, entryGroup, relayGroups, exitGroup, lbStrategy)
}
