package wire

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive/status"
)

func TestAgentPanesCrossTheWire(t *testing.T) {
	in := map[string][]status.AgentPane{
		"s1": {
			{PaneID: "%3", WindowName: "claude", Status: terminal.StatusReady, Tool: "claude"},
			{PaneID: "%7", WindowName: "codex", Status: terminal.StatusApproval, Tool: "codex"},
		},
		"s2": {},
	}
	assert.Equal(t, in, AgentPanesFromProto(AgentPanesToProto(in)))
}
