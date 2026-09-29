package tasks

import (
	"strings"
	"testing"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/terminal"
)

func TestRenderDetailContent_WithBlockers(t *testing.T) {
	item := &hc.Item{
		ID:     "hc-abc123",
		Title:  "My Task",
		Type:   hc.ItemTypeTask,
		Status: hc.StatusOpen,
		EpicID: "hc-epic1",
	}
	blockers := []hc.Item{
		{ID: "hc-blk1", Title: "Blocker Task", Status: hc.StatusInProgress, Type: hc.ItemTypeTask, EpicID: "hc-epic1"},
	}

	content := renderDetailContent(item, nil, blockers, 80)
	plain := terminal.StripANSI(content)

	if !strings.Contains(plain, "Blockers (1)") {
		t.Errorf("expected 'Blockers (1)' in content, got:\n%s", plain)
	}
	if !strings.Contains(plain, "Blocker Task") {
		t.Errorf("expected 'Blocker Task' in content, got:\n%s", plain)
	}
	if !strings.Contains(plain, "hc-blk1") {
		t.Errorf("expected 'hc-blk1' in content, got:\n%s", plain)
	}
}

func TestRenderDetailContent_BlockedByChildren(t *testing.T) {
	item := &hc.Item{
		ID:      "hc-parent",
		Title:   "Parent Task",
		Type:    hc.ItemTypeTask,
		Status:  hc.StatusOpen,
		EpicID:  "hc-epic1",
		Blocked: true,
	}

	content := renderDetailContent(item, nil, nil, 80)
	plain := terminal.StripANSI(content)

	if !strings.Contains(plain, "Blocked by open children") {
		t.Errorf("expected 'Blocked by open children' in content, got:\n%s", plain)
	}
}

func TestRenderDetailContent_NotBlockedNoSection(t *testing.T) {
	item := &hc.Item{
		ID:     "hc-abc123",
		Title:  "My Task",
		Type:   hc.ItemTypeTask,
		Status: hc.StatusOpen,
		EpicID: "hc-epic1",
	}

	content := renderDetailContent(item, nil, nil, 80)
	plain := terminal.StripANSI(content)

	if strings.Contains(plain, "Blockers") {
		t.Errorf("expected no 'Blockers' section for non-blocked item, got:\n%s", plain)
	}
	if strings.Contains(plain, "Blocked by open children") {
		t.Errorf("expected no 'Blocked by open children' for non-blocked item, got:\n%s", plain)
	}
}
