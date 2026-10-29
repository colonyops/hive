package deeplink

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		raw  string
		want Location
	}{
		{"hive://code/hive-fix-auth", Location{Area: AreaCode, Slug: "hive-fix-auth"}},
		{"hive://code/web?window=@3&canvas=plan", Location{Area: AreaCode, Slug: "web", Window: "@3", Canvas: "plan"}},
		{"hive://code/web?server=box-1", Location{Area: AreaCode, Slug: "web", Server: "box-1"}},
		{"hive://code/a%2Fb", Location{Area: AreaCode, Slug: "a/b"}},
		{"hive://chats/research", Location{Area: AreaChats, Workspace: "research"}},
		{"hive://chats/research/42?canvas=notes", Location{Area: AreaChats, Workspace: "research", Chat: "42", Canvas: "notes"}},
		{"hive://inbox/work?feed=prs", Location{Area: AreaInbox, Profile: "work", Feed: "prs"}},
		{"hive://inbox/work/7", Location{Area: AreaInbox, Profile: "work", Item: 7}},
		{"hive://canvas/release%20plan?owner=colonyops/hive", Location{Area: AreaCanvas, Canvas: "release plan", Owner: "colonyops/hive"}},
		{"hive://canvas/todo?owner=@global", Location{Area: AreaCanvas, Canvas: "todo", Owner: "@global"}},
		{"hive://drive/colonyops/hive/plans/x.md", Location{Area: AreaDrive, Space: "colonyops/hive", Path: "plans/x.md"}},
		{"hive://drive/workspaces/hive/canvases/a.canvas.json", Location{Area: AreaDrive, Space: "workspaces/hive", Path: "canvases/a.canvas.json"}},
		{"hive://drive/shared/x.md", Location{Area: AreaDrive, Space: "shared", Path: "x.md"}},
		{"hive://drive/shared", Location{Area: AreaDrive, Space: "shared"}},
		{"hive://drive/colonyops/hive/", Location{Area: AreaDrive, Space: "colonyops/hive"}},
		{"hive://drive/colonyops/hive/release%20notes/v1.md", Location{Area: AreaDrive, Space: "colonyops/hive", Path: "release notes/v1.md"}},
		{"HIVE://Code/web/?window=@1&future=1", Location{Area: AreaCode, Slug: "web", Window: "@1"}},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := Parse(tt.raw)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParse_Rejects(t *testing.T) {
	tests := []string{
		"https://code/web",
		"hive:code/web",
		"hive://flows/work",
		"hive://code",
		"hive://code/web/extra",
		"hive://chats/research//42",
		"hive://chats/research?canvas=plan",
		"hive://inbox/work/abc",
		"hive://inbox/work/0",
		"hive://canvas/plan",
		"hive://drive",
		"hive://drive/workspaces",
		"hive://drive/colonyops",
		"hive://drive/shared/..",
		"hive://drive/shared/.git/config",
		"hive://drive/workspaces/a%2Fb/x.md",
	}

	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			_, err := Parse(raw)
			assert.Error(t, err)
		})
	}
}
