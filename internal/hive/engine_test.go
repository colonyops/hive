package hive_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/domain/notify"
	"github.com/colonyops/hive/internal/domain/review"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/doctor"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/hive/hivetest"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
)

func TestEngineReloadRebuildsTheConfigDerivedServices(t *testing.T) {
	before := hivetest.LoadConfig(t, t.TempDir())
	h := hivetest.NewEngine(t, hivetest.Options{Config: before})

	after := hivetest.LoadConfig(t, t.TempDir())
	after.GitPath = "/opt/git/bin/git"
	require.NoError(t, h.Engine.Reload(after))

	assert.Same(t, after, h.Engine.Config())

	sess, err := h.Engine.Sessions().CreateSession(t.Context(), sessionsvc.CreateOptions{
		Name: "review", Remote: "https://github.com/acme/site", SkipSpawn: true,
	})
	require.NoError(t, err)
	assert.Equal(t, after.ReposDir(), filepath.Dir(sess.Path), "the session service reads the reloaded ReposDir")

	_, _ = h.Engine.Git().Branch(t.Context(), sess.Path)
	calls := h.Exec.Calls()
	require.NotEmpty(t, calls)
	assert.Equal(t, "/opt/git/bin/git", calls[len(calls)-1].Cmd, "Git uses the reloaded GitPath")
	for _, call := range calls {
		assert.NotEqual(t, before.GitPath, call.Cmd, "nothing ran the old git after the reload: %s", strings.Join(call.Args, " "))
	}
}

func TestEngineReloadPublishesConfigReloadedOnce(t *testing.T) {
	h := hivetest.NewEngine(t, hivetest.Options{})
	next := hivetest.LoadConfig(t, t.TempDir())

	require.NoError(t, h.Engine.Reload(next))

	require.True(t, h.Bus.WaitFor(events.EventConfigReloaded, time.Second))
	var reloads []events.ConfigReloadedPayload
	for _, recorded := range h.Bus.Events() {
		if recorded.Event == events.EventConfigReloaded {
			reloads = append(reloads, recorded.Payload.(events.ConfigReloadedPayload))
		}
	}
	require.Len(t, reloads, 1)
	assert.Same(t, next, reloads[0].Config)
}

// The desktop's mock mode runs with no tmux to read, and status must stay off
// across a reload rather than come back with a nil source.
func TestEngineWithoutAPaneSourceHasNoStatus(t *testing.T) {
	h := hivetest.NewEngine(t, hivetest.Options{})
	assert.Nil(t, h.Engine.Status())
	assert.Nil(t, h.Engine.Terminal())

	require.NoError(t, h.Engine.Reload(hivetest.LoadConfig(t, t.TempDir())))
	assert.Nil(t, h.Engine.Status())
	assert.Nil(t, h.Engine.Terminal())
}

func TestEngineWithAPaneSourceHasStatus(t *testing.T) {
	h := hivetest.NewEngine(t, hivetest.Options{PaneSource: hivetest.PaneSource{}})
	require.NotNil(t, h.Engine.Status())
	assert.True(t, h.Engine.Status().Available())

	before := h.Engine.Status()
	require.NoError(t, h.Engine.Reload(hivetest.LoadConfig(t, t.TempDir())))
	assert.NotSame(t, before, h.Engine.Status(), "status options come from config, so a reload rebuilds them")
}

// A save with a typo must not leave the program with no session service.
func TestEngineFailedReloadKeepsTheOldServices(t *testing.T) {
	original := hivetest.LoadConfig(t, t.TempDir())
	h := hivetest.NewEngine(t, hivetest.Options{Config: original})
	sessions, messages, gitExec := h.Engine.Sessions(), h.Engine.Messages(), h.Engine.Git()

	broken := hivetest.LoadConfig(t, t.TempDir())
	broken.GitPath = ""
	require.Error(t, h.Engine.Reload(broken))

	assert.Same(t, original, h.Engine.Config())
	assert.Same(t, sessions, h.Engine.Sessions())
	assert.Same(t, messages, h.Engine.Messages())
	assert.Equal(t, gitExec, h.Engine.Git())
	h.Bus.AssertNotPublished(t, events.EventConfigReloaded, 50*time.Millisecond)
}

func TestEngineCapsMessagesPerTopic(t *testing.T) {
	cfg := hivetest.LoadConfig(t, t.TempDir())
	assert.Equal(t, 100, cfg.Messaging.MaxMessages)

	cfg.Messaging.MaxMessages = 2
	h := hivetest.NewEngine(t, hivetest.Options{Config: cfg})
	for _, payload := range []string{"one", "two", "three"} {
		_, err := h.Engine.Messages().Publish(t.Context(), messaging.Message{Payload: payload}, []string{"inbox"})
		require.NoError(t, err)
	}

	kept, err := h.Engine.Messages().Subscribe(t.Context(), "inbox", time.Time{})
	require.NoError(t, err)
	assert.Len(t, kept, 2)
}

type failingValidator struct{}

func (failingValidator) ValidateDeep(string) error {
	return errors.New("keybindings: unknown command")
}
func (failingValidator) Warnings() []config.ValidationWarning { return nil }

// A program's config can have sections the engine does not read, so doctor
// checks with the validator the program hands it.
func TestEngineDoctorChecksWithTheProgramsValidator(t *testing.T) {
	h := hivetest.NewEngine(t, hivetest.Options{})

	results := h.Engine.Doctor(failingValidator{}, nil).RunChecks(t.Context(), "", false)

	var details []string
	for _, result := range results {
		if result.Name != "Configuration" {
			continue
		}
		for _, item := range result.Items {
			if item.Status == doctor.StatusFail {
				details = append(details, item.Detail)
			}
		}
	}
	require.Len(t, details, 1)
	assert.Contains(t, details[0], "unknown command")
}

// The drive service holds the snapshot of the context root, so a reload
// keeps it and re-roots it rather than building a blank one.
func TestEngineDriveSurvivesAReloadAndFollowsTheContextDir(t *testing.T) {
	h := hivetest.NewEngine(t, hivetest.Options{})
	drive := h.Engine.Drive()

	next := hivetest.LoadConfig(t, t.TempDir())
	require.NoError(t, h.Engine.Reload(next))
	assert.Same(t, drive, h.Engine.Drive())
	assert.Equal(t, next.ContextDir(), h.Engine.Drive().Root())
}

func TestEngineHoneycombSurvivesAReload(t *testing.T) {
	h := hivetest.NewEngine(t, hivetest.Options{})
	hc := h.Engine.HC()

	require.NoError(t, h.Engine.Reload(hivetest.LoadConfig(t, t.TempDir())))
	assert.Same(t, hc, h.Engine.HC())
}

// Run under -race: readers fetch a service per call while reloads swap the set.
func TestEngineServesConcurrentReadersDuringReload(t *testing.T) {
	h := hivetest.NewEngine(t, hivetest.Options{PaneSource: hivetest.PaneSource{}})
	configs := make([]*config.Config, 20)
	for i := range configs {
		configs[i] = hivetest.LoadConfig(t, t.TempDir())
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := h.Engine.Sessions().ListSessions(context.Background()); err != nil {
					t.Error(err)
					return
				}
				assert.NotNil(t, h.Engine.Status())
				assert.NotNil(t, h.Engine.Git())
				_ = h.Engine.Config().ReposDir()
			}
		})
	}
	for _, cfg := range configs {
		require.NoError(t, h.Engine.Reload(cfg))
	}
	close(stop)
	wg.Wait()
	assert.Same(t, configs[len(configs)-1], h.Engine.Config())
}

func TestNewRejectsMissingPorts(t *testing.T) {
	_, err := hive.New(hivetest.LoadConfig(t, t.TempDir()), hive.Ports{})
	require.Error(t, err)
}

func TestEnginePersistenceSurvivesAReload(t *testing.T) {
	h := hivetest.NewEngine(t, hivetest.Options{})
	kvSvc, notifications, reviews := h.Engine.KV(), h.Engine.Notifications(), h.Engine.Reviews()

	require.NoError(t, h.Engine.Reload(hivetest.LoadConfig(t, t.TempDir())))
	assert.Equal(t, kvSvc, h.Engine.KV())
	assert.Equal(t, notifications, h.Engine.Notifications())
	assert.Equal(t, reviews, h.Engine.Reviews())
}

func TestEngineKVPersists(t *testing.T) {
	h := hivetest.NewEngine(t, hivetest.Options{})
	ctx := t.Context()

	require.NoError(t, h.Engine.KV().Set(ctx, "plugin.cache", "fresh"))

	var got string
	require.NoError(t, h.Engine.KV().Get(ctx, "plugin.cache", &got))
	assert.Equal(t, "fresh", got)
}

// Reads already hide an expired entry, so only the table shows the sweep.
func TestEngineSweepKVDeletesExpiredRows(t *testing.T) {
	h := hivetest.NewEngine(t, hivetest.Options{})
	ctx, cancel := context.WithCancel(t.Context())

	require.NoError(t, h.Engine.KV().Set(ctx, "plugin.cache", "fresh"))
	require.NoError(t, h.Engine.KV().SetTTL(ctx, "update.check", "stale", time.Nanosecond))

	done := make(chan struct{})
	go func() {
		h.Engine.SweepKV(ctx, time.Millisecond)
		close(done)
	}()

	assert.Eventually(t, func() bool {
		var rows int
		require.NoError(t, h.DB.Conn().QueryRowContext(t.Context(), "SELECT count(*) FROM kv_store").Scan(&rows))
		return rows == 1
	}, time.Second, time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SweepKV did not return after its context was canceled")
	}
}

func TestEngineNotificationsPersist(t *testing.T) {
	h := hivetest.NewEngine(t, hivetest.Options{})
	ctx := t.Context()

	_, err := h.Engine.Notifications().Save(ctx, notify.Notification{
		Level: notify.LevelError, Message: "spawn failed", CreatedAt: time.Now(),
	})
	require.NoError(t, err)

	saved, err := h.Engine.Notifications().List(ctx)
	require.NoError(t, err)
	require.Len(t, saved, 1)
	assert.Equal(t, "spawn failed", saved[0].Message)
}

func TestEngineReviewsPersist(t *testing.T) {
	h := hivetest.NewEngine(t, hivetest.Options{})
	ctx := t.Context()

	created, err := h.Engine.Reviews().CreateSession(ctx, "/ctx/plan.md", "abc123")
	require.NoError(t, err)
	require.NoError(t, h.Engine.Reviews().SaveComment(ctx, review.Comment{
		ID: "c1", SessionID: created.ID, StartLine: 1, EndLine: 1, CommentText: "tighten this",
		CreatedAt: time.Now(),
	}))

	found, err := h.Engine.Reviews().GetSessionByHash(ctx, "/ctx/plan.md", "abc123")
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
	comments, err := h.Engine.Reviews().ListComments(ctx, created.ID)
	require.NoError(t, err)
	require.Len(t, comments, 1)
	assert.Equal(t, "tighten this", comments[0].CommentText)
}
