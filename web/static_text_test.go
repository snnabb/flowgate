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
		"自定义链路：后续跳点需手动配置转发",
	} {
		if !strings.Contains(routeBuilder, good) {
			t.Fatalf("route builder copy missing %q", good)
		}
	}

	for _, bad := range []string{
		"閾捐矾璁剧疆",
		"鏈夊簭璺宠烦",
		"褰撳墠浠呬繚瀛樻寜椤哄簭閰嶇疆",
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
		"new-user-role",
		"new-user-parent",
		"new-user-traffic-quota",
		"new-user-ratio",
		"new-user-expires-at",
		"new-user-max-rules",
		"new-user-bandwidth-limit",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("users panel missing phase3 marker %q", marker)
		}
	}
}

func TestPhase3OwnershipUIIsPresent(t *testing.T) {
	t.Parallel()

	nodesContent, err := os.ReadFile(filepath.Join("static", "js", "components", "nodes.js"))
	if err != nil {
		t.Fatalf("read nodes.js: %v", err)
	}
	rulesContent, err := os.ReadFile(filepath.Join("static", "js", "components", "rules.js"))
	if err != nil {
		t.Fatalf("read rules.js: %v", err)
	}

	for _, marker := range []string{
		"node-owner",
		"owner_user_id",
		"node-owner-filtered-users",
	} {
		if !strings.Contains(string(nodesContent), marker) {
			t.Fatalf("nodes panel missing ownership marker %q", marker)
		}
	}

	for _, marker := range []string{
		"rule-owner-summary",
		"owner_user_id",
		"selected node owner",
	} {
		if !strings.Contains(string(rulesContent), marker) {
			t.Fatalf("rules panel missing ownership marker %q", marker)
		}
	}
}
