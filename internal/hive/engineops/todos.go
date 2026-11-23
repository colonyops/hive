package engineops

import (
	"context"
	"strings"

	"github.com/colonyops/hive/internal/domain/todo"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/todos"
	"github.com/colonyops/hive/pkg/randid"
)

func (h *Handlers) TodoCreate(ctx context.Context, in todos.TodoCreateIn) (todos.TodoOut, error) {
	source := todo.SourceAgent
	if in.Source != "" {
		var err error
		if source, err = todo.ParseSource(in.Source); err != nil {
			return todos.TodoOut{}, ops.Errorf(ops.KindInvalid, "source %q is not one of %s", in.Source, strings.Join(todo.SourceNames(), ", "))
		}
	}
	c, err := h.resolve(ctx, in.Caller)
	if err != nil {
		return todos.TodoOut{}, err
	}
	uri := in.URI
	if uri == "" && c.Session != nil {
		uri = "session://" + c.Session.ID
	}
	ref, err := todo.ParseRef(uri)
	if err != nil {
		return todos.TodoOut{}, ops.Errorf(ops.KindInvalid, "uri: %v", err)
	}

	id := randid.Generate(8)
	var t todo.Todo
	switch source {
	case todo.SourceHuman:
		t, err = todo.NewHumanTodo(id, in.Title, ref)
	case todo.SourceSystem:
		t, err = todo.NewSystemTodo(id, in.Title, ref)
	default:
		t, err = todo.NewAgentTodo(id, in.Title, c.ID, ref)
	}
	if err != nil {
		return todos.TodoOut{}, ops.Errorf(ops.KindInvalid, "%v", err)
	}
	created, err := h.engine.Todos().Add(ctx, t)
	if err != nil {
		return todos.TodoOut{}, opError(err, "add todo")
	}
	return todos.OutOf(created), nil
}

func (h *Handlers) TodoList(ctx context.Context, in todos.TodoListIn) (todos.TodoListOut, error) {
	var filter todo.ListFilter
	if in.Status != "" {
		status, err := todo.ParseStatus(in.Status)
		if err != nil {
			return todos.TodoListOut{}, ops.Errorf(ops.KindInvalid, "status %q is not one of %s", in.Status, strings.Join(todo.StatusNames(), ", "))
		}
		filter.Status = &status
	}
	list, err := h.engine.Todos().List(ctx, filter)
	if err != nil {
		return todos.TodoListOut{}, opError(err, "list todos")
	}
	out := todos.TodoListOut{Todos: make([]todos.TodoOut, 0, len(list))}
	for _, t := range list {
		out.Todos = append(out.Todos, todos.OutOf(t))
	}
	return out, nil
}

func (h *Handlers) TodoUpdate(ctx context.Context, in todos.TodoUpdateIn) (todos.TodoOut, error) {
	var (
		updated todo.Todo
		err     error
	)
	switch in.Status {
	case todo.StatusAcknowledged.String():
		updated, err = h.engine.Todos().Acknowledge(ctx, in.TodoID)
	case todo.StatusCompleted.String():
		updated, err = h.engine.Todos().Complete(ctx, in.TodoID)
	case todo.StatusDismissed.String():
		updated, err = h.engine.Todos().Dismiss(ctx, in.TodoID)
	default:
		return todos.TodoOut{}, ops.Errorf(ops.KindInvalid, "status %q is not one of %s", in.Status, strings.Join(todos.UpdateStatuses, ", "))
	}
	if err != nil {
		return todos.TodoOut{}, opError(err, "update todo")
	}
	return todos.OutOf(updated), nil
}
