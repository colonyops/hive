package tui

import (
	"os"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/core/terminal"
)

func TestMain(m *testing.M) {
	// Use a fixed theme so golden output is deterministic across machines.
	p, ok := styles.GetPalette("tokyo-night")
	if !ok {
		panic("tokyo-night theme not found")
	}
	styles.SetTheme(p)
	os.Exit(m.Run())
}

const dangerMessage = "This session has work that will be permanently lost:\n\n  • Uncommitted changes\n  • Unpushed commits"

// TestModal_Dangerous_Empty tests the dangerous modal before any text is typed.
func TestModal_Dangerous_Empty(t *testing.T) {
	m := NewDangerousModal("Delete Session?", dangerMessage, "delete")
	output := terminal.StripANSI(m.render())
	golden.RequireEqual(t, []byte(output))
}

// TestModal_Dangerous_Partial tests the dangerous modal with partial input ("del").
func TestModal_Dangerous_Partial(t *testing.T) {
	m := NewDangerousModal("Delete Session?", dangerMessage, "delete")
	for _, ch := range "del" {
		m.AddChar(string(ch))
	}
	output := terminal.StripANSI(m.render())
	golden.RequireEqual(t, []byte(output))
}

// TestModal_Dangerous_Ready tests the dangerous modal when the full word is typed.
func TestModal_Dangerous_Ready(t *testing.T) {
	m := NewDangerousModal("Delete Session?", dangerMessage, "delete")
	for _, ch := range "delete" {
		m.AddChar(string(ch))
	}
	output := terminal.StripANSI(m.render())
	golden.RequireEqual(t, []byte(output))
}

// TestModal_Dangerous_OnlyUncommitted tests with only uncommitted changes risk.
func TestModal_Dangerous_OnlyUncommitted(t *testing.T) {
	m := NewDangerousModal("Delete Session?",
		"This session has work that will be permanently lost:\n\n  • Uncommitted changes",
		"delete",
	)
	output := terminal.StripANSI(m.render())
	golden.RequireEqual(t, []byte(output))
}

// TestModal_Dangerous_Recycle tests the dangerous modal for a recycle action.
func TestModal_Dangerous_Recycle(t *testing.T) {
	m := NewDangerousModal("Recycle Session?", dangerMessage, "recycle")
	output := terminal.StripANSI(m.render())
	golden.RequireEqual(t, []byte(output))
}

// TestModal_Normal tests the standard (non-dangerous) confirm modal.
func TestModal_Normal(t *testing.T) {
	m := NewModal("Confirm", "Permanently delete 3 recycled session(s)?")
	output := terminal.StripANSI(m.render())
	golden.RequireEqual(t, []byte(output))
}
