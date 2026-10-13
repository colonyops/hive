package drive

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKindOf(t *testing.T) {
	tests := []struct {
		name string
		want Kind
	}{
		{"perf.canvas.json", KindCanvas},
		{"canvases/perf.canvas.json", KindCanvas},
		{"batch-2026-02-20.json", KindFile},
		{"plan.md", KindMarkdown},
		{"README.MD", KindMarkdown},
		{"report.html", KindHtml},
		{"report.htm", KindFile},
		{"notes.txt", KindFile},
		{"Makefile", KindFile},
		{".md", KindFile},
		{".canvas.json", KindFile},
		{"", KindFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, KindOf(tt.name))
		})
	}
}

func TestCanvasPath(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"perf", "canvases/perf.canvas.json"},
		{"release-notes", "canvases/release-notes.canvas.json"},
		{"perf.report", "canvases/perf.report.canvas.json"},
		{"v2_draft", "canvases/v2_draft.canvas.json"},
		{"a", "canvases/a.canvas.json"},
		{"reports/2026-10/perf", "reports/2026-10/perf.canvas.json"},
		{"Reports/perf", "Reports/perf.canvas.json"},
		{"canvases/perf", "canvases/perf.canvas.json"},
		{"perf.canvas.json", "canvases/perf.canvas.json"},
		{"reports/perf.canvas.json", "reports/perf.canvas.json"},
		{"./reports/perf", "reports/perf.canvas.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CanvasPath(tt.name)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			again, err := CanvasPath(got)
			require.NoError(t, err)
			assert.Equal(t, got, again, "a document path maps to itself")
		})
	}
}

func TestCanvasName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"canvases/perf.canvas.json", "perf"},
		{"canvases/release-notes.canvas.json", "release-notes"},
		{"reports/2026-10/perf.canvas.json", "reports/2026-10/perf"},
		{"canvases/sub/perf.canvas.json", "canvases/sub/perf"},
		{"perf.canvas.json", "./perf"},
		{"perf", "./perf"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, CanvasName(tt.path))
		})
	}
	for _, name := range []string{"perf", "reports/2026-10/perf", "canvases/sub/perf", "./perf"} {
		path, err := CanvasPath(name)
		require.NoError(t, err)
		assert.Equal(t, name, CanvasName(path), "CanvasName inverts CanvasPath")
	}
	for _, path := range []string{"canvases/x.canvas.json", "reports/x.canvas.json", "perf.canvas.json"} {
		got, err := CanvasPath(CanvasName(path))
		require.NoError(t, err)
		assert.Equal(t, path, got, "CanvasPath inverts CanvasName")
	}
}

func TestCanvasPathRejects(t *testing.T) {
	long := strings.Repeat("a", maxStemLength+1)
	for _, name := range []string{
		"", ".", "..", "../escape", "reports/../perf", "reports/./perf", ".hidden", "reports/.hidden",
		"-plan", "plan-", "Report", "reports/Perf", "has space", "/perf", "reports//perf", "reports/", long,
		"./", "./../perf", "././perf", "./Perf",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := CanvasPath(name)
			require.ErrorIs(t, err, ErrInvalidPath)
		})
	}
}
