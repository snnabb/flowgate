package node

import (
	"testing"

	"github.com/flowgate/flowgate/internal/common"
)

func TestStartRuleRejectsWebSocketWithInboundTLS(t *testing.T) {
	t.Parallel()

	agent := NewAgent("ws://panel.example/ws/node", "test-key", false)
	rule := common.RuleConfig{
		ID:         1,
		Protocol:   "tcp",
		ListenPort: 0,
		TargetAddr: "127.0.0.1",
		TargetPort: 443,
		WSEnabled:  true,
		TLSMode:    "client",
	}

	err := agent.startRule(rule)
	if err == nil {
		agent.stopRule(rule.ID)
		t.Fatal("expected invalid WS + inbound TLS rule to be rejected")
	}
}
