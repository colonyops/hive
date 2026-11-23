package engineops

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/tasks"
)

func (h *Handlers) TaskCreate(ctx context.Context, in tasks.TaskCreateIn) (tasks.TaskListOut, error) {
	c, err := h.resolve(ctx, in.Caller)
	if err != nil {
		return tasks.TaskListOut{}, err
	}
	repo := h.repo(ctx, in.Repo, c)
	itemType := hc.ItemTypeTask
	if in.Type != "" {
		if itemType, err = hc.ParseItemType(in.Type); err != nil {
			return tasks.TaskListOut{}, ops.Errorf(ops.KindInvalid, "type %q is not epic or task", in.Type)
		}
	}

	if len(in.Children) == 0 {
		item, err := h.engine.HC().CreateItem(ctx, repo, hc.CreateItemInput{
			Title: in.Title, Desc: in.Desc, Type: itemType, ParentID: in.ParentID,
		})
		if err != nil {
			return tasks.TaskListOut{}, opError(err, "create item")
		}
		return tasks.TaskListOut{Tasks: []hc.Item{item}}, nil
	}

	root := hc.CreateInput{Ref: in.Ref, Title: in.Title, Desc: in.Desc, Type: itemType}
	if root.Children, err = decodeChildren(in.Children); err != nil {
		return tasks.TaskListOut{}, err
	}
	items, err := h.engine.HC().CreateBulk(ctx, repo, root)
	if err != nil {
		return tasks.TaskListOut{}, opError(err, "create items")
	}
	return tasks.TaskListOut{Tasks: items}, nil
}

// decodeChildren reads task_create's children. A child with no type is a
// task, as a single create is.
func decodeChildren(children []map[string]any) ([]hc.CreateInput, error) {
	out := make([]hc.CreateInput, 0, len(children))
	for _, child := range children {
		if t, ok := child["type"]; !ok || t == "" {
			child["type"] = string(hc.ItemTypeTask)
		}
		nested, _ := child["children"].([]any)
		delete(child, "children")
		raw, err := json.Marshal(child)
		if err != nil {
			return nil, ops.Errorf(ops.KindInvalid, "children: %v", err)
		}
		var in hc.CreateInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, ops.Errorf(ops.KindInvalid, "children: %v", err)
		}
		if strings.TrimSpace(in.Title) == "" {
			return nil, ops.Errorf(ops.KindInvalid, "children: every item needs a title")
		}
		if len(nested) > 0 {
			maps := make([]map[string]any, 0, len(nested))
			for _, n := range nested {
				m, ok := n.(map[string]any)
				if !ok {
					return nil, ops.Errorf(ops.KindInvalid, "children: an item is not an object")
				}
				maps = append(maps, m)
			}
			if in.Children, err = decodeChildren(maps); err != nil {
				return nil, err
			}
		}
		out = append(out, in)
	}
	return out, nil
}

func (h *Handlers) TaskList(ctx context.Context, in tasks.TaskListIn) (tasks.TaskListOut, error) {
	c, err := h.resolve(ctx, in.Caller)
	if err != nil {
		return tasks.TaskListOut{}, err
	}
	sessionID, err := localSessionID(in.SessionID)
	if err != nil {
		return tasks.TaskListOut{}, err
	}
	filter := hc.ListFilter{EpicID: in.EpicID, SessionID: sessionID}
	if !in.AllRepos {
		filter.RepoKey = h.scope(ctx, in.Repo, in.EpicID, c)
	}
	switch in.Status {
	case tasks.StatusAll:
	case "":
		open := hc.StatusOpen
		filter.Status = &open
	default:
		status, err := parseStatus(in.Status)
		if err != nil {
			return tasks.TaskListOut{}, err
		}
		filter.Status = &status
	}
	items, err := h.engine.HC().ListItems(ctx, filter)
	if err != nil {
		return tasks.TaskListOut{}, opError(err, "list items")
	}
	return tasks.TaskListOut{Tasks: nonNil(items)}, nil
}

// localSessionID strips the local: qualifier. Items live on this machine, so
// an id qualified with any other server can never match.
func localSessionID(value string) (string, error) {
	server, id, qualified := strings.Cut(value, ":")
	switch {
	case !qualified:
		return value, nil
	case server == "local":
		return id, nil
	default:
		return "", ops.Errorf(ops.KindInvalid,
			"sessionId %q names server %q; tasks live on this machine, so use a bare id or local:<id>", value, server)
	}
}

func (h *Handlers) TaskGet(ctx context.Context, in tasks.TaskGetIn) (tasks.TaskGetOut, error) {
	item, err := h.engine.HC().GetItem(ctx, in.TaskID)
	if err != nil {
		return tasks.TaskGetOut{}, opError(err, "get item")
	}
	comments, err := h.engine.HC().ListComments(ctx, in.TaskID)
	if err != nil {
		return tasks.TaskGetOut{}, opError(err, "list comments")
	}
	return tasks.TaskGetOut{Task: item, Comments: nonNil(comments)}, nil
}

func (h *Handlers) TaskUpdate(ctx context.Context, in tasks.TaskUpdateIn) (hc.Item, error) {
	var update hc.ItemUpdate
	if in.Status != "" {
		status, err := parseStatus(in.Status)
		if err != nil {
			return hc.Item{}, err
		}
		update.Status = &status
	}
	if in.Assign {
		c, err := h.resolveAgent(ctx, in.Caller, "assign")
		if err != nil {
			return hc.Item{}, err
		}
		update.SessionID = &c.ID
	}
	if in.Unassign {
		empty := ""
		update.SessionID = &empty
	}
	update.Title, update.Desc = in.Title, in.Desc

	// Field changes go first so that a refused one leaves the blockers as
	// they were.
	fields := update != (hc.ItemUpdate{})
	var item hc.Item
	if fields {
		var err error
		if item, err = h.engine.HC().UpdateItem(ctx, in.TaskID, update); err != nil {
			return hc.Item{}, opError(err, "update item")
		}
	}
	if in.AddBlocker != "" {
		if err := h.engine.HC().AddBlocker(ctx, in.AddBlocker, in.TaskID); err != nil {
			return hc.Item{}, opError(err, "add blocker")
		}
	}
	if in.RemoveBlocker != "" {
		if err := h.engine.HC().RemoveBlocker(ctx, in.RemoveBlocker, in.TaskID); err != nil {
			return hc.Item{}, opError(err, "remove blocker")
		}
	}
	// UpdateItem answers the row before the blocker edges change.
	if !fields || in.AddBlocker != "" || in.RemoveBlocker != "" {
		var err error
		if item, err = h.engine.HC().GetItem(ctx, in.TaskID); err != nil {
			return hc.Item{}, opError(err, "get item")
		}
	}
	return item, nil
}

func (h *Handlers) TaskNext(ctx context.Context, in tasks.TaskNextIn) (hc.Item, error) {
	resolve := h.resolve
	if in.Assign {
		resolve = func(ctx context.Context, value string) (caller, error) { return h.resolveAgent(ctx, value, "assign") }
	}
	c, err := resolve(ctx, in.Caller)
	if err != nil {
		return hc.Item{}, err
	}
	filter := hc.NextFilter{EpicID: in.EpicID, RepoKey: h.scope(ctx, in.Repo, in.EpicID, c)}
	if in.Assign {
		filter.SessionID = c.ID
	}
	item, found, err := h.engine.HC().Next(ctx, filter)
	if err != nil {
		return hc.Item{}, opError(err, "next item")
	}
	if !found {
		return hc.Item{}, ops.Errorf(ops.KindNotFound, "%s in epic %q", NoActionableTask, in.EpicID)
	}
	if !in.Assign || item.Status == hc.StatusInProgress {
		return item, nil
	}
	inProgress := hc.StatusInProgress
	item, err = h.engine.HC().UpdateItem(ctx, item.ID, hc.ItemUpdate{Status: &inProgress, SessionID: &c.ID})
	if err != nil {
		return hc.Item{}, opError(err, "assign item")
	}
	return item, nil
}

// NoActionableTask starts the not_found message of a task_next that found
// nothing to hand out.
const NoActionableTask = "no actionable task"

// scope is the repository a read of an epic is scoped to. An epic id already
// names the tree, and its items carry the repository it was created in, so
// only an explicit repo narrows it.
func (h *Handlers) scope(ctx context.Context, explicit, epicID string, c caller) string {
	if epicID != "" {
		return explicit
	}
	return h.repo(ctx, explicit, c)
}

func (h *Handlers) TaskCommentCreate(ctx context.Context, in tasks.TaskCommentCreateIn) (hc.Comment, error) {
	if strings.TrimSpace(in.Message) == "" {
		return hc.Comment{}, ops.Errorf(ops.KindInvalid, "message is empty")
	}
	comment, err := h.engine.HC().AddComment(ctx, in.TaskID, in.Message)
	if err != nil {
		return hc.Comment{}, opError(err, "add comment")
	}
	return comment, nil
}

func (h *Handlers) TaskContextGet(ctx context.Context, in tasks.TaskContextGetIn) (hc.ContextBlock, error) {
	c, err := h.resolve(ctx, in.Caller)
	if err != nil {
		return hc.ContextBlock{}, err
	}
	epic, err := h.engine.HC().GetItem(ctx, in.EpicID)
	if err != nil {
		return hc.ContextBlock{}, opError(err, "get epic")
	}
	if !epic.IsEpic() {
		return hc.ContextBlock{}, ops.Errorf(ops.KindInvalid, "%s is a %s, not an epic", in.EpicID, epic.Type)
	}
	block, err := h.engine.HC().Context(ctx, in.EpicID, c.ID)
	if err != nil {
		return hc.ContextBlock{}, opError(err, "epic context")
	}
	return block, nil
}

func (h *Handlers) TaskClear(ctx context.Context, in tasks.TaskClearIn) (tasks.TaskClearOut, error) {
	c, err := h.resolve(ctx, in.Caller)
	if err != nil {
		return tasks.TaskClearOut{}, err
	}
	var olderThan time.Duration
	if in.OlderThan != "" {
		if olderThan, err = time.ParseDuration(in.OlderThan); err != nil {
			return tasks.TaskClearOut{}, ops.Errorf(ops.KindInvalid, "olderThan %q is not a duration such as 24h", in.OlderThan)
		}
	}
	statuses := []hc.Status{hc.StatusDone, hc.StatusCancelled}
	if len(in.Statuses) > 0 {
		statuses = statuses[:0]
		for _, s := range in.Statuses {
			status, err := parseStatus(s)
			if err != nil {
				return tasks.TaskClearOut{}, err
			}
			statuses = append(statuses, status)
		}
	}
	repo := ""
	if !in.AllRepos {
		if repo = h.repo(ctx, in.Repo, c); repo == "" {
			return tasks.TaskClearOut{}, ops.Errorf(ops.KindInvalid,
				"the caller names no repository; pass repo, or allRepos to remove finished tasks of every repository")
		}
	}
	count, err := h.engine.HC().Prune(ctx, hc.PruneOpts{
		OlderThan: olderThan, Statuses: statuses, RepoKey: repo, DryRun: in.DryRun,
	})
	if err != nil {
		return tasks.TaskClearOut{}, opError(err, "prune")
	}
	action := "pruned"
	if in.DryRun {
		action = "would prune"
	}
	return tasks.TaskClearOut{Action: action, Count: count}, nil
}

func parseStatus(s string) (hc.Status, error) {
	status, err := hc.ParseStatus(s)
	if err != nil {
		return "", ops.Errorf(ops.KindInvalid, "status %q is not one of %s", s, strings.Join(hc.StatusNames(), ", "))
	}
	return status, nil
}

// nonNil keeps an empty list a JSON array, which the output schema requires.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
