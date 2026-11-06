package drive

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKindOf(t *testing.T) {
	tests := []struct {
		name string
		want Kind
	}{
		{"perf.canvas.json", KindFile},
		{"batch-2026-02-20.json", KindFile},
		{"plan.md", KindMarkdown},
		{"README.MD", KindMarkdown},
		{"report.html", KindHtml},
		{"report.htm", KindFile},
		{"notes.txt", KindFile},
		{"Makefile", KindFile},
		{".md", KindFile},
		{"", KindFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, KindOf(tt.name))
		})
	}
}
