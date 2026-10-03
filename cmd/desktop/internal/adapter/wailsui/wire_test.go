package wailsui

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/hive/gitstatus"
)

// jsonKeys marshals v and returns its top-level keys, sorted.
func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func sorted(keys ...string) []string {
	slices.Sort(keys)
	return keys
}

// The frontend reads these names; the wire types exist to hold them still
// while the app types behind them change.
func TestWireTypesKeepTheFrontendFieldNames(t *testing.T) {
	taskKeys := []string{"id", "repoKey", "epicId", "parentId", "sessionId", "title", "type", "status", "blocked", "depth", "createdAt", "updatedAt"}
	for _, tt := range []struct {
		name string
		v    any
		want []string
	}{
		{"SessionSummary", SessionSummary{}, sorted("id", "name", "slug", "repo", "state")},
		{"SessionDetail", SessionDetail{}, sorted("id", "name", "slug", "repo", "state", "path", "cloneStrategy", "worktreeBranch", "tags", "createdAt", "updatedAt")},
		{"SessionRisk", SessionRisk{}, sorted("uncommittedChanges", "unpushedCommits", "recycleDeletes")},
		{"SessionGitStatus", SessionGitStatus{}, sorted("path", "branch", "dirty", "unpushed", "additions", "deletions", "host", "owner", "repo", "resolved", "error")},
		{"SessionWindowStatus", SessionWindowStatus{}, sorted("windowId", "status", "tool")},
		{"SessionStatus", SessionStatus{}, sorted("sessionId", "running", "windows")},
		{"SessionStatusSnapshot", SessionStatusSnapshot{}, sorted("items", "pollIntervalMs")},
		{"ItemSessionView", ItemSessionView{}, sorted("id", "name", "slug", "repo", "state", "running", "createdAt")},
		{"ItemChatView", ItemChatView{}, sorted("id", "workspace", "name", "createdAt")},
		{"SessionPullRequestKey", SessionPullRequestKey{}, sorted("host", "owner", "repo", "branch")},
		{"SessionPullRequest", SessionPullRequest{}, sorted("status", "number", "title", "state", "isDraft", "url", "reviewDecision", "checks", "additions", "deletions", "cached")},
		{"SessionLaunchRepository", SessionLaunchRepository{}, sorted("name", "repository")},
		{"SessionLaunchWorkspace", SessionLaunchWorkspace{}, sorted("dir", "name", "supportsPrompt")},
		{"SessionLaunchOptions", SessionLaunchOptions{}, sorted("repositories", "defaultRepository", "workspaces", "agents", "defaultAgent")},
		{"CreateSessionRequest", CreateSessionRequest{Repository: "r", Workspace: "w", Agent: "a", ItemIDs: []int64{1}}, sorted("repository", "workspace", "name", "prompt", "agent", "itemIds")},
		{"SessionDraft", SessionDraft{Repository: "r", Workspace: "w", Agent: "a", ItemIDs: []int64{1}, Failure: &SessionCreateFailure{}}, sorted("repository", "workspace", "name", "prompt", "agent", "itemIds", "failure")},
		{"SessionCreateFailure", SessionCreateFailure{}, sorted("reason", "step", "output", "cloneStrategy", "destination", "leftoverCheckout", "at")},
		{"TerminalTarget", TerminalTarget{WindowID: "@1"}, sorted("slug", "windowId")},
		{"TaskItem", TaskItem{}, sorted(taskKeys...)},
		{"TaskDetail", TaskDetail{}, sorted(append(slices.Clone(taskKeys), "desc", "blockers", "comments")...)},
		{"TaskBlocker", TaskBlocker{}, sorted("id", "title", "status")},
		{"TaskComment", TaskComment{}, sorted("id", "message", "createdAt")},
		{"ActionInvocationInput", ActionInvocationInput{Session: &SessionInvocationInput{}, Inputs: map[string]string{"k": "v"}, Rerun: true}, sorted("session", "inputs", "rerun")},
		{"SessionInvocationInput", SessionInvocationInput{Repository: "r", Workspace: "w", Agent: "a"}, sorted("name", "repository", "workspace", "agent")},
		{"ActionRunView", ActionRunView{Result: &ExecutionOutcome{}, Error: "e", Stdout: "o", Stderr: "e", ConfirmationRequired: true}, sorted("commandId", "status", "result", "error", "stdout", "stderr", "confirmationRequired")},
		{"ExecutionOutcome", ExecutionOutcome{Session: &SessionExecutionOutcome{}, Message: &MessageExecutionOutcome{}, Clipboard: &ClipboardExecutionOutcome{}}, sorted("session", "message", "clipboard")},
		{"SessionExecutionOutcome", SessionExecutionOutcome{Slug: "s", Path: "p"}, sorted("id", "name", "slug", "path")},
		{"MessageExecutionOutcome", MessageExecutionOutcome{}, sorted("topic", "sender")},
		{"ClipboardExecutionOutcome", ClipboardExecutionOutcome{}, sorted("text")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, jsonKeys(t, tt.v))
		})
	}
}

func TestSessionStatusSnapshotOfConvertsPollIntervalForBrowser(t *testing.T) {
	got := sessionStatusSnapshotOf(app.SessionStatusSnapshot{
		Items: []app.SessionStatus{{
			SessionID: "s1",
			Running:   true,
			Windows:   []app.SessionWindowStatus{{WindowID: "@1", Status: terminal.StatusApproval, Tool: "claude"}},
		}},
		PollInterval: 1750 * time.Millisecond,
	})

	assert.Equal(t, SessionStatusSnapshot{
		Items: []SessionStatus{{
			SessionID: "s1",
			Running:   true,
			Windows:   []SessionWindowStatus{{WindowID: "@1", Status: "approval", Tool: "claude"}},
		}},
		PollIntervalMS: 1750,
	}, got)
}

func TestSessionDetailOfReadsTheWorktreeBranch(t *testing.T) {
	s := session.Session{ID: "s1", Name: "review 81", Slug: "review-81", Remote: "acme/site", State: session.StateActive, CloneStrategy: session.CloneStrategyWorktree}
	s.SetMeta(session.MetaWorktreeBranch, "hive/review-81")

	got := sessionDetailOf(s)
	assert.Equal(t, "hive/review-81", got.WorktreeBranch)
	assert.Equal(t, "acme/site", got.Repo)
	assert.Equal(t, "active", got.State)
}

// The read rules (a failed read never claims dirty, a failed branch stands
// for the whole status) are pinned in hive/gitstatus. This pins that the wire
// carries the failure as text.
func TestSessionGitStatusOfCarriesAFailedReadAsText(t *testing.T) {
	got := sessionGitStatusOf(gitstatus.Status{Branch: "feat/bar", Resolved: true, Err: errors.New("git status: exit 128")})
	assert.False(t, got.Dirty)
	assert.True(t, got.Resolved)
	assert.Equal(t, "git status: exit 128", got.Error)

	unpushed := sessionGitStatusOf(gitstatus.Status{Resolved: true, UnpushedErr: errors.New("no upstream")})
	assert.Equal(t, "no upstream", unpushed.Error)
}

func TestTaskDetailOfFlattensTheItemAndKeepsEmptyListsNonNil(t *testing.T) {
	got := taskDetailOf(app.TaskDetail{ID: "t1", Desc: "do it", Type: hc.ItemTypeTask, Status: hc.StatusDone})
	assert.Equal(t, "t1", got.ID)
	assert.Equal(t, "do it", got.Desc)
	assert.Equal(t, "done", got.Status)
	assert.NotNil(t, got.Blockers)
	assert.NotNil(t, got.Comments)
}

func TestActionRunViewOfCopiesTheOutcome(t *testing.T) {
	got := actionRunViewOf(dispatch.ActionRunView{
		CommandID: 3, Status: "done",
		Result: &dispatch.ExecutionOutcome{Message: &dispatch.MessageExecutionOutcome{Topic: "t", Sender: "hive-desktop"}},
	})
	assert.Equal(t, ActionRunView{
		CommandID: 3, Status: "done",
		Result: &ExecutionOutcome{Message: &MessageExecutionOutcome{Topic: "t", Sender: "hive-desktop"}},
	}, got)
	assert.Nil(t, actionRunViewOf(dispatch.ActionRunView{}).Result)
}
