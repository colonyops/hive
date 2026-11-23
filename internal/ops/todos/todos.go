// Package todos declares the operator todo operations: items an agent raises
// for a person's attention. They run on the engine.
package todos

import (
	"time"

	"github.com/colonyops/hive/internal/domain/todo"
	"github.com/colonyops/hive/internal/ops"
)

var Toolset = ops.Toolset{Name: "todos", MCPPath: "/mcp"}

func All() []ops.Op {
	return []ops.Op{TodoCreate, TodoList, TodoUpdate}
}

type TodoCreateIn struct {
	Caller string `json:"caller"           jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory; outside one it is global. Never guess or reuse another value."`
	Title  string `json:"title"            jsonschema:"What the operator should do."`
	URI    string `json:"uri,omitempty"    jsonschema:"What it is about, as scheme://value: review://.hive/plans/x.md, session://<id>, https://... Defaults to session://<id> for a hive session caller."`
	Source string `json:"source,omitempty" jsonschema:"Who raised it."`
}

var TodoCreate = ops.Spec[TodoCreateIn]{
	Name: "todo.create", Owner: ops.OwnerEngine, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Ask the operator for attention",
	Guidance: "Add a pending todo to the operator's list, such as a document to review, and answer with it. An agent " +
		"todo records the caller's session or chat. Agents are rate limited.",
	Fields: ops.Constraints{"source": {Enum: todo.SourceNames(), Default: todo.SourceAgent.String()}},
}

type TodoListIn struct {
	Status string `json:"status,omitempty" jsonschema:"Only todos in this status."`
}

var TodoList = ops.Spec[TodoListIn]{
	Name: "todo.list", Owner: ops.OwnerEngine, Effect: ops.EffectRead, Toolset: Toolset,
	Summary:  "List operator todos",
	Guidance: "Answer with the todos. An empty list means nothing matched.",
	Fields:   ops.Constraints{"status": {Enum: todo.StatusNames()}},
}

// UpdateStatuses are the statuses a todo can move to.
var UpdateStatuses = []string{todo.StatusAcknowledged.String(), todo.StatusCompleted.String(), todo.StatusDismissed.String()}

type TodoUpdateIn struct {
	TodoID string `json:"todoId" jsonschema:"The todo's id."`
	Status string `json:"status" jsonschema:"acknowledged once seen, completed when done, dismissed when not needed."`
}

var TodoUpdate = ops.Spec[TodoUpdateIn]{
	Name: "todo.update", Owner: ops.OwnerEngine, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary:  "Change a todo's status",
	Guidance: "Move a todo to acknowledged, completed or dismissed and answer with it. A todo cannot go back to pending.",
	Args:     []string{"todoId"},
	Fields:   ops.Constraints{"status": {Enum: UpdateStatuses}},
}

// TodoOut is a todo as every surface prints it: todo.Todo's JSON, with the uri
// typed as the string it marshals to.
type TodoOut struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	Source      string    `json:"source"`
	Title       string    `json:"title"`
	URI         string    `json:"uri"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CompletedAt time.Time `json:"completed_at,omitzero"`
}

func OutOf(t todo.Todo) TodoOut {
	return TodoOut{
		ID: t.ID, SessionID: t.SessionID, Source: t.Source.String(), Title: t.Title, URI: t.URI.String(),
		Status: t.Status.String(), CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, CompletedAt: t.CompletedAt,
	}
}

type TodoListOut struct {
	Todos []TodoOut `json:"todos"`
}

func (o TodoListOut) JSONLines() []any {
	out := make([]any, len(o.Todos))
	for i, t := range o.Todos {
		out[i] = t
	}
	return out
}
