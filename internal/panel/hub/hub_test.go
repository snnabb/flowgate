package hub

import (
	"strings"
	"testing"

	"github.com/flowgate/flowgate/internal/common"
	"github.com/flowgate/flowgate/internal/panel/db"
	"github.com/flowgate/flowgate/internal/panel/model"
)

func TestTrafficQuotaExceededDisablesOwnedRules(t *testing.T) {
	t.Parallel()

	database := newTestHubDatabase(t)
	user, err := database.CreateUserWithOptions(&model.CreateUserRequest{
		Username:     "quota-owner",
		Role:         "user",
		TrafficQuota: 100,
		Ratio:        1,
	}, "hash-user")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	node, err := database.CreateNodeWithOwner(&model.CreateNodeRequest{Name: "quota-node"}, user.ID)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	rule, err := database.CreateRuleWithOwner(&model.CreateRuleRequest{
		NodeID:     node.ID,
		Name:       "quota-rule",
		Protocol:   "tcp",
		ListenPort: 36001,
		TargetAddr: "127.0.0.1",
		TargetPort: 8080,
		SpeedLimit: 64,
	}, user.ID)
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	h := New(database)
	h.handleNodeMessage(node.ID, &common.WSMessage{
		Action: common.ActionReportStats,
		Data: []common.TrafficReport{
			{RuleID: rule.ID, TrafficIn: 60, TrafficOut: 50},
		},
	})

	updatedRule, err := database.GetRuleByID(rule.ID)
	if err != nil {
		t.Fatalf("get updated rule: %v", err)
	}
	if updatedRule.Enabled {
		t.Fatal("expected quota-exceeded rule to be disabled")
	}

	updatedUser, err := database.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("get updated user: %v", err)
	}
	if updatedUser.TrafficUsed < updatedUser.TrafficQuota {
		t.Fatalf("expected traffic_used >= traffic_quota, got used=%d quota=%d", updatedUser.TrafficUsed, updatedUser.TrafficQuota)
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
