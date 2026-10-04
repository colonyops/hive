package app

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/prompt"
)

type fakeOrchLauncher struct {
	got dispatch.LaunchSessionRequest
	err error
}

func (f *fakeOrchLauncher) LaunchSession(_ context.Context, req dispatch.LaunchSessionRequest) (dispatch.SessionExecutionOutcome, error) {
	f.got = req
	return dispatch.SessionExecutionOutcome{ID: "s1", Name: req.Name}, f.err
}

func (f *fakeOrchStatuses) SessionLaunchOptions(context.Context) (dispatch.SessionLaunchOptions, error) {
	return dispatch.SessionLaunchOptions{Repositories: []dispatch.SessionLaunchRepository{
		{Name: "hive", Repository: "git@github.com:colonyops/hive.git"},
	}}, nil
}

type fakeOrchDriver struct {
	sessions []session.Session
	sendErr  error
}

func (f *fakeOrchDriver) ListSessions(context.Context) ([]session.Session, error) {
	return f.sessions, nil
}

func (f *fakeOrchDriver) GetSession(_ context.Context, id string) (session.Session, error) {
	return session.Session{ID: id}, nil
}

func (f *fakeOrchDriver) Peek(context.Context, session.Session, int) (prompt.SessionPeek, error) {
	return prompt.SessionPeek{}, nil
}

func (f *fakeOrchDriver) Send(context.Context, session.Session, string, prompt.SendOptions) (prompt.SessionPeek, error) {
	return prompt.SessionPeek{}, f.sendErr
}

func (f *fakeOrchDriver) SendKeys(context.Context, session.Session, []string, int) (prompt.SessionPeek, error) {
	return prompt.SessionPeek{}, nil
}

// fakeOrchStatuses replays one status per call, then repeats the last.
// A script entry of "error" fails that poll.
type fakeOrchStatuses struct {
	mu     sync.Mutex
	script []string
	calls  int
}

func (f *fakeOrchStatuses) SessionStatuses(context.Context) (SessionStatusSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	status := f.script[min(f.calls, len(f.script)-1)]
	f.calls++
	if status == "error" {
		return SessionStatusSnapshot{}, fmt.Errorf("tmux list-windows failed")
	}
	return SessionStatusSnapshot{
		PollIntervalMS: 1,
		Items: []SessionStatus{{
			SessionID: "s1", Running: true,
			Windows: []SessionWindowStatus{{Status: status}},
		}},
	}, nil
}

type fakeOrchInbox struct {
	mu     sync.Mutex
	unread []messaging.Message
	acked  []string
	// gate, when set, holds the first GetUnread after it has read.
	gate chan struct{}
}

func (f *fakeOrchInbox) Publish(_ context.Context, _ messaging.Message, topics []string) (messaging.PublishResult, error) {
	return messaging.PublishResult{Topics: topics}, nil
}

func (f *fakeOrchInbox) GetUnread(context.Context, string, string) ([]messaging.Message, error) {
	f.mu.Lock()
	unread, gate := f.unread, f.gate
	f.gate = nil
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	return unread, nil
}

func (f *fakeOrchInbox) Acknowledge(_ context.Context, _ string, ids []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acked = append(f.acked, ids...)
	f.unread = nil
	return nil
}

func newTestOrchestration(launcher *fakeOrchLauncher, driver *fakeOrchDriver, statuses *fakeOrchStatuses, inbox *fakeOrchInbox, done chan struct{}) *OrchestrationService {
	return newOrchestrationService(OrchestrationDeps{
		Launcher: launcher, Sessions: statuses,
		Hive: func() hiveSessions { return driver }, Prompts: driver,
		Messages: func() messageBus { return inbox },
		Done:     done, Logger: zerolog.Nop(),
	})
}

var testCaller = OrchestratorCaller{Session: 1, Workspace: "orchestrator", authorized: true}

func TestOrchestrationRejectsAnUnauthorizedCaller(t *testing.T) {
	launcher := &fakeOrchLauncher{}
	svc := newTestOrchestration(launcher, &fakeOrchDriver{}, &fakeOrchStatuses{script: []string{"ready"}}, &fakeOrchInbox{}, nil)
	forged := OrchestratorCaller{Session: 1, Workspace: "orchestrator"}

	_, err := svc.StartSession(t.Context(), forged, OrchestratedSessionRequest{Repository: "git@x:y/z", Name: "x"})
	assert.Equal(t, KindUnauthenticated, KindOf(err))
	assert.Empty(t, launcher.got.Name)

	_, err = svc.SendPrompt(t.Context(), forged, PromptSend{SessionID: "s1", Text: "hi"})
	assert.Equal(t, KindUnauthenticated, KindOf(err))
}

func TestOrchestrationStartSessionTagsAndValidates(t *testing.T) {
	launcher := &fakeOrchLauncher{}
	svc := newTestOrchestration(launcher, &fakeOrchDriver{}, &fakeOrchStatuses{script: []string{"ready"}}, &fakeOrchInbox{}, nil)

	_, err := svc.StartSession(t.Context(), testCaller, OrchestratedSessionRequest{Repository: "git@x:y/z", Name: "fix-auth", Prompt: " go ", Tags: []string{"auth", "orchestrator", " "}})
	require.NoError(t, err)
	assert.Equal(t, []string{"orchestrator", "auth"}, launcher.got.Tags)
	assert.Equal(t, "go", launcher.got.Prompt)

	_, err = svc.StartSession(t.Context(), testCaller, OrchestratedSessionRequest{Name: "x"})
	assert.Equal(t, KindInvalid, KindOf(err))

	launcher.err = fmt.Errorf("wrapped: %w", session.ErrDuplicateName)
	_, err = svc.StartSession(t.Context(), testCaller, OrchestratedSessionRequest{Repository: "git@x:y/r", Name: "x"})
	assert.Equal(t, KindConflict, KindOf(err))
}

func TestOrchestrationSessionsFiltersTheFleet(t *testing.T) {
	driver := &fakeOrchDriver{sessions: []session.Session{
		{ID: "s1", State: session.StateActive, Tags: []string{"orchestrator", "auth"}},
		{ID: "s2", State: session.StateActive, Tags: []string{"other"}},
		{ID: "s3", State: session.StateRecycled, Tags: []string{"orchestrator"}},
	}}
	svc := newTestOrchestration(&fakeOrchLauncher{}, driver, &fakeOrchStatuses{script: []string{"approval"}}, &fakeOrchInbox{}, nil)

	mine, err := svc.Sessions(t.Context(), testCaller, false, nil)
	require.NoError(t, err)
	require.Len(t, mine, 1)
	assert.Equal(t, "s1", mine[0].ID)
	assert.Equal(t, "approval", mine[0].AgentStatus)
	assert.True(t, mine[0].Running)

	all, err := svc.Sessions(t.Context(), testCaller, true, nil)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	tagged, err := svc.Sessions(t.Context(), testCaller, true, []string{"orchestrator"})
	require.NoError(t, err)
	assert.Len(t, tagged, 2)
}

func TestOrchestrationWaitForSession(t *testing.T) {
	driver := &fakeOrchDriver{sessions: []session.Session{{ID: "s1", State: session.StateActive, Tags: []string{"orchestrator"}}}}

	t.Run("reaches a named state", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			svc := newTestOrchestration(&fakeOrchLauncher{}, driver, &fakeOrchStatuses{script: []string{"active", "active", "approval"}}, &fakeOrchInbox{}, nil)
			res, err := svc.WaitForSession(t.Context(), testCaller, "s1", []string{"approval", "ready"}, time.Second)
			require.NoError(t, err)
			assert.True(t, res.Matched)
			assert.Equal(t, "approval", res.Session.AgentStatus)
		})
	})

	t.Run("any change when no state is named", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			svc := newTestOrchestration(&fakeOrchLauncher{}, driver, &fakeOrchStatuses{script: []string{"active", "active", "ready"}}, &fakeOrchInbox{}, nil)
			res, err := svc.WaitForSession(t.Context(), testCaller, "s1", nil, time.Second)
			require.NoError(t, err)
			assert.True(t, res.Matched)
			assert.Equal(t, "ready", res.Session.AgentStatus)
		})
	})

	t.Run("a failed status read is skipped, not read as missing", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			svc := newTestOrchestration(&fakeOrchLauncher{}, driver, &fakeOrchStatuses{script: []string{"active", "error", "ready"}}, &fakeOrchInbox{}, nil)
			res, err := svc.WaitForSession(t.Context(), testCaller, "s1", nil, 5*time.Second)
			require.NoError(t, err)
			assert.True(t, res.Matched)
			assert.Equal(t, "ready", res.Session.AgentStatus)
		})
	})

	t.Run("times out", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			svc := newTestOrchestration(&fakeOrchLauncher{}, driver, &fakeOrchStatuses{script: []string{"active"}}, &fakeOrchInbox{}, nil)
			res, err := svc.WaitForSession(t.Context(), testCaller, "s1", []string{"ready"}, 20*time.Millisecond)
			require.NoError(t, err)
			assert.False(t, res.Matched)
			assert.True(t, res.TimedOut)
		})
	})

	t.Run("returns on shutdown", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			done := make(chan struct{})
			close(done)
			svc := newTestOrchestration(&fakeOrchLauncher{}, driver, &fakeOrchStatuses{script: []string{"active"}}, &fakeOrchInbox{}, done)
			_, err := svc.WaitForSession(t.Context(), testCaller, "s1", []string{"ready"}, time.Minute)
			assert.Equal(t, KindUnavailable, KindOf(err))
		})
	})
}

func TestOrchestrationWaitForMessagesAcknowledges(t *testing.T) {
	inbox := &fakeOrchInbox{unread: []messaging.Message{{ID: "m1"}, {ID: "m2"}}}
	svc := newTestOrchestration(&fakeOrchLauncher{}, &fakeOrchDriver{}, &fakeOrchStatuses{script: []string{"ready"}}, inbox, nil)

	res, err := svc.WaitForMessages(t.Context(), testCaller, "t", time.Second)
	require.NoError(t, err)
	assert.Len(t, res.Messages, 2)
	assert.Equal(t, []string{"m1", "m2"}, inbox.acked)

	_, err = svc.WaitForMessages(t.Context(), testCaller, " ", time.Second)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestOrchestrationWaitForMessagesClaimsOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := make(chan struct{})
		inbox := &fakeOrchInbox{unread: []messaging.Message{{ID: "m1"}}, gate: gate}
		svc := newTestOrchestration(&fakeOrchLauncher{}, &fakeOrchDriver{}, &fakeOrchStatuses{script: []string{"ready"}}, inbox, nil)

		results := make(chan MessageWait, 2)
		for range 2 {
			go func() {
				res, err := svc.WaitForMessages(t.Context(), testCaller, "t", time.Second)
				assert.NoError(t, err)
				results <- res
			}()
		}
		synctest.Wait()
		close(gate)

		var got int
		for range 2 {
			got += len((<-results).Messages)
		}
		assert.Equal(t, 1, got, "two waits on one consumer take a message once")
	})
}

func TestOrchestrationSendPromptMapsDriverErrors(t *testing.T) {
	tests := []struct {
		err  error
		want Kind
	}{
		{session.ErrNotFound, KindNotFound},
		{prompt.ErrAgentPaneNotFound, KindNotFound},
	}
	for _, tt := range tests {
		svc := newTestOrchestration(&fakeOrchLauncher{}, &fakeOrchDriver{sendErr: tt.err}, &fakeOrchStatuses{script: []string{"ready"}}, &fakeOrchInbox{}, nil)
		_, err := svc.SendPrompt(t.Context(), testCaller, PromptSend{SessionID: "s1", Text: "hi"})
		assert.Equal(t, tt.want, KindOf(err), tt.err.Error())
	}
}

func TestClampWait(t *testing.T) {
	assert.Equal(t, defaultOrchestratorWait, clampWait(0))
	assert.Equal(t, MaxOrchestratorWait, clampWait(time.Hour))
	assert.Equal(t, 5*time.Second, clampWait(5*time.Second))
}

func TestOrchestrationStartSessionResolvesARepositoryName(t *testing.T) {
	launcher := &fakeOrchLauncher{}
	svc := newTestOrchestration(launcher, &fakeOrchDriver{}, &fakeOrchStatuses{script: []string{"ready"}}, &fakeOrchInbox{}, nil)

	_, err := svc.StartSession(t.Context(), testCaller, OrchestratedSessionRequest{Repository: "Hive", Name: "x"})
	require.NoError(t, err)
	assert.Equal(t, "git@github.com:colonyops/hive.git", launcher.got.Repo)

	_, err = svc.StartSession(t.Context(), testCaller, OrchestratedSessionRequest{Repository: "unknown", Name: "x"})
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestMostUrgentStatus(t *testing.T) {
	windows := []SessionWindowStatus{{Status: "ready"}, {Status: "approval"}, {Status: "active"}}
	assert.Equal(t, "approval", mostUrgentStatus(windows))
	assert.Empty(t, mostUrgentStatus(nil))
}
