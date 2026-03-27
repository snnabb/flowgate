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

func TestUserNodeAccessRoundTripAndScopedVisibility(t *testing.T) {
	t.Parallel()

	database := newTestDatabase(t)

	admin, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "admin",
		Role:     "admin",
	}, "hash-admin")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	user, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "user",
		Role:     "user",
	}, "hash-user")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	outsider, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "outsider",
		Role:     "user",
	}, "hash-outsider")
	if err != nil {
		t.Fatalf("create outsider user: %v", err)
	}

	alphaNode, err := database.CreateNode("alpha-node", "")
	if err != nil {
		t.Fatalf("create alpha node: %v", err)
	}
	betaNode, err := database.CreateNode("beta-node", "")
	if err != nil {
		t.Fatalf("create beta node: %v", err)
	}

	if err := database.ReplaceUserNodeAccess(user.ID, []model.UserNodeAccessInput{
		{NodeID: alphaNode.ID, TrafficQuota: 4096, BandwidthLimit: 1024, MaxRules: 1},
		{NodeID: betaNode.ID, TrafficQuota: 8192, BandwidthLimit: 2048, MaxRules: 3},
	}); err != nil {
		t.Fatalf("replace user node access: %v", err)
	}

	access, err := database.ListUserNodeAccess(user.ID)
	if err != nil {
		t.Fatalf("list user node access: %v", err)
	}
	if len(access) != 2 {
		t.Fatalf("expected 2 access rows, got %d", len(access))
	}
	if access[0].MaxRules != 1 {
		t.Fatalf("expected alpha max rules 1, got %d", access[0].MaxRules)
	}
	if access[1].MaxRules != 3 {
		t.Fatalf("expected beta max rules 3, got %d", access[1].MaxRules)
	}

	visibleUsers, err := database.ListUsersVisibleTo(admin)
	if err != nil {
		t.Fatalf("list users visible to admin: %v", err)
	}
	if got, want := collectUsernames(visibleUsers), []string{"admin", "user", "outsider"}; !sameStrings(got, want) {
		t.Fatalf("unexpected admin-visible users: got %v want %v", got, want)
	}

	visibleNodes, err := database.ListNodesVisibleTo(user)
	if err != nil {
		t.Fatalf("list nodes visible to user: %v", err)
	}
	if got, want := collectNodeNames(visibleNodes), []string{"alpha-node", "beta-node"}; !sameStrings(got, want) {
		t.Fatalf("unexpected user-visible nodes: got %v want %v", got, want)
	}

	if _, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:      alphaNode.ID,
		Name:        "user-rule",
		Protocol:    "tcp",
		ListenPort:  31001,
		TargetAddr:  "127.0.0.1",
		TargetPort:  8080,
	}, user.ID); err != nil {
		t.Fatalf("create user rule: %v", err)
	}

	outsiderNode, err := database.CreateNode("outsider-node", "")
	if err != nil {
		t.Fatalf("create outsider node: %v", err)
	}
	if _, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:      outsiderNode.ID,
		Name:        "outsider-rule",
		Protocol:    "tcp",
		ListenPort:  31002,
		TargetAddr:  "127.0.0.1",
		TargetPort:  8081,
	}, outsider.ID); err != nil {
		t.Fatalf("create outsider rule: %v", err)
	}

	visibleRules, err := database.ListRulesVisibleTo(user, 0)
	if err != nil {
		t.Fatalf("list rules visible to user: %v", err)
	}
	if got, want := collectRuleNames(visibleRules), []string{"user-rule"}; !sameStrings(got, want) {
		t.Fatalf("unexpected user-visible rules: got %v want %v", got, want)
	}
}

func TestRuleTrafficUsageAccumulatesAssignedNodeQuota(t *testing.T) {
	t.Parallel()

	database := newTestDatabase(t)
	user, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "quota-user",
		Role:     "user",
	}, "hash-user")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	node, err := database.CreateNode("quota-node", "")
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	if err := database.ReplaceUserNodeAccess(user.ID, []model.UserNodeAccessInput{
		{NodeID: node.ID, TrafficQuota: 80, BandwidthLimit: 256, MaxRules: 2},
	}); err != nil {
		t.Fatalf("replace user node access: %v", err)
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

	storedAccess, err := database.GetUserNodeAccess(user.ID, node.ID)
	if err != nil {
		t.Fatalf("get user node access: %v", err)
	}
	if storedAccess.TrafficUsed != 40 {
		t.Fatalf("expected traffic_used 40, got %d", storedAccess.TrafficUsed)
	}

	exceeded, err := database.CheckUserNodeTrafficQuotaExceeded(user.ID, node.ID)
	if err != nil {
		t.Fatalf("check quota exceeded: %v", err)
	}
	if exceeded {
		t.Fatal("expected quota to remain available after first traffic update")
	}

	if err := database.UpdateRuleTraffic(rule.ID, 20, 20); err != nil {
		t.Fatalf("update rule traffic second time: %v", err)
	}

	exceeded, err = database.CheckUserNodeTrafficQuotaExceeded(user.ID, node.ID)
	if err != nil {
		t.Fatalf("check quota exceeded after second update: %v", err)
	}
	if !exceeded {
		t.Fatal("expected quota to be exceeded after second traffic update")
	}
}

func TestCountTopLevelRulesByOwnerAndNode(t *testing.T) {
	t.Parallel()

	database := newTestDatabase(t)
	user, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "node-limit-user",
		Role:     "user",
	}, "hash-user")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	nodeOne, err := database.CreateNode("node-limit-a", "")
	if err != nil {
		t.Fatalf("create node one: %v", err)
	}
	nodeTwo, err := database.CreateNode("node-limit-b", "")
	if err != nil {
		t.Fatalf("create node two: %v", err)
	}

	for idx, nodeID := range []int64{nodeOne.ID, nodeOne.ID, nodeTwo.ID} {
		if _, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
			NodeID:     nodeID,
			Name:       "rule",
			Protocol:   "tcp",
			ListenPort: 33010 + idx,
			TargetAddr: "127.0.0.1",
			TargetPort: 8080 + idx,
		}, user.ID); err != nil {
			t.Fatalf("create rule %d: %v", idx, err)
		}
	}

	countOne, err := database.CountTopLevelRulesByOwnerAndNode(user.ID, nodeOne.ID)
	if err != nil {
		t.Fatalf("count node one rules: %v", err)
	}
	if countOne != 2 {
		t.Fatalf("expected 2 rules on node one, got %d", countOne)
	}

	countTwo, err := database.CountTopLevelRulesByOwnerAndNode(user.ID, nodeTwo.ID)
	if err != nil {
		t.Fatalf("count node two rules: %v", err)
	}
	if countTwo != 1 {
		t.Fatalf("expected 1 rule on node two, got %d", countTwo)
	}
}

func TestNormalizePanelEventText(t *testing.T) {
	t.Parallel()

	title, details := normalizePanelEventText("User created", "admin created demo-user")
	if title != "用户已创建" {
		t.Fatalf("expected chinese title, got %q", title)
	}
	if details != "admin 创建了用户 demo-user" {
		t.Fatalf("expected chinese details, got %q", details)
	}

	title, details = normalizePanelEventText("鐢ㄦ埛宸插垱寤?", "admin 鍒涘缓浜嗙敤鎴?123")
	if title != "用户已创建" {
		t.Fatalf("expected mojibake title normalized, got %q", title)
	}
	if details != "admin 创建了用户 123" {
		t.Fatalf("expected mojibake details normalized, got %q", details)
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
