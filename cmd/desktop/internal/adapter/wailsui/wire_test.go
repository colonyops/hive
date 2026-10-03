package wailsui

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/domain/session"
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

// The frontend reads these names. The wire types hold them still while the
// domain types behind them change; the app and dispatch types carry them
// directly.
func TestFrontendTypesKeepTheirFieldNames(t *testing.T) {
	taskKeys := []string{"id", "repoKey", "epicId", "parentId", "sessionId", "title", "type", "status", "blocked", "depth", "createdAt", "updatedAt"}
	for _, tt := range []struct {
		name string
		v    any
		want []string
	}{
		{"SessionSummary", SessionSummary{}, sorted("id", "name", "slug", "repo", "state")},
		{"SessionDetail", SessionDetail{}, sorted("id", "name", "slug", "repo", "state", "path", "cloneStrategy", "worktreeBranch", "tags", "createdAt", "updatedAt")},
		{"SessionRisk", app.SessionRisk{}, sorted("uncommittedChanges", "unpushedCommits", "recycleDeletes")},
		{"SessionGitStatus", SessionGitStatus{}, sorted("path", "branch", "dirty", "unpushed", "additions", "deletions", "host", "owner", "repo", "resolved", "error")},
		{"SessionWindowStatus", app.SessionWindowStatus{}, sorted("windowId", "status", "tool")},
		{"SessionStatus", app.SessionStatus{}, sorted("sessionId", "running", "windows")},
		{"SessionStatusSnapshot", app.SessionStatusSnapshot{}, sorted("items", "pollIntervalMs")},
		{"ItemSessionView", app.ItemSessionView{}, sorted("id", "name", "slug", "repo", "state", "running", "createdAt")},
		{"ItemChatView", app.ItemChatView{}, sorted("id", "workspace", "name", "createdAt")},
		{"SessionPullRequestKey", app.SessionPullRequestKey{}, sorted("host", "owner", "repo", "branch")},
		{"SessionPullRequest", app.SessionPullRequest{}, sorted("status", "number", "title", "state", "isDraft", "url", "reviewDecision", "checks", "additions", "deletions", "cached")},
		{"SessionLaunchRepository", dispatch.SessionLaunchRepository{}, sorted("name", "repository")},
		{"SessionLaunchWorkspace", dispatch.SessionLaunchWorkspace{}, sorted("dir", "name", "supportsPrompt")},
		{"SessionLaunchOptions", dispatch.SessionLaunchOptions{}, sorted("repositories", "defaultRepository", "workspaces", "agents", "defaultAgent")},
		{"CreateSessionRequest", dispatch.CreateSessionRequest{Repository: "r", Workspace: "w", Agent: "a", ItemIDs: []int64{1}}, sorted("repository", "workspace", "name", "prompt", "agent", "itemIds")},
		{"SessionDraft", dispatch.SessionDraft{Repository: "r", Workspace: "w", Agent: "a", ItemIDs: []int64{1}, Failure: &dispatch.SessionCreateFailure{}}, sorted("repository", "workspace", "name", "prompt", "agent", "itemIds", "failure")},
		{"SessionCreateFailure", dispatch.SessionCreateFailure{}, sorted("reason", "step", "output", "cloneStrategy", "destination", "leftoverCheckout", "at")},
		{"TerminalTarget", dispatch.TerminalTarget{WindowID: "@1"}, sorted("slug", "windowId")},
		{"TaskItem", TaskItem{}, sorted(taskKeys...)},
		{"TaskDetail", TaskDetail{}, sorted(append(slices.Clone(taskKeys), "desc", "blockers", "comments")...)},
		{"TaskBlocker", app.TaskBlocker{}, sorted("id", "title", "status")},
		{"TaskComment", app.TaskComment{}, sorted("id", "message", "createdAt")},
		{"ActionInvocationInput", dispatch.ActionInvocationInput{Session: &dispatch.SessionInvocationInput{}, Inputs: map[string]string{"k": "v"}, Rerun: true}, sorted("session", "inputs", "rerun")},
		{"SessionInvocationInput", dispatch.SessionInvocationInput{Repository: "r", Workspace: "w", Agent: "a"}, sorted("name", "repository", "workspace", "agent")},
		{"ActionRunView", dispatch.ActionRunView{Result: &dispatch.ExecutionOutcome{}, Error: "e", Stdout: "o", Stderr: "e", ConfirmationRequired: true}, sorted("commandId", "status", "result", "error", "stdout", "stderr", "confirmationRequired")},
		{"ExecutionOutcome", dispatch.ExecutionOutcome{Session: &dispatch.SessionExecutionOutcome{}, Message: &dispatch.MessageExecutionOutcome{}, Clipboard: &dispatch.ClipboardExecutionOutcome{}}, sorted("session", "message", "clipboard")},
		{"SessionExecutionOutcome", dispatch.SessionExecutionOutcome{Slug: "s", Path: "p"}, sorted("id", "name", "slug", "path")},
		{"MessageExecutionOutcome", dispatch.MessageExecutionOutcome{}, sorted("topic", "sender")},
		{"ClipboardExecutionOutcome", dispatch.ClipboardExecutionOutcome{}, sorted("text")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, jsonKeys(t, tt.v))
		})
	}
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

func TestTaskDetailOfFlattensTheItem(t *testing.T) {
	got := taskDetailOf(app.TaskDetail{ID: "t1", Desc: "do it", Type: hc.ItemTypeTask, Status: hc.StatusDone, Blocked: true})
	assert.Equal(t, "t1", got.ID)
	assert.Equal(t, "do it", got.Desc)
	assert.Equal(t, "done", got.Status)
	assert.True(t, got.Blocked)
}
