package api

import (
	"testing"

	"github.com/flowgate/flowgate/internal/common"
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

func TestValidateCreateRuleRejectsInvalidHopChainJSON(t *testing.T) {
	t.Parallel()

	req := &model.CreateRuleRequest{
		RouteMode: common.RouteModeHopChain,
		RouteHops: `{"broken":true}`,
	}

	if err := validateCreateRuleRouteSettings(req); err == nil {
		t.Fatal("expected create validation to reject invalid route_hops json")
	}
}

func TestValidateCreateRuleRejectsEmptyHopChain(t *testing.T) {
	t.Parallel()

	req := &model.CreateRuleRequest{
		RouteMode: common.RouteModeHopChain,
		RouteHops: `[]`,
	}

	if err := validateCreateRuleRouteSettings(req); err == nil {
		t.Fatal("expected create validation to reject empty hop chain")
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

func TestValidateUpdateRuleRejectsHopWithoutTargets(t *testing.T) {
	t.Parallel()

	existing := &model.Rule{
		RouteMode: common.RouteModeDirect,
		RouteHops: "[]",
	}
	mode := common.RouteModeHopChain
	hops := `[{"order":1,"targets":[]}]`
	req := &model.UpdateRuleRequest{
		RouteMode: &mode,
		RouteHops: &hops,
	}

	if err := validateUpdateRuleRouteSettings(existing, req); err == nil {
		t.Fatal("expected update validation to reject hop chain without targets")
	}
}
