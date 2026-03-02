package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderItem(t *testing.T) {
	// Fixed reference time for deterministic "ago" output.
	// timeutil.Ago uses time.Since, so we use real recent times
	// and strip the timestamps in the golden file comparison.
	// Instead, we'll test structural output by stripping ANSI.

	now := time.Now()

	tests := []struct {
		name      string
		item      hc.Item
		comments  []hc.Comment
		epicTitle string
	}{
		{
			name: "full",
			item: hc.Item{
				ID:        "hc-abc12345",
				Title:     "JWT middleware",
				Type:      hc.ItemTypeTask,
				Status:    hc.StatusInProgress,
				EpicID:    "hc-epic1234",
				ParentID:  "hc-epic1234",
				Depth:     1,
				Desc:      "Validate tokens on protected routes",
				SessionID: "session-mighty-oak",
			},
			comments: []hc.Comment{
				{
					ID:        "hcc-001",
					ItemID:    "hc-abc12345",
					Message:   "Started implementing middleware validation logic",
					CreatedAt: now.Add(-2 * time.Hour),
				},
				{
					ID:        "hcc-002",
					ItemID:    "hc-abc12345",
					Message:   "CHECKPOINT: middleware wired, need to add refresh token handling",
					CreatedAt: now.Add(-12 * time.Minute),
				},
			},
			epicTitle: "Implement JWT authentication",
		},
		{
			name: "minimal",
			item: hc.Item{
				ID:     "hc-xyz98765",
				Title:  "Set up auth system",
				Type:   hc.ItemTypeEpic,
				Status: hc.StatusOpen,
			},
			comments:  nil,
			epicTitle: "",
		},
		{
			name: "long_text_wrapping",
			item: hc.Item{
				ID:       "hc-wrap1234",
				Title:    "Implement text wrapping",
				Type:     hc.ItemTypeTask,
				Status:   hc.StatusInProgress,
				EpicID:   "hc-epic1234",
				ParentID: "hc-epic1234",
				Depth:    1,
				Desc:     "This is a very long description that should wrap at the terminal width boundary so that continuation lines are properly indented and aligned with the first line of text in the description area.",
			},
			comments: []hc.Comment{
				{
					ID:        "hcc-010",
					ItemID:    "hc-wrap1234",
					Message:   "This is a long comment message that needs to wrap properly within the thread body so that each continuation line is prefixed with the pipe character and maintains visual alignment.",
					CreatedAt: now.Add(-30 * time.Minute),
				},
			},
			epicTitle: "Implement JWT authentication",
		},
		{
			name: "done_no_comments",
			item: hc.Item{
				ID:       "hc-done1234",
				Title:    "Write unit tests",
				Type:     hc.ItemTypeTask,
				Status:   hc.StatusDone,
				EpicID:   "hc-epic1234",
				ParentID: "hc-epic1234",
				Depth:    1,
				Desc:     "Cover all edge cases",
			},
			comments:  nil,
			epicTitle: "Implement JWT authentication",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			renderItem(&buf, tt.item, tt.comments, tt.epicTitle)

			got := terminal.StripANSI(buf.String())

			golden := filepath.Join("testdata", "render_item_"+tt.name+".golden")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				require.NoError(t, os.MkdirAll("testdata", 0o755))
				require.NoError(t, os.WriteFile(golden, []byte(got), 0o644))
			}

			want, err := os.ReadFile(golden)
			require.NoError(t, err, "golden file missing — run with UPDATE_GOLDEN=1 to create it")
			assert.Equal(t, string(want), got)
		})
	}
}
