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
