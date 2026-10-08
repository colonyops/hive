package hive_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/notify"
	"github.com/colonyops/hive/internal/domain/review"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/doctor"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/hive/events/testbus"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/store/db"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

type stubMux struct{}

func (stubMux) CreateSession(context.Context, multiplexer.SessionSpec) error { return nil }
func (stubMux) OpenSession(context.Context, multiplexer.SessionSpec, multiplexer.Target) error {
	return nil
}

func (stubMux) AddWindows(context.Context, multiplexer.Target, []multiplexer.WindowSpec) error {
	return nil
}

func (stubMux) AttachOrSwitch(context.Context, multiplexer.Target, multiplexer.AttachStreams) error {
	return nil
}

func (stubMux) CurrentSession(context.Context) (multiplexer.Target, error) {
	return multiplexer.Target{}, nil
}
func (stubMux) RenameSession(context.Context, multiplexer.Target, string) error { return nil }
func (stubMux) KillSession(context.Context, multiplexer.Target) error           { return nil }
func (stubMux) KillWindow(context.Context, multiplexer.Target) error            { return nil }

var _ sessionsvc.Multiplexer = stubMux{}

type stubPanes struct{}

func (stubPanes) ListPanes(context.Context) ([]multiplexer.Pane, error) { return nil, nil }

func (stubPanes) CapturePane(context.Context, multiplexer.Target, multiplexer.CaptureOptions) (string, error) {
	return "", nil
}

func loadConfig(t *testing.T, dataDir string) *config.Config {
	t.Helper()
	t.Setenv("HIVE_DEFAULT_AGENT", "")
	cfg, err := config.Load("", dataDir)
	require.NoError(t, err)
	return cfg
}

type harness struct {
	engine *hive.Engine
	db     *db.DB
	exec   *executiltest.Exec
	bus    *testbus.Bus
}

func newEngine(t *testing.T, cfg *config.Config, panes terminal.PaneSource) harness {
	t.Helper()
	database, err := hive.OpenDB(t.Context(), t.TempDir(), cfg.Database)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	h := harness{db: database, exec: &executiltest.Exec{}, bus: testbus.New(t)}
	h.engine, err = hive.New(cfg, hive.Ports{
		DB:         database,
		Bus:        h.bus.EventBus,
		Executor:   h.exec,
		Mux:        stubMux{},
		PaneSource: panes,
		DataDir:    t.TempDir(),
		Logger:     zerolog.Nop(),
	})
	require.NoError(t, err)
	return h
}

func TestEngineReloadRebuildsTheConfigDerivedServices(t *testing.T) {
	before := loadConfig(t, t.TempDir())
	h := newEngine(t, before, nil)

	after := loadConfig(t, t.TempDir())
	after.GitPath = "/opt/git/bin/git"
	require.NoError(t, h.engine.Reload(after))

	assert.Same(t, after, h.engine.Config())

	sess, err := h.engine.Sessions().CreateSession(t.Context(), sessionsvc.CreateOptions{
		Name: "review", Remote: "https://github.com/acme/site", SkipSpawn: true,
	})
	require.NoError(t, err)
	assert.Equal(t, after.ReposDir(), filepath.Dir(sess.Path), "the session service reads the reloaded ReposDir")

	_, _ = h.engine.Git().Branch(t.Context(), sess.Path)
	calls := h.exec.Calls()
	require.NotEmpty(t, calls)
	assert.Equal(t, "/opt/git/bin/git", calls[len(calls)-1].Cmd, "Git uses the reloaded GitPath")
	for _, call := range calls {
		assert.NotEqual(t, before.GitPath, call.Cmd, "nothing ran the old git after the reload: %s", strings.Join(call.Args, " "))
	}
}

func TestEngineReloadPublishesConfigReloadedOnce(t *testing.T) {
	h := newEngine(t, loadConfig(t, t.TempDir()), nil)
	next := loadConfig(t, t.TempDir())

	require.NoError(t, h.engine.Reload(next))

	require.True(t, h.bus.WaitFor(events.EventConfigReloaded, time.Second))
	var reloads []events.ConfigReloadedPayload
	for _, recorded := range h.bus.Events() {
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
	h := newEngine(t, loadConfig(t, t.TempDir()), nil)
	assert.Nil(t, h.engine.Status())
	assert.Nil(t, h.engine.Terminal())

	require.NoError(t, h.engine.Reload(loadConfig(t, t.TempDir())))
	assert.Nil(t, h.engine.Status())
	assert.Nil(t, h.engine.Terminal())
}

func TestEngineWithAPaneSourceHasStatus(t *testing.T) {
	h := newEngine(t, loadConfig(t, t.TempDir()), stubPanes{})
	require.NotNil(t, h.engine.Status())
	assert.True(t, h.engine.Status().Available())

	before := h.engine.Status()
	require.NoError(t, h.engine.Reload(loadConfig(t, t.TempDir())))
	assert.NotSame(t, before, h.engine.Status(), "status options come from config, so a reload rebuilds them")
}

// A save with a typo must not leave the program with no session service.
func TestEngineFailedReloadKeepsTheOldServices(t *testing.T) {
	original := loadConfig(t, t.TempDir())
	h := newEngine(t, original, nil)
	sessions, messages, gitExec := h.engine.Sessions(), h.engine.Messages(), h.engine.Git()

	broken := loadConfig(t, t.TempDir())
	broken.GitPath = ""
	require.Error(t, h.engine.Reload(broken))

	assert.Same(t, original, h.engine.Config())
	assert.Same(t, sessions, h.engine.Sessions())
	assert.Same(t, messages, h.engine.Messages())
	assert.Equal(t, gitExec, h.engine.Git())
	h.bus.AssertNotPublished(t, events.EventConfigReloaded, 50*time.Millisecond)
}

func TestEngineCapsMessagesPerTopic(t *testing.T) {
	cfg := loadConfig(t, t.TempDir())
	assert.Equal(t, 100, cfg.Messaging.MaxMessages)

	cfg.Messaging.MaxMessages = 2
	h := newEngine(t, cfg, nil)
	for _, payload := range []string{"one", "two", "three"} {
		_, err := h.engine.Messages().Publish(t.Context(), messaging.Message{Payload: payload}, []string{"inbox"})
		require.NoError(t, err)
	}

	kept, err := h.engine.Messages().Subscribe(t.Context(), "inbox", time.Time{})
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
	h := newEngine(t, loadConfig(t, t.TempDir()), nil)

	results := h.engine.Doctor(failingValidator{}, nil).RunChecks(t.Context(), "", false)

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

func TestEngineHoneycombSurvivesAReload(t *testing.T) {
	h := newEngine(t, loadConfig(t, t.TempDir()), nil)
	hc := h.engine.HC()

	require.NoError(t, h.engine.Reload(loadConfig(t, t.TempDir())))
	assert.Same(t, hc, h.engine.HC())
}

// Run under -race: readers fetch a service per call while reloads swap the set.
func TestEngineServesConcurrentReadersDuringReload(t *testing.T) {
	h := newEngine(t, loadConfig(t, t.TempDir()), stubPanes{})
	configs := make([]*config.Config, 20)
	for i := range configs {
		configs[i] = loadConfig(t, t.TempDir())
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
				if _, err := h.engine.Sessions().ListSessions(context.Background()); err != nil {
					t.Error(err)
					return
				}
				assert.NotNil(t, h.engine.Status())
				assert.NotNil(t, h.engine.Git())
				_ = h.engine.Config().ReposDir()
			}
		})
	}
	for _, cfg := range configs {
		require.NoError(t, h.engine.Reload(cfg))
	}
	close(stop)
	wg.Wait()
	assert.Same(t, configs[len(configs)-1], h.engine.Config())
}

func TestNewRejectsMissingPorts(t *testing.T) {
	_, err := hive.New(loadConfig(t, t.TempDir()), hive.Ports{})
	require.Error(t, err)
}

func TestEnginePersistenceSurvivesAReload(t *testing.T) {
	h := newEngine(t, loadConfig(t, t.TempDir()), nil)
	kvSvc, notifications, reviews := h.engine.KV(), h.engine.Notifications(), h.engine.Reviews()

	require.NoError(t, h.engine.Reload(loadConfig(t, t.TempDir())))
	assert.Equal(t, kvSvc, h.engine.KV())
	assert.Equal(t, notifications, h.engine.Notifications())
	assert.Equal(t, reviews, h.engine.Reviews())
}

func TestEngineKVPersists(t *testing.T) {
	h := newEngine(t, loadConfig(t, t.TempDir()), nil)
	ctx := t.Context()

	require.NoError(t, h.engine.KV().Set(ctx, "plugin.cache", "fresh"))

	var got string
	require.NoError(t, h.engine.KV().Get(ctx, "plugin.cache", &got))
	assert.Equal(t, "fresh", got)
}

// Reads already hide an expired entry, so only the table shows the sweep.
func TestEngineSweepKVDeletesExpiredRows(t *testing.T) {
	h := newEngine(t, loadConfig(t, t.TempDir()), nil)
	ctx, cancel := context.WithCancel(t.Context())

	require.NoError(t, h.engine.KV().Set(ctx, "plugin.cache", "fresh"))
	require.NoError(t, h.engine.KV().SetTTL(ctx, "update.check", "stale", time.Nanosecond))

	done := make(chan struct{})
	go func() {
		h.engine.SweepKV(ctx, time.Millisecond)
		close(done)
	}()

	assert.Eventually(t, func() bool {
		var rows int
		require.NoError(t, h.db.Conn().QueryRowContext(t.Context(), "SELECT count(*) FROM kv_store").Scan(&rows))
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
	h := newEngine(t, loadConfig(t, t.TempDir()), nil)
	ctx := t.Context()

	_, err := h.engine.Notifications().Save(ctx, notify.Notification{
		Level: notify.LevelError, Message: "spawn failed", CreatedAt: time.Now(),
	})
	require.NoError(t, err)

	saved, err := h.engine.Notifications().List(ctx)
	require.NoError(t, err)
	require.Len(t, saved, 1)
	assert.Equal(t, "spawn failed", saved[0].Message)
}

func TestEngineReviewsPersist(t *testing.T) {
	h := newEngine(t, loadConfig(t, t.TempDir()), nil)
	ctx := t.Context()

	created, err := h.engine.Reviews().CreateSession(ctx, "/ctx/plan.md", "abc123")
	require.NoError(t, err)
	require.NoError(t, h.engine.Reviews().SaveComment(ctx, review.Comment{
		ID: "c1", SessionID: created.ID, StartLine: 1, EndLine: 1, CommentText: "tighten this",
		CreatedAt: time.Now(),
	}))

	found, err := h.engine.Reviews().GetSessionByHash(ctx, "/ctx/plan.md", "abc123")
	require.NoError(t, err)
	assert.Equal(t, created.ID, found.ID)
	comments, err := h.engine.Reviews().ListComments(ctx, created.ID)
	require.NoError(t, err)
	require.Len(t, comments, 1)
	assert.Equal(t, "tighten this", comments[0].CommentText)
}
