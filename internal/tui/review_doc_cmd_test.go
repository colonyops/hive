package tui

import (
	"sync/atomic"
	"testing"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/eventbus/testbus"
	"github.com/colonyops/hive/internal/tui/views/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHiveDocReviewCmd_nil_reviewView_shows_toast(t *testing.T) {
	tb := testbus.New(t)

	var received atomic.Int32
	tb.SubscribeNotificationPublished(func(_ eventbus.NotificationPublishedPayload) {
		received.Add(1)
	})

	handler := NewKeybindingResolver(nil, map[string]config.UserCommand{}, testRenderer)
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

	tb.AssertPublished(t, eventbus.EventNotificationPublished)
	assert.Equal(t, int32(1), received.Load(), "expected a warning notification to be published")
}

func TestHiveDocReviewCmd_Execute(t *testing.T) {
	// Create a mock model with review view
	docs := []review.Document{
		{
			Path:    "/test/doc1.md",
			RelPath: ".hive/plans/doc1.md",
			Type:    review.DocTypePlan,
		},
		{
			Path:    "/test/doc2.md",
			RelPath: ".hive/research/doc2.md",
			Type:    review.DocTypeResearch,
		},
	}
	reviewView := review.New(docs, "/test", nil, 0)
	reviewView.SetSize(100, 40)

	// Create a minimal handler for testing
	handler := NewKeybindingResolver(nil, map[string]config.UserCommand{}, testRenderer)

	m := &Model{
		activeView: ViewSessions,
		reviewView: &reviewView,
		handler:    handler,
		modals:     NewModalCoordinator(),
		width:      100,
		height:     40,
	}

	// Execute command without argument - falls back to review view documents
	cmd := HiveDocReviewCmd{Arg: ""}
	_ = cmd.Execute(m)

	// Check that view stays on Sessions (picker shown on Sessions view)
	assert.Equal(t, ViewSessions, m.activeView)

	// Check that picker modal is shown on the Model (using fallback docs)
	require.NotNil(t, m.modals.DocPicker, "Expected picker modal to be created on Model")
}

func TestOpenDocument(t *testing.T) {
	docs := []review.Document{
		{
			Path:    "/test/doc1.md",
			RelPath: ".hive/plans/doc1.md",
			Type:    review.DocTypePlan,
		},
		{
			Path:    "/test/doc2.md",
			RelPath: ".hive/research/doc2.md",
			Type:    review.DocTypeResearch,
		},
	}
	reviewView := review.New(docs, "/test", nil, 0)
	reviewView.SetSize(100, 40)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "find by full path",
			path:    "/test/doc1.md",
			wantErr: false,
		},
		{
			name:    "find by relative path",
			path:    ".hive/plans/doc1.md",
			wantErr: false,
		},
		{
			name:    "find by basename",
			path:    "doc2.md",
			wantErr: false,
		},
		{
			name:    "not found",
			path:    "nonexistent.md",
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

func TestGetAllDocuments(t *testing.T) {
	docs := []review.Document{
		{RelPath: "doc1.md", Type: review.DocTypePlan},
		{RelPath: "doc2.md", Type: review.DocTypeResearch},
	}

	reviewView := review.New(docs, "/test", nil, 0)

	allDocs := reviewView.GetAllDocuments()

	// Should return all non-header documents
	assert.Len(t, allDocs, 2, "Expected 2 documents, got %d", len(allDocs))
}
