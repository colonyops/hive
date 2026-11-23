// Package tasks declares the Honeycomb task operations: epics and tasks that
// agents create, claim, comment on and finish. They run on the engine, so the
// hive CLI serves them without the desktop.
package tasks

import (
	"errors"
	"strings"

	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/ops"
)

var Toolset = ops.Toolset{Name: "tasks", MCPPath: "/mcp"}

func All() []ops.Op {
	return []ops.Op{TaskCreate, TaskList, TaskGet, TaskUpdate, TaskNext, TaskCommentCreate, TaskContextGet, TaskClear}
}

var (
	types    = []string{string(hc.ItemTypeEpic), string(hc.ItemTypeTask)}
	statuses = []string{string(hc.StatusOpen), string(hc.StatusInProgress), string(hc.StatusDone), string(hc.StatusCancelled)}
)

// StatusAll is the task_list status that keeps every status.
const StatusAll = "all"

const (
	repoGuidance = "A task belongs to a repository (owner/repo): repo when given, else the remote of the caller's hive session or working directory, else none."
	epicGuidance = "An epicId already scopes the read, so only an explicit repo narrows it."
)

type TaskCreateIn struct {
	Caller   string           `json:"caller"             jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory; outside one it is global. Never guess or reuse another value."`
	Repo     string           `json:"repo,omitempty"     jsonschema:"The repository the items belong to, as owner/repo. Defaults to the caller session's repository."`
	Ref      string           `json:"ref,omitempty"      jsonschema:"A label for this item that a child's blockers may name. Not stored."`
	Title    string           `json:"title"              jsonschema:"The item's title."`
	Type     string           `json:"type,omitempty"     jsonschema:"epic for a root item, or task (default)."`
	Desc     string           `json:"desc,omitempty"     jsonschema:"The item's description."`
	ParentID string           `json:"parentId,omitempty" jsonschema:"The parent item's id. A task without children needs one."`
	Tags     []string         `json:"tags,omitempty"     jsonschema:"Tags for the item. Tags compare case-insensitively, keep the first spelling, are at most 40 characters and cannot contain commas."`
	Children []map[string]any `json:"children,omitempty" jsonschema:"Items to create under this one, each {title, type, desc, tags, ref, blockers, assignSession, children}; type defaults to task. ref is a label local to this call; blockers lists the refs that must finish first. Nesting goes up to 10 levels."`

	AssignSession string `json:"assignSession,omitempty" jsonschema:"Assign the item to this hive session or chat id (a local:<id> reference is accepted). The session must exist. For a tree, set it on each child instead."`
}

func (in TaskCreateIn) Validate() error {
	if strings.TrimSpace(in.Title) == "" {
		return errors.New("title is empty")
	}
	if in.ParentID != "" && len(in.Children) > 0 {
		return errors.New("parentId and children cannot be combined: a tree starts at a new root")
	}
	if len(in.Children) > 0 && in.Type != string(hc.ItemTypeEpic) {
		return errors.New("an item with children is the root of a tree and must be an epic")
	}
	if in.AssignSession != "" && len(in.Children) > 0 {
		return errors.New("assignSession assigns one item; for a tree, set assignSession on each child")
	}
	if len(in.Children) == 0 && in.ParentID == "" && in.Type != string(hc.ItemTypeEpic) {
		return errors.New("a task needs a parentId; use type epic for a root item")
	}
	return nil
}

var TaskCreate = ops.Spec[TaskCreateIn]{
	Name: "task.create", Owner: ops.OwnerEngine, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Create a task or an epic with its tasks",
	Guidance: "Create one item, or a tree: an item with children, each of which may have children of its own. " +
		"task_next walks the tree and hands out only leaf tasks; a parent task resolves when its children are done. " +
		"To order work, give an item a ref and list that ref in a later item's blockers. " +
		"To hand an item to a session you dispatched, set its assignSession. " + repoGuidance + " " +
		"Answers with every item it created, root first.",
	Args:   []string{"title"},
	Fields: ops.Constraints{"type": {Enum: types, Default: string(hc.ItemTypeTask)}},
}

type TaskListIn struct {
	Caller    string   `json:"caller"              jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory; outside one it is global. Never guess or reuse another value."`
	EpicID    string   `json:"epicId,omitempty"    jsonschema:"Only the items of this epic."`
	Status    string   `json:"status,omitempty"    jsonschema:"Only items in this status, or all."`
	SessionID string   `json:"sessionId,omitempty" jsonschema:"Only items assigned to this session or chat id. A local:<id> reference is accepted."`
	Repo      string   `json:"repo,omitempty"      jsonschema:"Only items of this repository, as owner/repo. Defaults to the caller session's repository."`
	AllRepos  bool     `json:"allRepos,omitempty"  jsonschema:"Items of every repository; repo is ignored."`
	Tags      []string `json:"tags,omitempty"      jsonschema:"Only items carrying every one of these tags. Tags compare case-insensitively."`
}

var TaskList = ops.Spec[TaskListIn]{
	Name: "task.list", Owner: ops.OwnerEngine, Effect: ops.EffectRead, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "List tasks and epics",
	Guidance: "List items, open ones by default. " + repoGuidance + " " + epicGuidance + " " +
		"An empty list means nothing matched.",
	Args:   []string{"epicId"},
	Fields: ops.Constraints{"status": {Enum: append([]string{StatusAll}, statuses...), Default: string(hc.StatusOpen)}},
}

type TaskGetIn struct {
	TaskID string `json:"taskId" jsonschema:"The item's id."`
}

var TaskGet = ops.Spec[TaskGetIn]{
	Name: "task.get", Owner: ops.OwnerEngine, Effect: ops.EffectRead, Toolset: Toolset,
	Summary:  "Read a task or an epic with its comments",
	Guidance: "Answers with the item and its comments, oldest first. A comment that starts with CHECKPOINT is a handoff note.",
	Args:     []string{"taskId"},
}

type TaskUpdateIn struct {
	Caller        string   `json:"caller"                  jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory; outside one it is global. Never guess or reuse another value."`
	TaskID        string   `json:"taskId"                  jsonschema:"The item's id."`
	Status        string   `json:"status,omitempty"        jsonschema:"The new status."`
	Title         *string  `json:"title,omitempty"         jsonschema:"The new title."`
	Desc          *string  `json:"desc,omitempty"          jsonschema:"The new description."`
	Assign        bool     `json:"assign,omitempty"        jsonschema:"Assign the item to the caller, which must be a hive session or a chat."`
	AssignSession string   `json:"assignSession,omitempty" jsonschema:"Assign the item to this hive session or chat id (a local:<id> reference is accepted), for a caller that hands out work. The session must exist."`
	Unassign      bool     `json:"unassign,omitempty"      jsonschema:"Remove the item's assignment."`
	AddBlocker    string   `json:"addBlocker,omitempty"    jsonschema:"The id of an item that must finish before this one."`
	RemoveBlocker string   `json:"removeBlocker,omitempty" jsonschema:"The id of a blocker to remove."`
	ClearTags     bool     `json:"clearTags,omitempty"     jsonschema:"Remove every tag, before addTag and removeTag apply."`
	AddTag        []string `json:"addTag,omitempty"        jsonschema:"Tags to add. A tag the item already carries is kept as it is."`
	RemoveTag     []string `json:"removeTag,omitempty"     jsonschema:"Tags to remove, after the adds. A tag the item does not carry is ignored."`
}

func (in TaskUpdateIn) Validate() error {
	switch {
	case in.Status == "" && in.Title == nil && in.Desc == nil && !in.Assign && in.AssignSession == "" && !in.Unassign && in.AddBlocker == "" && in.RemoveBlocker == "" &&
		!in.ClearTags && len(in.AddTag) == 0 && len(in.RemoveTag) == 0:
		return errors.New("give at least one of status, title, desc, assign, assignSession, unassign, addBlocker, removeBlocker, clearTags, addTag or removeTag")
	case countTrue(in.Assign, in.AssignSession != "", in.Unassign) > 1:
		return errors.New("assign, assignSession and unassign cannot be combined")
	case in.AddBlocker != "" && in.RemoveBlocker != "":
		return errors.New("addBlocker and removeBlocker cannot be combined")
	case in.Title != nil && *in.Title == "":
		return errors.New("title cannot be empty")
	}
	return nil
}

var TaskUpdate = ops.Spec[TaskUpdateIn]{
	Name: "task.update", Owner: ops.OwnerEngine, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Change a task's status, text, assignment, tags or blockers",
	Guidance: "Apply the changes given and answer with the item as it is now. Setting an epic to done or cancelled " +
		"sets every unfinished item under it the same way. Tag changes apply in order: clearTags, then addTag, then removeTag; " +
		"two agents that each add a tag both keep theirs. A blocker that would make a cycle is a conflict, and the " +
		"other changes of the call are kept.",
	Args:   []string{"taskId"},
	Fields: ops.Constraints{"status": {Enum: statuses}},
}

type TaskNextIn struct {
	Caller string `json:"caller"           jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory; outside one it is global. Never guess or reuse another value."`
	EpicID string `json:"epicId"           jsonschema:"The epic to take work from."`
	Repo   string `json:"repo,omitempty"   jsonschema:"Only tasks of this repository, as owner/repo. Defaults to the caller session's repository."`
	Assign bool   `json:"assign,omitempty" jsonschema:"Claim the task: resume one already in progress for the caller, else assign the next open one to the caller and set it in progress."`
}

var TaskNext = ops.Spec[TaskNextIn]{
	Name: "task.next", Owner: ops.OwnerEngine, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Get the next actionable task of an epic",
	Guidance: "Answer with the next leaf task of the epic that is open or in progress and has no unfinished children or " +
		"blockers. Without assign it changes nothing. With assign it claims the task for the caller, which must be a " +
		"hive session or a chat. No actionable task is not_found. " + epicGuidance,
	Args: []string{"epicId"},
}

type TaskCommentCreateIn struct {
	TaskID  string `json:"taskId"  jsonschema:"The item's id."`
	Message string `json:"message" jsonschema:"The comment. Start it with CHECKPOINT: to leave a handoff note for the next agent."`
}

var TaskCommentCreate = ops.Spec[TaskCommentCreateIn]{
	Name: "task_comment.create", Owner: ops.OwnerEngine, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary:  "Comment on a task",
	Guidance: "Attach a progress note, a decision or a handoff to an item, and answer with the comment.",
	Args:     []string{"taskId", "message"},
}

type TaskContextGetIn struct {
	Caller string `json:"caller" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory; outside one it is global. Never guess or reuse another value."`
	EpicID string `json:"epicId" jsonschema:"The epic's id."`
}

var TaskContextGet = ops.Spec[TaskContextGetIn]{
	Name: "task_context.get", Owner: ops.OwnerEngine, Effect: ops.EffectRead, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary:  "Read an epic's context block",
	Guidance: "Answer with the epic, its task counts by status, the caller's tasks with their latest comment, and every open or in-progress task.",
	Args:     []string{"epicId"},
}

type TaskClearIn struct {
	Caller    string   `json:"caller"              jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory; outside one it is global. Never guess or reuse another value."`
	OlderThan string   `json:"olderThan,omitempty" jsonschema:"Only items last changed longer ago than this Go duration, such as 24h. Default: any age."`
	Statuses  []string `json:"statuses,omitempty"  jsonschema:"The statuses to remove. Default: done and cancelled."`
	Repo      string   `json:"repo,omitempty"      jsonschema:"Only items of this repository, as owner/repo. Defaults to the caller session's repository."`
	AllRepos  bool     `json:"allRepos,omitempty"  jsonschema:"Remove the finished tasks of every repository; repo is ignored."`
	DryRun    bool     `json:"dryRun,omitempty"    jsonschema:"Count what would be removed and remove nothing."`
}

var TaskClear = ops.Spec[TaskClearIn]{
	Name: "task.clear", Owner: ops.OwnerEngine, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Remove finished tasks",
	Guidance: "Delete done and cancelled items, or the statuses given, and answer with how many it removed. " + repoGuidance +
		" A caller that names no repository must pass allRepos to clear every repository.",
}

// TaskListOut answers task_create and task_list.
type TaskListOut struct {
	Tasks []hc.Item `json:"tasks"`
}

func (o TaskListOut) JSONLines() []any { return lines(o.Tasks) }

type TaskGetOut struct {
	Task     hc.Item      `json:"task"`
	Comments []hc.Comment `json:"comments"`
}

// JSONLines prints the item, then its comments.
func (o TaskGetOut) JSONLines() []any { return append([]any{o.Task}, lines(o.Comments)...) }

type TaskClearOut struct {
	Action string `json:"action" jsonschema:"pruned, or would prune for a dry run."`
	Count  int    `json:"count"`
}

func countTrue(values ...bool) int {
	n := 0
	for _, v := range values {
		if v {
			n++
		}
	}
	return n
}

func lines[T any](items []T) []any {
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}
