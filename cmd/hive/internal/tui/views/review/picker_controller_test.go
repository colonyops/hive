package review

import (
	"testing"
	"time"
)

func TestPickerController_GetLatest(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		documents []Document
		wantPath  string // expected RelPath of latest document, empty if nil expected
	}{
		{
			name:      "returns nil when no documents",
			documents: []Document{},
			wantPath:  "",
		},
		{
			name: "returns single document when only one exists",
			documents: []Document{
				{RelPath: "doc1.md", ModTime: now},
			},
			wantPath: "doc1.md",
		},
		{
			name: "returns newest when multiple documents",
			documents: []Document{
				{RelPath: "old.md", ModTime: now.Add(-2 * time.Hour)},
				{RelPath: "newest.md", ModTime: now},
				{RelPath: "older.md", ModTime: now.Add(-1 * time.Hour)},
			},
			wantPath: "newest.md",
		},
		{
			name: "handles identical timestamps (returns first encountered)",
			documents: []Document{
				{RelPath: "doc1.md", ModTime: now},
				{RelPath: "doc2.md", ModTime: now},
				{RelPath: "doc3.md", ModTime: now},
			},
			wantPath: "doc1.md",
		},
		{
			name: "handles documents with same timestamp and one newer",
			documents: []Document{
				{RelPath: "doc1.md", ModTime: now},
				{RelPath: "doc2.md", ModTime: now.Add(1 * time.Second)},
				{RelPath: "doc3.md", ModTime: now},
			},
			wantPath: "doc2.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewPickerController(tt.documents)
			got := pc.GetLatest()

			if tt.wantPath == "" {
				if got != nil {
					t.Errorf("GetLatest() = %v, want nil", got)
				}
			} else {
				if got == nil {
					t.Errorf("GetLatest() = nil, want document with path %q", tt.wantPath)
				} else if got.RelPath != tt.wantPath {
					t.Errorf("GetLatest() returned document with path %q, want %q", got.RelPath, tt.wantPath)
				}
			}
		})
	}
}
