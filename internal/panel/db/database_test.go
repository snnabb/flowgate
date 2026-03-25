package db

import (
	"strings"
	"testing"

	"github.com/flowgate/flowgate/internal/common"
	"github.com/flowgate/flowgate/internal/panel/model"
)

func TestNodeGroupCRUD(t *testing.T) {
	t.Parallel()

	database := newTestDatabase(t)

	group, err := database.CreateNodeGroup("entry-hk", "香港入口组")
	if err != nil {
		t.Fatalf("create node group: %v", err)
	}

	groups, err := database.ListNodeGroups()
	if err != nil {
		t.Fatalf("list node groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Name != "entry-hk" {
		t.Fatalf("expected group name entry-hk, got %q", groups[0].Name)
	}
	if groups[0].NodeCount != 0 {
		t.Fatalf("expected empty group node count, got %d", groups[0].NodeCount)
	}

	if _, err := database.CreateNode("hk-01", group.Name); err != nil {
		t.Fatalf("create node in group: %v", err)
	}

	groups, err = database.ListNodeGroups()
	if err != nil {
		t.Fatalf("list groups after node create: %v", err)
	}
	if groups[0].NodeCount != 1 {
		t.Fatalf("expected node count 1, got %d", groups[0].NodeCount)
	}

	if err := database.DeleteNodeGroup(group.ID); err == nil {
		t.Fatal("expected delete to fail when nodes still reference the group")
	}
}

func TestRuleRouteFieldsRoundTrip(t *testing.T) {
	t.Parallel()

	database := newTestDatabase(t)

	node, err := database.CreateNode("edge-01", "")
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	req := &model.CreateRuleRequest{
		NodeID:      node.ID,
		Name:        "route-skeleton",
		Protocol:    "tcp",
		ListenPort:  22001,
		TargetAddr:  "127.0.0.1",
		TargetPort:  8080,
		RouteMode:   common.RouteModeGroupChain,
		EntryGroup:  "entry-hk",
		RelayGroups: "relay-sg,relay-jp",
		ExitGroup:   "exit-us",
		LBStrategy:  common.LBStrategyLeastLatency,
	}

	rule, err := database.CreateRule(req)
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	stored, err := database.GetRuleByID(rule.ID)
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	if stored.RouteMode != common.RouteModeGroupChain {
		t.Fatalf("expected route mode %q, got %q", common.RouteModeGroupChain, stored.RouteMode)
	}
	if stored.EntryGroup != "entry-hk" || stored.RelayGroups != "relay-sg,relay-jp" || stored.ExitGroup != "exit-us" {
		t.Fatalf("unexpected stored route groups: %+v", stored)
	}
	if stored.LBStrategy != common.LBStrategyLeastLatency {
		t.Fatalf("expected lb strategy %q, got %q", common.LBStrategyLeastLatency, stored.LBStrategy)
	}

	mode := common.RouteModeDirect
	lb := common.LBStrategyRoundRobin
	update := &model.UpdateRuleRequest{
		RouteMode:  &mode,
		EntryGroup: stringPtr(""),
		RelayGroups: stringPtr(""),
		ExitGroup:  stringPtr(""),
		LBStrategy: &lb,
	}
	if err := database.UpdateRule(rule.ID, update); err != nil {
		t.Fatalf("update rule: %v", err)
	}

	stored, err = database.GetRuleByID(rule.ID)
	if err != nil {
		t.Fatalf("get updated rule: %v", err)
	}
	if stored.RouteMode != common.RouteModeDirect {
		t.Fatalf("expected updated route mode %q, got %q", common.RouteModeDirect, stored.RouteMode)
	}
	if stored.LBStrategy != common.LBStrategyRoundRobin {
		t.Fatalf("expected updated lb strategy %q, got %q", common.LBStrategyRoundRobin, stored.LBStrategy)
	}
}

func newTestDatabase(t *testing.T) *Database {
	t.Helper()

	database, err := New(t.TempDir() + "/flowgate-test.db")
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo to work") {
			t.Skip("sqlite cgo runtime unavailable in this test environment")
		}
		t.Fatalf("new database: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	return database
}

func stringPtr(value string) *string {
	return &value
}
