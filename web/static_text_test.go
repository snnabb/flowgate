package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRulesRouteBuilderCopyIsReadableChinese(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join("static", "js", "components", "rules.js"))
	if err != nil {
		t.Fatalf("read rules.js: %v", err)
	}

	source := string(content)
	marker := "// Phase 2 route builder:"
	idx := strings.Index(source, marker)
	if idx == -1 {
		t.Fatalf("missing marker %q", marker)
	}

	routeBuilder := source[idx:]

	for _, good := range []string{
		"链路设置",
		"有序跳点",
		"自定义链路：后续跳点",
	} {
		if !strings.Contains(routeBuilder, good) {
			t.Fatalf("route builder copy missing %q", good)
		}
	}

	for _, bad := range []string{
		"闁炬崘鐭剧拋鍓х枂",
		"閺堝绨捄瀹犵儲",
		"瑜版挸澧犳禒鍛箽鐎涙ɑ瀵滄い鍝勭碍闁板秶鐤",
	} {
		if strings.Contains(routeBuilder, bad) {
			t.Fatalf("route builder copy still contains mojibake %q", bad)
		}
	}
}

func TestPhase3UsersPanelFieldsArePresent(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join("static", "js", "components", "users.js"))
	if err != nil {
		t.Fatalf("read users.js: %v", err)
	}

	source := string(content)
	for _, marker := range []string{
		"new-user-name",
		"new-user-password",
		"edit-user-enabled",
		"data-access-enabled",
		"data-access-quota",
		"data-access-bandwidth",
		"data-access-max-rules",
		"self-access-body",
		"self-rules-body",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("users panel missing phase3 marker %q", marker)
		}
	}

	for _, marker := range []string{
		"用户管理",
		"创建用户",
		"节点权限",
		"每节点规则数",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("users panel missing chinese copy %q", marker)
		}
	}

	for _, marker := range []string{
		"User Management",
		"Create User",
		"Assigned Nodes",
		"Quota Summary",
	} {
		if strings.Contains(source, marker) {
			t.Fatalf("users panel still contains english copy %q", marker)
		}
	}
}

func TestPhase3NodeUIMatchesAdminUserModel(t *testing.T) {
	t.Parallel()

	nodesContent, err := os.ReadFile(filepath.Join("static", "js", "components", "nodes.js"))
	if err != nil {
		t.Fatalf("read nodes.js: %v", err)
	}
	source := string(nodesContent)

	for _, marker := range []string{
		"showCreateNodeModal",
		"node-name",
		"showDeployCmd",
		"本节点规则",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("nodes panel missing simplified marker %q", marker)
		}
	}

	for _, marker := range []string{
		"node-owner",
		"node-group",
		"node-owner-filtered-users",
		"showNodeGroupsModal",
	} {
		if strings.Contains(source, marker) {
			t.Fatalf("nodes panel still contains removed marker %q", marker)
		}
	}

	for _, marker := range []string{
		"Delete Node",
		"Node created",
		"Loading...",
		"Rules on this node",
	} {
		if strings.Contains(source, marker) {
			t.Fatalf("nodes panel still contains english copy %q", marker)
		}
	}
}

func TestPhase3RuleUIMatchesAdminUserModel(t *testing.T) {
	t.Parallel()

	rulesContent, err := os.ReadFile(filepath.Join("static", "js", "components", "rules.js"))
	if err != nil {
		t.Fatalf("read rules.js: %v", err)
	}
	source := string(rulesContent)

	for _, marker := range []string{
		"showCreateRuleModal",
		"showEditRuleModal",
		"bandwidthKBToM",
		"转发规则",
		"搜索规则",
		"全部节点",
		"添加规则",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("rules panel missing simplified marker %q", marker)
		}
	}

	for _, marker := range []string{
		"owner_user_id",
		"褰撳墠鑺傜偣褰掑睘",
		"KB/s",
		"getSelectedRuleOwnerId",
		"renderRuleOwnerSummary",
		"Forwarding Rules",
		"Search rules",
		"All nodes",
		"Add Rule",
	} {
		if strings.Contains(source, marker) {
			t.Fatalf("rules panel still contains removed marker %q", marker)
		}
	}
}

func TestPhase3DashboardCopyIsChinese(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join("static", "js", "components", "dashboard.js"))
	if err != nil {
		t.Fatalf("read dashboard.js: %v", err)
	}

	source := string(content)
	for _, marker := range []string{
		"仪表盘",
		"最近事件",
		"在线节点",
		"剩余流量",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("dashboard missing chinese copy %q", marker)
		}
	}

	for _, marker := range []string{
		"Global admin overview",
		"Your assigned resources and traffic",
		"Recent Events",
		"Loading...",
	} {
		if strings.Contains(source, marker) {
			t.Fatalf("dashboard still contains english copy %q", marker)
		}
	}
}

func TestRuleUIStillUsesApiGetNodesMethod(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join("static", "js", "components", "rules.js"))
	if err != nil {
		t.Fatalf("read rules.js: %v", err)
	}

	source := string(content)
	if strings.Contains(source, "API.get节点s") {
		t.Fatalf("rules panel should not translate API.getNodes into API.get节点s")
	}
	if !strings.Contains(source, "API.getNodes") {
		t.Fatalf("rules panel should call API.getNodes when loading node data")
	}
}

func TestRuleUIDoesNotTranslateHelperIdentifiers(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join("static", "js", "components", "rules.js"))
	if err != nil {
		t.Fatalf("read rules.js: %v", err)
	}

	source := string(content)
	for _, bad := range []string{
		"format带宽Limit",
		"format流量WithLimit",
		"format延迟",
		"parse带宽M",
		"parse流量Limit",
		"format流量LimitInput",
		"testRule延迟",
		"_managedChain节点s",
	} {
		if strings.Contains(source, bad) {
			t.Fatalf("rules panel should not contain translated helper identifier %q", bad)
		}
	}
}

func TestShellAndLoginCopyAreChinese(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join("static", "js", "app.js"))
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}

	source := string(content)
	for _, marker := range []string{
		"端口转发控制面板",
		"仪表盘",
		"退出登录",
		"登录管理面板",
		"用户名",
		"密码",
		"创建首个管理员",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("app shell missing chinese copy %q", marker)
		}
	}

	for _, marker := range []string{
		"Port forwarding control panel",
		"<span>Dashboard</span>",
		">Logout<",
		"aria-label=\"Logout\"",
		"Sign in to the panel",
		"<label>Username</label>",
		"placeholder=\"Username\"",
		"Create the first admin",
		"Login failed:",
	} {
		if strings.Contains(source, marker) {
			t.Fatalf("app shell still contains english copy %q", marker)
		}
	}
}
