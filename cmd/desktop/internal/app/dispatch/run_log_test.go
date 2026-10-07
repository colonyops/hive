package dispatch

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
)

type memoryRunLogSink struct {
	mu    sync.Mutex
	lines []stores.ActionRunLogLine
}

func (s *memoryRunLogSink) AppendLog(_ context.Context, commandID, attempt int64, lines []stores.NewActionRunLogLine) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, line := range lines {
		s.lines = append(s.lines, stores.ActionRunLogLine{CommandID: commandID, Attempt: attempt, Stream: line.Stream, Text: line.Text})
	}
	return nil
}

func (s *memoryRunLogSink) texts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.lines))
	for _, line := range s.lines {
		out = append(out, line.Stream+": "+line.Text)
	}
	return out
}

func TestRunLog_SplitsStreamsIntoLinesAndFlushesTheRemainderOnClose(t *testing.T) {
	sink := &memoryRunLogSink{}
	log := newRunLog(t.Context(), sink, 7, 2, zerolog.Nop())
	log.Systemf("$ build")
	_, _ = log.Stdout().Write([]byte("one\ntw"))
	_, _ = log.Stderr().Write([]byte("warn\r\n"))
	_, _ = log.Stdout().Write([]byte("o\nthree"))
	log.Close(t.Context())

	assert.Equal(t, []string{"system: $ build", "stdout: one", "stderr: warn", "stdout: two", "stdout: three"}, sink.texts())
	assert.Equal(t, int64(2), sink.lines[0].Attempt)
	assert.Equal(t, int64(7), sink.lines[0].CommandID)
}

func TestRunLog_StopsAtTheAttemptBoundWithOneMarker(t *testing.T) {
	sink := &memoryRunLogSink{}
	log := newRunLog(t.Context(), sink, 1, 1, zerolog.Nop())
	line := strings.Repeat("x", 1023) + "\n"
	for range maxRunLogAttemptBytes/1024 + 10 {
		_, _ = log.Stdout().Write([]byte(line))
	}
	log.Close(t.Context())

	texts := sink.texts()
	assert.Contains(t, texts[len(texts)-1], "Output truncated")
	assert.Len(t, texts, maxRunLogAttemptBytes/1023+1)
}

func TestRunLog_NilDiscards(t *testing.T) {
	var log *RunLog
	log.Systemf("ignored")
	_, err := log.Stdout().Write([]byte("ignored\n"))
	require.NoError(t, err)
	log.Close(t.Context())
	assert.Nil(t, RunLogFrom(t.Context()))
}

func shellAction(id, command string) actions.Action {
	return actions.Action{
		ID: id, Label: "Shell " + id, Type: "shell", ShowInDetail: true,
		Config: &actions.ShellConfig{CommandTemplate: command},
	}
}

func TestWorker_RecordsShellOutputAndOutcomeInTheRunLog(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	st := stores.New(db, stores.Options{})
	worker := NewWorker(st.OutputCommands, fakeActionLister{"build": shellAction("build", "echo out; echo err >&2; exit 3")},
		NewDispatcher(map[string]Executor{"shell": NewShellExecutor(zerolog.Nop(), hostEnvironment{})}), 0, zerolog.Nop())
	worker.SetRunLogSink(st.ActionRuns)

	view, err := worker.Confirm(t.Context(), "build", "item-1", []byte(`{}`), models.ItemRef{}, ActionInvocationInput{})
	require.NoError(t, err)
	waitForWorker(t, worker)

	lines, err := st.ActionRuns.ListLog(t.Context(), view.CommandID, 0, 100)
	require.NoError(t, err)
	texts := make([]string, 0, len(lines))
	for _, line := range lines {
		texts = append(texts, line.Stream+": "+line.Text)
		assert.Equal(t, int64(1), line.Attempt)
	}
	require.Len(t, texts, 6)
	assert.Equal(t, "system: Started Shell build (shell, manual, attempt 1)", texts[0])
	assert.Equal(t, "system: $ echo out; echo err >&2; exit 3", texts[1])
	assert.ElementsMatch(t, []string{"stdout: out", "stderr: err"}, texts[2:4])
	assert.True(t, strings.HasPrefix(texts[4], "system: Process exited with code 3 after "), texts[4])
	assert.True(t, strings.HasPrefix(texts[5], "system: Failed after "), texts[5])
}

func TestWorker_RunLogIsReadableWhileTheCommandRuns(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	st := stores.New(db, stores.Options{})
	worker := NewWorker(st.OutputCommands, fakeActionLister{"wait": shellAction("wait", "echo started; sleep 5")},
		NewDispatcher(map[string]Executor{"shell": NewShellExecutor(zerolog.Nop(), hostEnvironment{})}), 0, zerolog.Nop())
	worker.SetRunLogSink(st.ActionRuns)

	view, err := worker.Confirm(t.Context(), "wait", "item-1", []byte(`{}`), models.ItemRef{}, ActionInvocationInput{})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		lines, err := st.ActionRuns.ListLog(t.Context(), view.CommandID, 0, 100)
		require.NoError(t, err)
		for _, line := range lines {
			if line.Stream == RunLogStdout && line.Text == "started" {
				return true
			}
		}
		return false
	}, 3*time.Second, 50*time.Millisecond)

	require.True(t, worker.Cancel(view.CommandID))
	waitForWorkerWithin(t, worker, 5*time.Second)
	lines, err := st.ActionRuns.ListLog(t.Context(), view.CommandID, 0, 100)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(lines[len(lines)-1].Text, "Cancelled after "), lines[len(lines)-1].Text)
}

func TestWorker_RetryAttemptsLogUnderTheirOwnAttempt(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	st := stores.New(db, stores.Options{})
	enqueueTestCommand(t, db, "flaky", "item-1", `{}`)
	exec := &fakeExecutor{err: fmt.Errorf("not yet")}
	worker := NewWorker(st.OutputCommands, fakeActionLister{"flaky": launchSessionAction("flaky", true)},
		NewDispatcher(map[string]Executor{"launch-session": exec}), 0, zerolog.Nop())
	worker.SetRunLogSink(st.ActionRuns)
	worker.retryDelay = 0

	worker.Tick(t.Context())
	waitForWorker(t, worker)
	worker.Tick(t.Context())
	waitForWorker(t, worker)

	runs, err := st.ActionRuns.List(t.Context(), 0, 10)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	lines, err := st.ActionRuns.ListLog(t.Context(), runs[0].ID, 0, 100)
	require.NoError(t, err)
	require.Len(t, lines, 6)
	assert.Equal(t, int64(1), lines[0].Attempt)
	assert.Equal(t, "Started Test action (launch-session, automatic, attempt 1)", lines[0].Text)
	assert.Contains(t, lines[1].Text, "Attempt 1 failed after ")
	assert.Equal(t, "Retrying in 0ms", lines[2].Text)
	assert.Equal(t, int64(2), lines[3].Attempt)
}

func waitForWorkerWithin(t *testing.T, worker *Worker, d time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), d)
	defer cancel()
	require.NoError(t, worker.WaitIdle(ctx))
}

func TestWorker_NodeCommandsWriteNoRunLogAndAreNotListed(t *testing.T) {
	t.Parallel()
	db := openTestPipelineDB(t)
	st := stores.New(db, stores.Options{})
	nodeID := models.NotifyActionID("flow/notify")
	enqueueTestCommand(t, db, nodeID, "item-1", `{}`)
	worker := NewWorker(st.OutputCommands, fakeActionLister{nodeID: launchSessionAction(nodeID, true)},
		NewDispatcher(map[string]Executor{"launch-session": &fakeExecutor{}}), 0, zerolog.Nop())
	worker.SetRunLogSink(st.ActionRuns)

	worker.Tick(t.Context())
	waitForWorker(t, worker)

	runs, err := st.ActionRuns.List(t.Context(), 0, 10)
	require.NoError(t, err)
	assert.Empty(t, runs)
	lines, err := st.ActionRuns.ListLog(t.Context(), 1, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, lines)
}
