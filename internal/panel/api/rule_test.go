package api

import (
	"testing"

	"github.com/flowgate/flowgate/internal/panel/model"
)

func TestValidateCreateRuleRejectsWebSocketWithInboundTLS(t *testing.T) {
	t.Parallel()

	req := &model.CreateRuleRequest{
		WSEnabled: true,
		TLSMode:   "client",
	}

	if err := validateCreateRuleTunnelSettings(req); err == nil {
		t.Fatal("expected create validation to reject WS + inbound TLS")
	}
}

func TestValidateUpdateRuleRejectsWebSocketWithInboundTLS(t *testing.T) {
	t.Parallel()

	existing := &model.Rule{
		WSEnabled: false,
		TLSMode:   "none",
	}
	wsEnabled := true
	tlsMode := "both"
	req := &model.UpdateRuleRequest{
		WSEnabled: &wsEnabled,
		TLSMode:   &tlsMode,
	}

	if err := validateUpdateRuleTunnelSettings(existing, req); err == nil {
		t.Fatal("expected update validation to reject WS + inbound TLS")
	}
}

func TestValidateCreateRuleRejectsUnknownRouteMode(t *testing.T) {
	t.Parallel()

	req := &model.CreateRuleRequest{
		RouteMode: "mystery",
	}

	if err := validateCreateRuleRouteSettings(req); err == nil {
		t.Fatal("expected create validation to reject unknown route mode")
	}
}

func TestValidateCreateRuleRejectsIncompleteGroupChain(t *testing.T) {
	t.Parallel()

	req := &model.CreateRuleRequest{
		RouteMode:  "group_chain",
		EntryGroup: "entry-hk",
	}

	if err := validateCreateRuleRouteSettings(req); err == nil {
		t.Fatal("expected create validation to reject incomplete group chain")
	}
}

func TestValidateUpdateRuleRejectsUnknownLoadBalanceStrategy(t *testing.T) {
	t.Parallel()

	existing := &model.Rule{
		RouteMode:  "direct",
		LBStrategy: "none",
	}
	strategy := "weighted-magic"
	req := &model.UpdateRuleRequest{
		LBStrategy: &strategy,
	}

	if err := validateUpdateRuleRouteSettings(existing, req); err == nil {
		t.Fatal("expected update validation to reject unknown load balance strategy")
	}
}
