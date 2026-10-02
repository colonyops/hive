package app

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
)

// fakeItemSessionStore stands in for the durable link table. links is keyed by
// external id, which is enough to tell one item's sessions from another's here.
type fakeItemSessionStore struct {
	refs      map[int64]models.ItemRef
	links     map[string][]stores.ItemSession
	chats     map[string][]stores.ItemChat
	unlinked  []string
	unlinkErr error
}

func (f *fakeItemSessionStore) RefByID(_ context.Context, itemID int64) (models.ItemRef, error) {
	ref, ok := f.refs[itemID]
	if !ok {
		return models.ItemRef{}, stores.NotFoundError{Entity: "inbox_item", Key: fmt.Sprint(itemID)}
	}
	return ref, nil
}

func (f *fakeItemSessionStore) List(_ context.Context, ref models.ItemRef) ([]stores.ItemSession, error) {
	return f.links[ref.ExternalID], nil
}

func (f *fakeItemSessionStore) ListChats(_ context.Context, ref models.ItemRef) ([]stores.ItemChat, error) {
	return f.chats[ref.ExternalID], nil
}

func (f *fakeItemSessionStore) Unlink(_ context.Context, sessionIDs []string) error {
	if f.unlinkErr != nil {
		return f.unlinkErr
	}
	f.unlinked = append(f.unlinked, sessionIDs...)
	return nil
}

func itemSessionsService(manager *fakeSessionManager, links *fakeItemSessionStore) *SessionsService {
	return newSessionsService(SessionsDeps{Launcher: &fakeSessionLauncher{}, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}, Items: links, Links: links, Logger: zerolog.Nop()})
}

func TestSessionsService_ItemSessionsJoinsLinksToLiveHiveState(t *testing.T) {
	manager := &fakeSessionManager{
		sessions: []dispatch.SessionSummary{
			{ID: "s1", Name: "review 81 renamed", Slug: "review-81-renamed", Repo: "acme/site", State: "active"},
			{ID: "s2", Name: "review 81 rerun", Slug: "review-81-rerun", Repo: "acme/site", State: "recycled"},
		},
		running: map[string]bool{"s1": true},
	}
	links := &fakeItemSessionStore{
		refs: map[int64]models.ItemRef{7: {ProfileID: "p", SourceKind: "github", ExternalID: "acme/site#81"}},
		links: map[string][]stores.ItemSession{"acme/site#81": {
			{SessionID: "s2", CreatedAt: 200},
			{SessionID: "s1", CreatedAt: 100},
		}},
	}

	views, err := itemSessionsService(manager, links).ItemSessions(t.Context(), 7)
	require.NoError(t, err)
	require.Len(t, views, 2)

	// The link order is preserved, and everything but createdAt comes from
	// hive — so a session renamed outside this app reports its current name.
	assert.Equal(t, "s2", views[0].ID)
	assert.Equal(t, "recycled", views[0].State)
	assert.False(t, views[0].Running)
	assert.Equal(t, "s1", views[1].ID)
	assert.Equal(t, "review 81 renamed", views[1].Name)
	assert.Equal(t, "review-81-renamed", views[1].Slug)
	assert.True(t, views[1].Running)
	assert.Equal(t, int64(100), views[1].CreatedAt.UnixMilli())
}

// Nothing tells this app when a session is deleted from the CLI, so the read
// is what notices — and it must both hide and drop the link.
func TestSessionsService_ItemSessionsPrunesLinksHiveCannotAccountFor(t *testing.T) {
	manager := &fakeSessionManager{
		sessions: []dispatch.SessionSummary{{ID: "s1", Name: "kept", Slug: "kept", State: "active"}},
	}
	links := &fakeItemSessionStore{
		refs: map[int64]models.ItemRef{7: {ProfileID: "p", ExternalID: "acme/site#81"}},
		links: map[string][]stores.ItemSession{"acme/site#81": {
			{SessionID: "s1", CreatedAt: 100},
			{SessionID: "deleted-elsewhere", CreatedAt: 90},
		}},
	}

	views, err := itemSessionsService(manager, links).ItemSessions(t.Context(), 7)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, "s1", views[0].ID)
	assert.Equal(t, []string{"deleted-elsewhere"}, links.unlinked)
}

// A listing that failed proves nothing about what still exists; pruning on it
// would throw associations away because hive.db was momentarily unreadable.
func TestSessionsService_ItemSessionsKeepsLinksWhenHiveCannotBeRead(t *testing.T) {
	manager := &fakeSessionManager{err: errors.New("hive.db locked")}
	links := &fakeItemSessionStore{
		refs:  map[int64]models.ItemRef{7: {ProfileID: "p", ExternalID: "acme/site#81"}},
		links: map[string][]stores.ItemSession{"acme/site#81": {{SessionID: "s1", CreatedAt: 100}}},
	}

	_, err := itemSessionsService(manager, links).ItemSessions(t.Context(), 7)
	assert.Equal(t, KindInternal, KindOf(err))
	assert.Empty(t, links.unlinked)
}

// The view is already correct without the prune, so a failed cleanup must not
// hide the sessions that do still exist.
func TestSessionsService_ItemSessionsSurvivesAFailedPrune(t *testing.T) {
	manager := &fakeSessionManager{
		sessions: []dispatch.SessionSummary{{ID: "s1", Name: "kept", Slug: "kept", State: "active"}},
	}
	links := &fakeItemSessionStore{
		refs: map[int64]models.ItemRef{7: {ProfileID: "p", ExternalID: "acme/site#81"}},
		links: map[string][]stores.ItemSession{"acme/site#81": {
			{SessionID: "s1", CreatedAt: 100},
			{SessionID: "gone", CreatedAt: 90},
		}},
		unlinkErr: errors.New("disk full"),
	}

	views, err := itemSessionsService(manager, links).ItemSessions(t.Context(), 7)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, "s1", views[0].ID)
}

// Liveness is the last thing added to an otherwise complete answer, so losing
// it must not cost the sessions themselves — the frontend renders a failed read
// as an empty pane.
func TestSessionsService_ItemSessionsKeepsSessionsWhenLivenessCannotBeRead(t *testing.T) {
	manager := &fakeSessionManager{
		sessions:   []dispatch.SessionSummary{{ID: "s1", Name: "kept", Slug: "kept", State: "active"}},
		runningErr: errors.New("tmux is not reachable"),
	}
	links := &fakeItemSessionStore{
		refs:  map[int64]models.ItemRef{7: {ProfileID: "p", ExternalID: "acme/site#81"}},
		links: map[string][]stores.ItemSession{"acme/site#81": {{SessionID: "s1", CreatedAt: 100}}},
	}

	views, err := itemSessionsService(manager, links).ItemSessions(t.Context(), 7)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, "s1", views[0].ID)
	assert.False(t, views[0].Running)
}

func TestSessionsService_ItemSessionsRejectsAnUnknownItem(t *testing.T) {
	manager, _ := activeSession()
	links := &fakeItemSessionStore{refs: map[int64]models.ItemRef{}}

	_, err := itemSessionsService(manager, links).ItemSessions(t.Context(), 404)
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestSessionsService_CreateSessionCarriesEveryDraftedItemInOneLaunch(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	manager, _ := activeSession()
	first := models.ItemRef{ProfileID: "p", SourceKind: "github", ExternalID: "acme/site#81"}
	second := models.ItemRef{ProfileID: "p", SourceKind: "github", ExternalID: "acme/site#82"}
	fake := &fakeItemSessionStore{refs: map[int64]models.ItemRef{7: first, 8: second, 9: first}}
	svc := newSessionsService(SessionsDeps{Launcher: launcher, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}, Items: fake, Links: fake, Logger: zerolog.Nop()})

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "review-81", ItemIDs: []int64{7, 8, 7, 9}})
	require.NoError(t, err)
	require.Len(t, launcher.calls, 1)
	assert.Equal(t, []models.ItemRef{first, second}, launcher.calls[0].Origins)
}

// An item pruned between opening the form and submitting it must not cost the
// user the session they asked for.
func TestSessionsService_CreateSessionLaunchesUnlinkedWhenTheItemHasGone(t *testing.T) {
	launcher := &fakeSessionLauncher{}
	manager, _ := activeSession()
	fake := &fakeItemSessionStore{refs: map[int64]models.ItemRef{}}
	svc := newSessionsService(SessionsDeps{Launcher: launcher, Manager: manager, Statuses: manager, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}, Items: fake, Links: fake, Logger: zerolog.Nop()})

	_, err := svc.CreateSession(t.Context(), dispatch.CreateSessionRequest{Repository: "r", Name: "review-81", ItemIDs: []int64{404}})
	require.NoError(t, err)
	require.Len(t, launcher.calls, 1)
	assert.Empty(t, launcher.calls[0].Origins)
}

func TestSessionsService_ItemChatsListsLinkedChats(t *testing.T) {
	links := &fakeItemSessionStore{
		refs: map[int64]models.ItemRef{7: {ProfileID: "p", SourceKind: "github", ExternalID: "acme/site#81"}},
		chats: map[string][]stores.ItemChat{"acme/site#81": {
			{ChatID: 9, Workspace: "triage", Name: "launch-p-triage-acme-site-81", CreatedAt: 200},
		}},
	}

	views, err := itemSessionsService(&fakeSessionManager{}, links).ItemChats(t.Context(), 7)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, int64(9), views[0].ID)
	assert.Equal(t, "triage", views[0].Workspace)
	assert.Equal(t, int64(200), views[0].CreatedAt.UnixMilli())

	_, err = itemSessionsService(&fakeSessionManager{}, links).ItemChats(t.Context(), 8)
	assert.Equal(t, KindNotFound, KindOf(err))
}
