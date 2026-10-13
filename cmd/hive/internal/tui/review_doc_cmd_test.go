package tui

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/cmd/hive/internal/plugins"
	"github.com/colonyops/hive/cmd/hive/internal/tui/views/review"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/hive/events/testbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHiveDocReviewCmd_nil_reviewView_shows_toast(t *testing.T) {
	tb := testbus.New(t)

	var received atomic.Int32
	tb.SubscribeNotificationPublished(func(_ events.NotificationPublishedPayload) {
		received.Add(1)
	})

	handler := NewKeybindingResolver(zerolog.Nop(), nil, plugins.NewCommandSet(map[string]config.UserCommand{}, nil), testRenderer)
	m := &Model{
		activeView:      ViewSessions,
		reviewView:      nil,
		handler:         handler,
		modals:          NewModalCoordinator(),
		bus:             tb.EventBus,
		toastController: NewToastController(),
		width:           100,
		height:          40,
	}

	cmd := HiveDocReviewCmd{Arg: ""}
	_ = cmd.Execute(m)

	tb.AssertPublished(t, events.EventNotificationPublished)
	assert.Equal(t, int32(1), received.Load(), "expected a warning notification to be published")
}

func TestHiveDocReviewCmd_Execute(t *testing.T) {
	// Create a mock model with review view
	docs := []review.Document{
		{
			Path:    "/test/plans/doc1.md",
			RelPath: "plans/doc1.md",
			Type:    "plan",
		},
		{
			Path:    "/test/research/doc2.md",
			RelPath: "research/doc2.md",
			Type:    "research",
		},
	}
	reviewView := review.New(zerolog.Nop(), nil, nil, docs, review.Scope{Dir: "/test"}, nil, nil, 0)
	reviewView.SetSize(100, 40)

	// Create a minimal handler for testing
	handler := NewKeybindingResolver(zerolog.Nop(), nil, plugins.NewCommandSet(map[string]config.UserCommand{}, nil), testRenderer)

	m := &Model{
		activeView: ViewSessions,
		reviewView: &reviewView,
		handler:    handler,
		modals:     NewModalCoordinator(),
		width:      100,
		height:     40,
	}

	// Execute command without argument - switches to review tab
	cmd := HiveDocReviewCmd{Arg: ""}
	_ = cmd.Execute(m)

	// Check that view switches to Docs tab
	assert.Equal(t, ViewReview, m.activeView)
}

func TestOpenDocument(t *testing.T) {
	spaceDir := t.TempDir()
	unlisted := filepath.Join(spaceDir, "notes", "new.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(unlisted), 0o755))
	require.NoError(t, os.WriteFile(unlisted, []byte("# new\n"), 0o644))

	space, err := drive.RepositorySpace("owner", "repo")
	require.NoError(t, err)
	tree := drive.Entry{Kind: drive.KindDir, Children: []drive.Entry{
		{Path: "plans", Kind: drive.KindDir, Children: []drive.Entry{{Path: "plans/doc1.md", Kind: drive.KindMarkdown}}},
		{Path: "research", Kind: drive.KindDir, Children: []drive.Entry{{Path: "research/doc2.md", Kind: drive.KindMarkdown}}},
	}}
	docs := review.DocumentsFromTree(space, spaceDir, tree, false)
	reviewView := review.New(zerolog.Nop(), nil, nil, docs, review.Scope{Space: space, Dir: spaceDir}, nil, nil, 0)
	reviewView.SetSymlinkName(".context")
	reviewView.SetSize(100, 40)

	tests := []struct {
		name     string
		path     string
		wantPath string
	}{
		{name: "full path", path: filepath.Join(spaceDir, "plans", "doc1.md"), wantPath: filepath.Join(spaceDir, "plans", "doc1.md")},
		{name: "space-relative path", path: "plans/doc1.md", wantPath: filepath.Join(spaceDir, "plans", "doc1.md")},
		{name: "path through the configured symlink", path: ".context/research/doc2.md", wantPath: filepath.Join(spaceDir, "research", "doc2.md")},
		{name: "path through another symlink name", path: ".hive/plans/doc1.md", wantPath: filepath.Join(spaceDir, "plans", "doc1.md")},
		{name: "unlisted file through the configured symlink", path: ".context/notes/new.md", wantPath: unlisted},
		{name: "basename", path: "doc2.md", wantPath: filepath.Join(spaceDir, "research", "doc2.md")},
		{name: "not found", path: "nonexistent.md"},
		{name: "not found through the configured symlink", path: ".context/notes/missing.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := reviewView.OpenDocumentByPath(tt.path)()

			openMsg, ok := msg.(review.OpenDocumentMsg)
			require.True(t, ok, "Expected OpenDocumentMsg, got %T", msg)
			if tt.wantPath == "" {
				assert.Error(t, openMsg.Err)
				return
			}
			require.NoError(t, openMsg.Err)
			assert.Equal(t, tt.wantPath, openMsg.Path)
		})
	}
}

// TestOpenDocument_SuffixMatch covers todo URIs relative to the repo root
// ("review://.hive/plans/doc.md") with no symlink name configured.
func TestOpenDocument_SuffixMatch(t *testing.T) {
	docs := []review.Document{
		{
			Path:    "/real/context/owner/repo/plans/doc1.md",
			RelPath: "plans/doc1.md",
			Type:    "plan",
		},
		{
			Path:    "/real/context/owner/repo/research/doc2.md",
			RelPath: "research/doc2.md",
			Type:    "research",
		},
	}
	reviewView := review.New(zerolog.Nop(), nil, nil, docs, review.Scope{Dir: "/real/context/owner/repo"}, nil, nil, 0)
	reviewView.SetSize(100, 40)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "repo-root-relative path matches context-dir-relative relpath",
			path:    ".hive/plans/doc1.md",
			wantErr: false,
		},
		{
			name:    "exact relpath still works",
			path:    "plans/doc1.md",
			wantErr: false,
		},
		{
			name:    "absolute path still works",
			path:    "/real/context/owner/repo/research/doc2.md",
			wantErr: false,
		},
		{
			name:    "not found",
			path:    ".hive/plans/nonexistent.md",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := reviewView.OpenDocumentByPath(tt.path)
			msg := cmd()

			openMsg, ok := msg.(review.OpenDocumentMsg)
			require.True(t, ok, "Expected OpenDocumentMsg, got %T", msg)
			if tt.wantErr {
				assert.Error(t, openMsg.Err, "Expected error but got none")
			} else {
				assert.NoError(t, openMsg.Err, "Expected no error but got: %v", openMsg.Err)
			}
		})
	}
}

func TestResolveReviewURI(t *testing.T) {
	cfg := &config.Config{DataDir: "/data"}
	cfg.Context.SymlinkName = ".hive"

	tests := []struct {
		name     string
		uriValue string
		remote   string
		want     string
	}{
		{
			name:     "resolves .hive/ prefixed path to context dir",
			uriValue: ".hive/plans/my-plan.md",
			remote:   "git@github.com:owner/repo.git",
			want:     "/data/context/owner/repo/plans/my-plan.md",
		},
		{
			name:     "resolves path without .hive/ prefix",
			uriValue: "plans/my-plan.md",
			remote:   "https://github.com/owner/repo.git",
			want:     "/data/context/owner/repo/plans/my-plan.md",
		},
		{
			name:     "returns uriValue when remote is empty",
			uriValue: ".hive/plans/my-plan.md",
			remote:   "",
			want:     ".hive/plans/my-plan.md",
		},
		{
			name:     "returns uriValue when remote cannot be parsed",
			uriValue: ".hive/plans/my-plan.md",
			remote:   "invalid",
			want:     ".hive/plans/my-plan.md",
		},
		{
			name:     "handles SSH remote with port",
			uriValue: ".hive/research/notes.md",
			remote:   "ssh://gitea@gitea.example.com:222/hay-kot/infra.git",
			want:     "/data/context/hay-kot/infra/research/notes.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveReviewURI(tt.uriValue, tt.remote, cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}
