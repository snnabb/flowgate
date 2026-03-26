package db

import (
	"strings"
	"testing"
	"time"

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
		RouteMode:   common.RouteModeHopChain,
		RouteHops:   `[{"order":1,"targets":[{"host":"1.2.3.4","port":4000},{"host":"1.2.3.4","port":40001}],"lb_strategy":"round_robin"},{"order":2,"targets":[{"host":"1.2.3.5","port":40001}],"lb_strategy":"none"}]`,
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
	if stored.RouteMode != common.RouteModeHopChain {
		t.Fatalf("expected route mode %q, got %q", common.RouteModeHopChain, stored.RouteMode)
	}
	if stored.RouteHops == "" || !strings.Contains(stored.RouteHops, `"host":"1.2.3.4"`) || !strings.Contains(stored.RouteHops, `"host":"1.2.3.5"`) {
		t.Fatalf("unexpected stored route hops: %s", stored.RouteHops)
	}
	if stored.LBStrategy != common.LBStrategyLeastLatency {
		t.Fatalf("expected lb strategy %q, got %q", common.LBStrategyLeastLatency, stored.LBStrategy)
	}

	mode := common.RouteModeDirect
	lb := common.LBStrategyRoundRobin
	update := &model.UpdateRuleRequest{
		RouteMode: &mode,
		RouteHops: stringPtr("[]"),
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
	if stored.RouteHops != "[]" {
		t.Fatalf("expected updated route hops [], got %s", stored.RouteHops)
	}
}

func TestUserScopeAndOwnershipRoundTrip(t *testing.T) {
	t.Parallel()

	database := newTestDatabase(t)
	expiresAt := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)

	admin, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "admin",
		Role:     "admin",
	}, "hash-admin")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	reseller, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username:       "reseller",
		Role:           "reseller",
		ParentID:       &admin.ID,
		TrafficQuota:   2048,
		Ratio:          1.5,
		ExpiresAt:      &expiresAt,
		MaxRules:       3,
		BandwidthLimit: 512,
	}, "hash-reseller")
	if err != nil {
		t.Fatalf("create reseller: %v", err)
	}

	child, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "child",
		Role:     "user",
		ParentID: &reseller.ID,
	}, "hash-child")
	if err != nil {
		t.Fatalf("create child user: %v", err)
	}

	outsider, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "outsider",
		Role:     "user",
	}, "hash-outsider")
	if err != nil {
		t.Fatalf("create outsider user: %v", err)
	}

	storedReseller, err := database.GetUserByUsername("reseller")
	if err != nil {
		t.Fatalf("get reseller: %v", err)
	}
	if storedReseller.ParentID != admin.ID {
		t.Fatalf("expected parent_id %d, got %d", admin.ID, storedReseller.ParentID)
	}
	if storedReseller.Ratio != 1.5 {
		t.Fatalf("expected ratio 1.5, got %v", storedReseller.Ratio)
	}
	if storedReseller.MaxRules != 3 {
		t.Fatalf("expected max_rules 3, got %d", storedReseller.MaxRules)
	}
	if storedReseller.BandwidthLimit != 512 {
		t.Fatalf("expected bandwidth limit 512, got %d", storedReseller.BandwidthLimit)
	}
	if storedReseller.ExpiresAt == nil || !storedReseller.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expires_at %v, got %v", expiresAt, storedReseller.ExpiresAt)
	}

	visibleUsers, err := database.ListUsersVisibleTo(reseller)
	if err != nil {
		t.Fatalf("list users visible to reseller: %v", err)
	}
	if got, want := collectUsernames(visibleUsers), []string{"reseller", "child"}; !sameStrings(got, want) {
		t.Fatalf("unexpected reseller-visible users: got %v want %v", got, want)
	}

	nodeOwnedByReseller, err := database.CreateNodeWithOwner(&model.CreateNodeRequest{Name: "reseller-node"}, reseller.ID)
	if err != nil {
		t.Fatalf("create reseller node: %v", err)
	}
	childNode, err := database.CreateNodeWithOwner(&model.CreateNodeRequest{Name: "child-node"}, child.ID)
	if err != nil {
		t.Fatalf("create child node: %v", err)
	}
	outsiderNode, err := database.CreateNodeWithOwner(&model.CreateNodeRequest{Name: "outsider-node"}, outsider.ID)
	if err != nil {
		t.Fatalf("create outsider node: %v", err)
	}

	visibleNodes, err := database.ListNodesVisibleTo(reseller)
	if err != nil {
		t.Fatalf("list nodes visible to reseller: %v", err)
	}
	if got, want := collectNodeNames(visibleNodes), []string{"reseller-node", "child-node"}; !sameStrings(got, want) {
		t.Fatalf("unexpected reseller-visible nodes: got %v want %v", got, want)
	}

	if _, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:      nodeOwnedByReseller.ID,
		Name:        "reseller-rule",
		Protocol:    "tcp",
		ListenPort:  31001,
		TargetAddr:  "127.0.0.1",
		TargetPort:  8080,
	}, reseller.ID); err != nil {
		t.Fatalf("create reseller rule: %v", err)
	}
	if _, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:      childNode.ID,
		Name:        "child-rule",
		Protocol:    "tcp",
		ListenPort:  31002,
		TargetAddr:  "127.0.0.1",
		TargetPort:  8081,
	}, child.ID); err != nil {
		t.Fatalf("create child rule: %v", err)
	}
	if _, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:      outsiderNode.ID,
		Name:        "outsider-rule",
		Protocol:    "tcp",
		ListenPort:  31003,
		TargetAddr:  "127.0.0.1",
		TargetPort:  8082,
	}, outsider.ID); err != nil {
		t.Fatalf("create outsider rule: %v", err)
	}

	visibleRules, err := database.ListRulesVisibleTo(reseller, 0)
	if err != nil {
		t.Fatalf("list rules visible to reseller: %v", err)
	}
	if got, want := collectRuleNames(visibleRules), []string{"reseller-rule", "child-rule"}; !sameStrings(got, want) {
		t.Fatalf("unexpected reseller-visible rules: got %v want %v", got, want)
	}
}

func TestRuleTrafficUsageRespectsUserRatio(t *testing.T) {
	t.Parallel()

	database := newTestDatabase(t)
	user, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username:       "quota-user",
		Role:           "user",
		TrafficQuota:   100,
		Ratio:          1.5,
		MaxRules:       1,
		BandwidthLimit: 256,
	}, "hash-user")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	node, err := database.CreateNodeWithOwner(&model.CreateNodeRequest{Name: "quota-node"}, user.ID)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	rule, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:      node.ID,
		Name:        "quota-rule",
		Protocol:    "tcp",
		ListenPort:  32001,
		TargetAddr:  "127.0.0.1",
		TargetPort:  9000,
		SpeedLimit:  128,
	}, user.ID)
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	ruleCount, err := database.CountTopLevelRulesByOwner(user.ID)
	if err != nil {
		t.Fatalf("count rules by owner: %v", err)
	}
	if ruleCount != 1 {
		t.Fatalf("expected 1 top-level rule, got %d", ruleCount)
	}

	if err := database.UpdateRuleTraffic(rule.ID, 20, 20); err != nil {
		t.Fatalf("update rule traffic: %v", err)
	}

	storedUser, err := database.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("get user by id: %v", err)
	}
	if storedUser.TrafficUsed != 60 {
		t.Fatalf("expected traffic_used 60, got %d", storedUser.TrafficUsed)
	}

	exceeded, err := database.CheckUserTrafficQuotaExceeded(user.ID)
	if err != nil {
		t.Fatalf("check quota exceeded: %v", err)
	}
	if exceeded {
		t.Fatal("expected quota to remain available after first traffic update")
	}

	if err := database.UpdateRuleTraffic(rule.ID, 20, 20); err != nil {
		t.Fatalf("update rule traffic second time: %v", err)
	}

	exceeded, err = database.CheckUserTrafficQuotaExceeded(user.ID)
	if err != nil {
		t.Fatalf("check quota exceeded after second update: %v", err)
	}
	if !exceeded {
		t.Fatal("expected quota to be exceeded after second traffic update")
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

func collectUsernames(users []model.User) []string {
	result := make([]string, 0, len(users))
	for _, user := range users {
		result = append(result, user.Username)
	}
	return result
}

func collectNodeNames(nodes []model.Node) []string {
	result := make([]string, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, node.Name)
	}
	return result
}

func collectRuleNames(rules []model.Rule) []string {
	result := make([]string, 0, len(rules))
	for _, rule := range rules {
		result = append(result, rule.Name)
	}
	return result
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]int, len(got))
	for _, item := range got {
		seen[item]++
	}
	for _, item := range want {
		seen[item]--
		if seen[item] < 0 {
			return false
		}
	}
	for _, v := range seen {
		if v != 0 {
			return false
		}
	}
	return true
}
