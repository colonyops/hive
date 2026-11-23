package engineops_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/engineops"
	"github.com/colonyops/hive/internal/hive/hivetest"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/catalog"
	"github.com/colonyops/hive/internal/ops/messages"
	"github.com/colonyops/hive/internal/ops/tasks"
	"github.com/colonyops/hive/internal/ops/todos"
	"github.com/colonyops/hive/internal/store"
)

const chatID = "0b5f3c2e-7a41-4d8e-9c3a-2f6d1e8b4a90"

type fixture struct {
	h       *engineops.Handlers
	sess    session.Session
	outside string
	repo    string
	harness hivetest.Harness
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	harness := hivetest.NewEngine(t, hivetest.Options{})
	sess := session.Session{
		ID: "abc123", Name: "worker", Slug: "worker", Path: t.TempDir(), State: session.StateActive,
		Remote: "git@github.com:acme/widgets.git", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, store.NewSessionStore(harness.DB).Save(t.Context(), sess))
	return fixture{
		h: engineops.New(zerolog.Nop(), harness.Engine), sess: sess, outside: t.TempDir(),
		repo: "acme/widgets", harness: harness,
	}
}

func kindOf(t *testing.T, err error) ops.Kind {
	t.Helper()
	require.Error(t, err)
	opErr, ok := errors.AsType[*ops.Error](err)
	require.True(t, ok, "%T is not *ops.Error: %v", err, err)
	return opErr.Kind
}

func children(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var out []map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &out))
	return out
}

func TestOpsCoverTheEngineCatalog(t *testing.T) {
	f := newFixture(t)
	var want, got []string
	for _, op := range catalog.All() {
		if op.Info().Owner == ops.OwnerEngine {
			want = append(want, op.Info().Name)
		}
	}
	for _, op := range f.h.Ops() {
		assert.Equal(t, ops.OwnerEngine, op.Spec.Info().Owner)
		got = append(got, op.Spec.Info().Name)
	}
	assert.ElementsMatch(t, want, got)
}

func TestTaskTreeClaimAndFinish(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()

	created, err := f.h.TaskCreate(ctx, tasks.TaskCreateIn{
		Caller: f.sess.Path, Title: "Auth", Type: "epic",
		Children: children(t, `[{"ref":"jwt","title":"JWT","type":"task"},{"title":"Login","type":"task","blockers":["jwt"]}]`),
	})
	require.NoError(t, err)
	require.Len(t, created.Tasks, 3)
	epic := created.Tasks[0]
	assert.Equal(t, f.repo, epic.RepoKey, "the caller session's remote scopes the tree")

	next, err := f.h.TaskNext(ctx, tasks.TaskNextIn{Caller: f.sess.ID, EpicID: epic.ID, Assign: true})
	require.NoError(t, err)
	assert.Equal(t, "JWT", next.Title, "Login waits for its blocker")
	assert.Equal(t, hc.StatusInProgress, next.Status)
	assert.Equal(t, f.sess.ID, next.SessionID)

	again, err := f.h.TaskNext(ctx, tasks.TaskNextIn{Caller: f.sess.ID, EpicID: epic.ID, Assign: true})
	require.NoError(t, err)
	assert.Equal(t, next.ID, again.ID, "a claimed task is resumed")

	_, err = f.h.TaskCommentCreate(ctx, tasks.TaskCommentCreateIn{TaskID: next.ID, Message: "CHECKPOINT: wired"})
	require.NoError(t, err)
	got, err := f.h.TaskGet(ctx, tasks.TaskGetIn{TaskID: next.ID})
	require.NoError(t, err)
	require.Len(t, got.Comments, 1)
	assert.Len(t, got.JSONLines(), 2, "the item, then each comment")

	block, err := f.h.TaskContextGet(ctx, tasks.TaskContextGetIn{Caller: f.sess.ID, EpicID: epic.ID})
	require.NoError(t, err)
	require.Len(t, block.MyTasks, 1)

	done, err := f.h.TaskUpdate(ctx, tasks.TaskUpdateIn{Caller: f.outside, TaskID: next.ID, Status: "done"})
	require.NoError(t, err, "a status change needs no session")
	assert.Equal(t, hc.StatusDone, done.Status)

	list, err := f.h.TaskList(ctx, tasks.TaskListIn{Caller: f.sess.ID, EpicID: epic.ID, Status: tasks.StatusAll})
	require.NoError(t, err)
	assert.Len(t, list.Tasks, 2, "an epic filter lists the epic's items")
	open, err := f.h.TaskList(ctx, tasks.TaskListIn{Caller: f.sess.ID, EpicID: epic.ID})
	require.NoError(t, err)
	assert.Len(t, open.Tasks, 1, "open by default")

	cleared, err := f.h.TaskClear(ctx, tasks.TaskClearIn{Caller: f.sess.ID, DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, tasks.TaskClearOut{Action: "would prune", Count: 1}, cleared)
}

func TestTaskRefusals(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	created, err := f.h.TaskCreate(ctx, tasks.TaskCreateIn{Caller: f.outside, Title: "Epic", Type: "epic"})
	require.NoError(t, err)
	epic := created.Tasks[0]
	assert.Empty(t, epic.RepoKey, "a directory with no git remote scopes nothing")
	a, err := f.h.TaskCreate(ctx, tasks.TaskCreateIn{Caller: f.outside, Title: "A", ParentID: epic.ID})
	require.NoError(t, err)
	b, err := f.h.TaskCreate(ctx, tasks.TaskCreateIn{Caller: f.outside, Title: "B", ParentID: epic.ID})
	require.NoError(t, err)

	_, err = f.h.TaskUpdate(ctx, tasks.TaskUpdateIn{TaskID: b.Tasks[0].ID, AddBlocker: a.Tasks[0].ID})
	require.NoError(t, err)
	_, err = f.h.TaskUpdate(ctx, tasks.TaskUpdateIn{TaskID: a.Tasks[0].ID, AddBlocker: b.Tasks[0].ID})
	assert.Equal(t, ops.KindConflict, kindOf(t, err))

	_, err = f.h.TaskNext(ctx, tasks.TaskNextIn{Caller: f.outside, EpicID: epic.ID, Assign: true})
	assert.Equal(t, ops.KindInvalid, kindOf(t, err), "a claim needs a session or chat")
	_, err = f.h.TaskNext(ctx, tasks.TaskNextIn{Caller: "zzz999", EpicID: epic.ID})
	assert.Equal(t, ops.KindNotFound, kindOf(t, err), "an id that names no session stays not_found")
	_, err = f.h.TaskNext(ctx, tasks.TaskNextIn{Caller: f.outside, EpicID: "hc-missing"})
	assert.Equal(t, ops.KindNotFound, kindOf(t, err))
	_, err = f.h.TaskGet(ctx, tasks.TaskGetIn{TaskID: "hc-missing"})
	assert.Equal(t, ops.KindNotFound, kindOf(t, err))
	_, err = f.h.TaskList(ctx, tasks.TaskListIn{Caller: f.outside, SessionID: "server-b:abc"})
	assert.Equal(t, ops.KindInvalid, kindOf(t, err))
	_, err = f.h.TaskCreate(ctx, tasks.TaskCreateIn{
		Caller: f.outside, Title: "Tree", Type: "epic", Children: children(t, `[{"title":"x","type":"task","blockers":["nope"]}]`),
	})
	assert.Equal(t, ops.KindInvalid, kindOf(t, err))
}

func TestAssignSession(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	created, err := f.h.TaskCreate(ctx, tasks.TaskCreateIn{Caller: "global", Title: "Epic", Type: "epic"})
	require.NoError(t, err)
	epic := created.Tasks[0]

	for _, value := range []string{f.sess.ID, "local:" + f.sess.ID, chatID} {
		got, err := f.h.TaskCreate(ctx, tasks.TaskCreateIn{Caller: "global", Title: "t", ParentID: epic.ID, AssignSession: value})
		require.NoError(t, err, value)
		want := f.sess.ID
		if value == chatID {
			want = chatID
		}
		assert.Equal(t, want, got.Tasks[0].SessionID, value)
		assert.Equal(t, hc.StatusOpen, got.Tasks[0].Status, "assigning does not start the task")
	}

	task, err := f.h.TaskCreate(ctx, tasks.TaskCreateIn{Caller: "global", Title: "free", ParentID: epic.ID})
	require.NoError(t, err)
	id := task.Tasks[0].ID
	updated, err := f.h.TaskUpdate(ctx, tasks.TaskUpdateIn{Caller: "global", TaskID: id, AssignSession: "local:" + f.sess.ID})
	require.NoError(t, err, "a global caller can assign work")
	assert.Equal(t, f.sess.ID, updated.SessionID)
	cleared, err := f.h.TaskUpdate(ctx, tasks.TaskUpdateIn{Caller: "global", TaskID: id, Unassign: true})
	require.NoError(t, err)
	assert.Empty(t, cleared.SessionID)

	tree, err := f.h.TaskCreate(ctx, tasks.TaskCreateIn{
		Caller: "global", Title: "Tree", Type: "epic",
		Children: children(t, `[{"title":"mine","assignSession":"local:`+f.sess.ID+`","children":[{"title":"deep","assignSession":"`+chatID+`"}]},{"title":"free"}]`),
	})
	require.NoError(t, err)
	owners := map[string]string{}
	for _, item := range tree.Tasks {
		owners[item.Title] = item.SessionID
	}
	assert.Equal(t, map[string]string{"Tree": "", "mine": f.sess.ID, "deep": chatID, "free": ""}, owners)

	for name, value := range map[string]string{"unknown": "zzz999", "other server": "server-b:" + f.sess.ID, "path": f.outside, "global": "global"} {
		_, err = f.h.TaskUpdate(ctx, tasks.TaskUpdateIn{TaskID: id, AssignSession: value})
		want := ops.KindInvalid
		if name == "unknown" {
			want = ops.KindNotFound
		}
		assert.Equal(t, want, kindOf(t, err), name)
	}

	_, err = f.h.TaskCreate(ctx, tasks.TaskCreateIn{Caller: "global", Title: "x", ParentID: epic.ID, AssignSession: "zzz999"})
	assert.Equal(t, ops.KindNotFound, kindOf(t, err))

	_, err = f.h.TaskCreate(ctx, tasks.TaskCreateIn{
		Caller: "global", Title: "Bad", Type: "epic", Children: children(t, `[{"title":"t","assignSession":"zzz999"}]`),
	})
	assert.Equal(t, ops.KindNotFound, kindOf(t, err))
	all, err := f.h.TaskList(ctx, tasks.TaskListIn{Caller: "global", AllRepos: true, Status: tasks.StatusAll})
	require.NoError(t, err)
	for _, item := range all.Tasks {
		assert.NotEqual(t, "Bad", item.Title, "a refused tree writes nothing")
	}

	require.Error(t, tasks.TaskUpdateIn{TaskID: id, AssignSession: f.sess.ID, Assign: true}.Validate())
	require.Error(t, tasks.TaskUpdateIn{TaskID: id, AssignSession: f.sess.ID, Unassign: true}.Validate())
	require.NoError(t, tasks.TaskUpdateIn{TaskID: id, AssignSession: f.sess.ID}.Validate())
	require.Error(t, tasks.TaskCreateIn{Title: "T", Type: "epic", AssignSession: f.sess.ID, Children: []map[string]any{{"title": "c"}}}.Validate())
}

func TestAChatClaimsATask(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	created, err := f.h.TaskCreate(ctx, tasks.TaskCreateIn{
		Caller: "global", Title: "Epic", Type: "epic", Children: children(t, `[{"title":"one","type":"task"}]`),
	})
	require.NoError(t, err)
	got, err := f.h.TaskNext(ctx, tasks.TaskNextIn{Caller: chatID, EpicID: created.Tasks[0].ID, Assign: true})
	require.NoError(t, err)
	assert.Equal(t, chatID, got.SessionID)
}

func TestReviewedEdges(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()

	created, err := f.h.TaskCreate(ctx, tasks.TaskCreateIn{
		Caller: chatID, Title: "From a chat", Type: "epic", Ref: "root",
		Children: children(t, `[{"title":"untyped","blockers":["root"]},{"title":"open"}]`),
	})
	require.NoError(t, err)
	assert.Empty(t, created.Tasks[0].RepoKey, "a chat names no repository")
	assert.Equal(t, hc.ItemTypeTask, created.Tasks[1].Type, "a child with no type is a task")

	next, err := f.h.TaskNext(ctx, tasks.TaskNextIn{Caller: f.sess.ID, EpicID: created.Tasks[0].ID, Assign: true})
	require.NoError(t, err, "the epic scopes the read, not the worker's repository")
	assert.Equal(t, "open", next.Title)

	_, err = f.h.TaskClear(ctx, tasks.TaskClearIn{Caller: chatID})
	assert.Equal(t, ops.KindInvalid, kindOf(t, err), "an unscoped clear must say allRepos")
	_, err = f.h.TaskClear(ctx, tasks.TaskClearIn{Caller: chatID, AllRepos: true, DryRun: true})
	require.NoError(t, err)

	_, err = f.h.TaskContextGet(ctx, tasks.TaskContextGetIn{Caller: chatID, EpicID: next.ID})
	assert.Equal(t, ops.KindInvalid, kindOf(t, err), "a task is not an epic")

	require.Error(t, tasks.TaskCreateIn{Title: " ", Type: "epic"}.Validate())

	todo, err := f.h.TodoCreate(ctx, todos.TodoCreateIn{Caller: chatID, Title: "From a chat"})
	require.NoError(t, err)
	assert.Empty(t, todo.URI, "a chat is not a session to open")
	assert.Equal(t, chatID, todo.SessionID)
}

func TestMessages(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()

	pub, err := f.h.MessageCreate(ctx, messages.MessageCreateIn{
		Caller: f.outside, Topics: []string{engineops.InboxTopic(f.sess.ID)}, Payload: "hello",
	})
	require.NoError(t, err)
	assert.Empty(t, pub.Sender, "a caller in no session sends unnamed")

	inbox, err := f.h.MessageList(ctx, messages.MessageListIn{Caller: f.sess.Path, Inbox: true, Unread: true})
	require.NoError(t, err)
	require.Len(t, inbox.Messages, 1)

	read, err := f.h.MessageReadPut(ctx, messages.MessageReadPutIn{Caller: f.sess.ID, MessageIDs: []string{inbox.Messages[0].ID}})
	require.NoError(t, err)
	assert.Equal(t, f.sess.ID, read.Reader)

	unread, err := f.h.MessageList(ctx, messages.MessageListIn{Caller: f.sess.ID, Inbox: true, Unread: true})
	require.NoError(t, err)
	assert.Empty(t, unread.Messages)

	topics, err := f.h.TopicList(ctx, messages.TopicListIn{Caller: f.sess.ID})
	require.NoError(t, err)
	require.Len(t, topics.Topics, 1)
	assert.Equal(t, 1, topics.Topics[0].MessageCount)

	_, err = f.h.MessageList(ctx, messages.MessageListIn{Caller: f.outside, Inbox: true})
	assert.Equal(t, ops.KindInvalid, kindOf(t, err))

	none, err := f.h.MessageList(ctx, messages.MessageListIn{Caller: f.outside, Topic: "nobody.home"})
	require.NoError(t, err)
	assert.Empty(t, none.Messages)
}

func TestMessageWait(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()

	timedOut, err := f.h.MessageList(ctx, messages.MessageListIn{Caller: f.outside, Topic: "handoff", Wait: 1})
	require.NoError(t, err)
	assert.True(t, timedOut.TimedOut)
	assert.Equal(t, []any{messages.TimeoutLine{Status: "timeout", Topic: "handoff", Duration: "1s"}}, timedOut.JSONLines())

	go func() {
		time.Sleep(200 * time.Millisecond)
		_, _ = f.h.MessageCreate(context.Background(), messages.MessageCreateIn{Caller: chatID, Topics: []string{"handoff"}, Payload: "go"})
	}()
	got, err := f.h.MessageList(ctx, messages.MessageListIn{Caller: f.outside, Topic: "handoff", Wait: 5})
	require.NoError(t, err)
	require.Len(t, got.Messages, 1)
	assert.Equal(t, chatID, got.Messages[0].Sender)
}

func TestTodos(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()

	created, err := f.h.TodoCreate(ctx, todos.TodoCreateIn{Caller: f.sess.ID, Title: "Review plan"})
	require.NoError(t, err)
	assert.Equal(t, "session://"+f.sess.ID, created.URI)
	assert.Equal(t, "agent", created.Source)
	assert.Equal(t, f.sess.ID, created.SessionID)

	system, err := f.h.TodoCreate(ctx, todos.TodoCreateIn{Caller: f.outside, Title: "Migrated", Source: "system"})
	require.NoError(t, err)
	assert.Empty(t, system.URI)

	updated, err := f.h.TodoUpdate(ctx, todos.TodoUpdateIn{TodoID: created.ID, Status: "completed"})
	require.NoError(t, err)
	assert.Equal(t, "completed", updated.Status)

	pending, err := f.h.TodoList(ctx, todos.TodoListIn{Status: "pending"})
	require.NoError(t, err)
	require.Len(t, pending.Todos, 1)
	assert.Equal(t, system.ID, pending.Todos[0].ID)

	_, err = f.h.TodoUpdate(ctx, todos.TodoUpdateIn{TodoID: "missing", Status: "completed"})
	assert.Equal(t, ops.KindNotFound, kindOf(t, err))
	_, err = f.h.TodoCreate(ctx, todos.TodoCreateIn{Caller: f.outside, Title: "x", URI: "bare"})
	assert.Equal(t, ops.KindInvalid, kindOf(t, err))
}

// TestMountedToolsAnswer drives every engine tool's schema through a real MCP
// server, so an output type the SDK cannot validate fails here.
func TestMountedToolsAnswer(t *testing.T) {
	f := newFixture(t)
	srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	for _, op := range f.h.Ops() {
		op.Mount(srv, func(err error) error { return err })
	}
	serverT, clientT := mcp.NewInMemoryTransports()
	_, err := srv.Connect(t.Context(), serverT, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "client"}, nil).Connect(t.Context(), clientT, nil)
	require.NoError(t, err)

	listed, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	assert.Len(t, listed.Tools, len(f.h.Ops()))

	call := func(tool string, args map[string]any) *mcp.CallToolResult {
		res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
		require.NoError(t, err)
		return res
	}
	res := call("task_create", map[string]any{
		"caller": f.sess.ID, "title": "Epic", "type": "epic",
		"children": []any{map[string]any{"title": "one", "type": "task"}},
	})
	require.False(t, res.IsError, "%v", res.Content)
	var created tasks.TaskListOut
	raw, _ := json.Marshal(res.StructuredContent)
	require.NoError(t, json.Unmarshal(raw, &created))
	epicID := created.Tasks[0].ID

	for tool, args := range map[string]map[string]any{
		"task_next":        {"caller": f.sess.ID, "epicId": epicID, "assign": true},
		"task_list":        {"caller": f.sess.ID},
		"task_context_get": {"caller": f.sess.ID, "epicId": epicID},
		"message_create":   {"caller": f.sess.ID, "topics": []string{"t"}, "payload": "p"},
		"message_list":     {"caller": f.sess.ID},
		"topic_list":       {"caller": f.sess.ID},
		"todo_create":      {"caller": f.sess.ID, "title": "x"},
		"todo_list":        {},
		"task_clear":       {"caller": f.sess.ID, "dryRun": true},
	} {
		res := call(tool, args)
		assert.False(t, res.IsError, "%s: %v", tool, res.Content)
	}

	res = call("task_next", map[string]any{"caller": f.sess.ID, "epicId": "hc-missing"})
	require.True(t, res.IsError)
	text := res.Content[0].(*mcp.TextContent).Text
	kind, ok := ops.ParseKind(text)
	require.True(t, ok, text)
	assert.Equal(t, ops.KindNotFound, kind)
}
