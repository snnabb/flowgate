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
	marker := "// Phase 2 route builder overrides:"
	idx := strings.Index(source, marker)
	if idx == -1 {
		t.Fatalf("missing marker %q", marker)
	}

	routeBuilder := source[idx:]

	for _, good := range []string{
		"链路设置",
		"有序跳点",
		"当前仅保存按顺序配置的跳点链路",
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
