package review

import (
	"testing"
	"time"
)

func TestPickerController_Filter(t *testing.T) {
	now := time.Now()
	documents := []Document{
		{RelPath: ".hive/plans/feature.md", Path: "/repo/.hive/plans/feature.md", ModTime: now},
		{RelPath: ".hive/research/api.md", Path: "/repo/.hive/research/api.md", ModTime: now},
		{RelPath: ".hive/context/notes.txt", Path: "/repo/.hive/context/notes.txt", ModTime: now},
		{RelPath: ".hive/plans/API-design.md", Path: "/repo/.hive/plans/API-design.md", ModTime: now},
	}

	pc := NewPickerController(documents)

	tests := []struct {
		name  string
		query string
		want  int // expected number of matches
	}{
		{
			name:  "empty query returns all documents",
			query: "",
			want:  4,
		},
		{
			name:  "case insensitive matching on RelPath",
			query: "PLANS",
			want:  2,
		},
		{
			name:  "case insensitive matching on filename",
			query: "api",
			want:  2, // matches both "api.md" and "API-design.md"
		},
		{
			name:  "partial match works",
			query: "feat",
			want:  1,
		},
		{
			name:  "no matches returns empty slice",
			query: "nonexistent",
			want:  0,
		},
		{
			name:  "matches on full path",
			query: "/repo/",
			want:  4,
		},
		{
			name:  "matches on extension",
			query: ".txt",
			want:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pc.Filter(tt.query)
			if len(got) != tt.want {
				t.Errorf("Filter(%q) returned %d documents, want %d", tt.query, len(got), tt.want)
			}
		})
	}
}

func TestPickerController_IsRecent(t *testing.T) {
	now := time.Now()
	pc := NewPickerController(nil)

	tests := []struct {
		name    string
		modTime time.Time
		want    bool
	}{
		{
			name:    "document modified 1 hour ago is recent",
			modTime: now.Add(-1 * time.Hour),
			want:    true,
		},
		{
			name:    "document modified 23 hours ago is recent",
			modTime: now.Add(-23 * time.Hour),
			want:    true,
		},
		{
			name:    "document modified just under 24 hours ago is recent (boundary)",
			modTime: now.Add(-24*time.Hour + 1*time.Second),
			want:    true,
		},
		{
			name:    "document modified 25 hours ago is not recent",
			modTime: now.Add(-25 * time.Hour),
			want:    false,
		},
		{
			name:    "document modified 48 hours ago is not recent",
			modTime: now.Add(-48 * time.Hour),
			want:    false,
		},
		{
			name:    "document modified in future is recent",
			modTime: now.Add(1 * time.Hour),
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Document{ModTime: tt.modTime}
			got := pc.IsRecent(doc)
			if got != tt.want {
				t.Errorf("IsRecent(doc with ModTime %v) = %v, want %v", tt.modTime, got, tt.want)
			}
		})
	}
}

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

func TestPickerController_SortByModTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		documents []Document
		wantOrder []string // expected order of RelPaths (newest first)
	}{
		{
			name:      "empty slice",
			documents: []Document{},
			wantOrder: []string{},
		},
		{
			name: "single document",
			documents: []Document{
				{RelPath: "doc1.md", ModTime: now},
			},
			wantOrder: []string{"doc1.md"},
		},
		{
			name: "multiple documents in random order",
			documents: []Document{
				{RelPath: "middle.md", ModTime: now.Add(-1 * time.Hour)},
				{RelPath: "newest.md", ModTime: now},
				{RelPath: "oldest.md", ModTime: now.Add(-2 * time.Hour)},
			},
			wantOrder: []string{"newest.md", "middle.md", "oldest.md"},
		},
		{
			name: "documents already sorted (newest first)",
			documents: []Document{
				{RelPath: "newest.md", ModTime: now},
				{RelPath: "middle.md", ModTime: now.Add(-1 * time.Hour)},
				{RelPath: "oldest.md", ModTime: now.Add(-2 * time.Hour)},
			},
			wantOrder: []string{"newest.md", "middle.md", "oldest.md"},
		},
		{
			name: "documents sorted in reverse (oldest first)",
			documents: []Document{
				{RelPath: "oldest.md", ModTime: now.Add(-2 * time.Hour)},
				{RelPath: "middle.md", ModTime: now.Add(-1 * time.Hour)},
				{RelPath: "newest.md", ModTime: now},
			},
			wantOrder: []string{"newest.md", "middle.md", "oldest.md"},
		},
		{
			name: "preserves stability for identical timestamps",
			documents: []Document{
				{RelPath: "doc1.md", ModTime: now},
				{RelPath: "doc2.md", ModTime: now},
				{RelPath: "doc3.md", ModTime: now},
			},
			wantOrder: []string{"doc1.md", "doc2.md", "doc3.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewPickerController(tt.documents)
			got := pc.SortByModTime()

			// Verify length
			if len(got) != len(tt.wantOrder) {
				t.Fatalf("SortByModTime() returned %d documents, want %d", len(got), len(tt.wantOrder))
			}

			// Verify order
			for i, want := range tt.wantOrder {
				if got[i].RelPath != want {
					t.Errorf("SortByModTime()[%d] = %q, want %q", i, got[i].RelPath, want)
				}
			}

			// Verify original slice is unchanged
			if len(pc.documents) > 0 && len(tt.documents) > 0 {
				if pc.documents[0].RelPath != tt.documents[0].RelPath {
					t.Error("SortByModTime() modified original documents slice")
				}
			}
		})
	}
}
