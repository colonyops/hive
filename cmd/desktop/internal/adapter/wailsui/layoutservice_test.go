package wailsui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

func TestLayoutServiceReportReachesTheMirror(t *testing.T) {
	mirror := &app.LayoutsService{}
	svc := NewLayoutService(mirror)

	require.NoError(t, svc.Report(t.Context(), LayoutReport{
		Kind: "code", OnScreen: "abc123",
		Layouts: map[string]PanelLayoutWire{
			"abc123": {
				FocusedPanel: "p0",
				Root: PanelNodeWire{
					Kind: "split", ID: "s1", Axis: "row", Ratio: 0.4,
					Children: []PanelNodeWire{
						{Kind: "panel", ID: "p0", ActiveTab: "t0", Picking: true, Tabs: []PanelTabWire{{ID: "t0", Kind: "window", WindowID: "@3"}}},
						{Kind: "panel", ID: "p1"},
					},
				},
			},
		},
	}))

	got, onScreen, known := mirror.Reported("code", "abc123")
	require.True(t, known)
	assert.True(t, onScreen)
	assert.Equal(t, app.PanelLayoutReport{
		FocusedPanel: "p0",
		Root: app.PanelNodeReport{
			Kind: "split", ID: "s1", Axis: "row", Ratio: 0.4,
			Children: []app.PanelNodeReport{
				{Kind: "panel", ID: "p0", ActiveTab: "t0", Picking: true, Tabs: []app.PanelTabReport{{ID: "t0", Kind: "window", WindowID: "@3"}}},
				{Kind: "panel", ID: "p1"},
			},
		},
	}, got)

	err := svc.Report(t.Context(), LayoutReport{Kind: "nope"})
	assert.Equal(t, app.KindInvalid, app.KindOf(err))
}
