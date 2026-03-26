package hub

import (
	"strings"
	"testing"

	"github.com/flowgate/flowgate/internal/common"
	"github.com/flowgate/flowgate/internal/panel/db"
	"github.com/flowgate/flowgate/internal/panel/model"
)

func TestNodeQuotaExceededDisablesOnlyAssignedNodeRules(t *testing.T) {
	t.Parallel()

	database := newTestHubDatabase(t)
	user, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username: "quota-owner",
		Role:     "user",
	}, "hash-user")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	nodeOne, err := database.CreateNode("quota-node-one", "")
	if err != nil {
		t.Fatalf("create node one: %v", err)
	}
	nodeTwo, err := database.CreateNode("quota-node-two", "")
	if err != nil {
		t.Fatalf("create node two: %v", err)
	}
	if err := database.ReplaceUserNodeAccess(user.ID, []model.UserNodeAccessInput{
		{NodeID: nodeOne.ID, TrafficQuota: 100, BandwidthLimit: 64},
		{NodeID: nodeTwo.ID, TrafficQuota: 1000, BandwidthLimit: 64},
	}); err != nil {
		t.Fatalf("replace user node access: %v", err)
	}

	ruleOne, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:     nodeOne.ID,
		Name:       "quota-rule-one",
		Protocol:   "tcp",
		ListenPort: 36001,
		TargetAddr: "127.0.0.1",
		TargetPort: 8080,
		SpeedLimit: 64,
	}, user.ID)
	if err != nil {
		t.Fatalf("create rule one: %v", err)
	}
	ruleTwo, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:     nodeTwo.ID,
		Name:       "quota-rule-two",
		Protocol:   "tcp",
		ListenPort: 36002,
		TargetAddr: "127.0.0.1",
		TargetPort: 8081,
		SpeedLimit: 64,
	}, user.ID)
	if err != nil {
		t.Fatalf("create rule two: %v", err)
	}

	h := New(database)
	h.handleNodeMessage(nodeOne.ID, &common.WSMessage{
		Action: common.ActionReportStats,
		Data: []common.TrafficReport{
			{RuleID: ruleOne.ID, TrafficIn: 60, TrafficOut: 50},
		},
	})

	updatedRuleOne, err := database.GetRuleByID(ruleOne.ID)
	if err != nil {
		t.Fatalf("get updated rule one: %v", err)
	}
	if updatedRuleOne.Enabled {
		t.Fatal("expected quota-exceeded rule on node one to be disabled")
	}

	updatedRuleTwo, err := database.GetRuleByID(ruleTwo.ID)
	if err != nil {
		t.Fatalf("get updated rule two: %v", err)
	}
	if !updatedRuleTwo.Enabled {
		t.Fatal("expected rule on unaffected node to remain enabled")
	}
}

func newTestHubDatabase(t *testing.T) *db.Database {
	t.Helper()

	database, err := db.New(t.TempDir() + "/flowgate-hub-test.db")
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
