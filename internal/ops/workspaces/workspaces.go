// Package workspaces declares the agent workspace and scheduled chat
// operations of the desktop's app-control MCP server.
package workspaces

import "github.com/colonyops/hive/internal/ops"

var Toolset = ops.Toolset{Name: "workspaces", MCPPath: "/mcp"}

func All() []ops.Op {
	return []ops.Op{WorkspaceList, ScheduleList, SchedulePut, ScheduleDelete, SchedulePreview, ScheduleRunList}
}

var WorkspaceList = ops.Spec[ops.NoInput]{
	Name: "workspace.list", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "List agent workspaces",
	Guidance: "List every agent workspace with its command template, whether that command carries a permission bypass (danger), and its schedule ids. " +
		"dir is what every schedule tool's workspace argument takes. A workspace whose manifest does not parse still lists, " +
		"with problem set; it refuses schedule writes until the file is fixed.",
}

type WorkspaceIn struct {
	Workspace string `json:"workspace" jsonschema:"The workspace directory name, as workspace_list reports it in dir. For the workspace this chat runs in, this process's HIVE_AGENT_WORKSPACE holds the workspace's absolute path: pass it as is, or its last path segment."`
}

var ScheduleList = ops.Spec[WorkspaceIn]{
	Name: "schedule.list", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "List a workspace's scheduled chats",
	Guidance: "List one workspace's schedules in manifest order, each with when it fires next and its newest run whatever the outcome. " +
		"A workspace that does not exist is not_found; one whose manifest does not parse is invalid, with the reason.",
}

// SchedulePutIn's optional fields are pointers so an omitted field can be
// told from one set to its zero value: omitted keeps what the schedule has.
type SchedulePutIn struct {
	Workspace string  `json:"workspace"          jsonschema:"The workspace directory name, as workspace_list reports it in dir. For the workspace this chat runs in, this process's HIVE_AGENT_WORKSPACE holds the workspace's absolute path: pass it as is, or its last path segment."`
	ID        string  `json:"id"                 jsonschema:"[a-z0-9-]+, unique in the workspace. An existing id is edited in place; a new one is appended."`
	Name      *string `json:"name,omitempty"     jsonschema:"Optional. Shown in the UI and used to name the launched chat; when empty, both fall back to the id, and schedule_list reports no name. Pass \"\" to clear a stored name."`
	Cron      *string `json:"cron,omitempty"     jsonschema:"A 5-field cron expression, or @hourly, @daily, @weekly, @monthly, @every 1h. Local time. Required for a new schedule."`
	Prompt    *string `json:"prompt,omitempty"   jsonschema:"A Go text/template rendered into the agent's opening message. Required for a new schedule. Variables: .Now, .ScheduledFor, .LastRun (unset on the first run: guard it with {{ if .LastRun }}), .Reason, .Missed, .Schedule.ID, .Schedule.Name, .Schedule.Cron, .Workspace.Dir, .Workspace.Name; date \"2006-01-02\" .LastRun formats a time and renders empty when it is unset. Write the task itself: Hive frames it with what started the chat and how to end the session. Use schedule_preview to check both renders."`
	Disabled  *bool   `json:"disabled,omitempty" jsonschema:"true pauses the schedule, false resumes it. Omitted, an existing schedule keeps its state."`
	OnMissed  *string `json:"onMissed,omitempty" jsonschema:"run (a new schedule's default) folds occurrences missed while the app was closed into one catch-up launch; skip records them instead of launching. Omitted, an existing schedule keeps its setting."`
}

var SchedulePut = ops.Spec[SchedulePutIn]{
	Name: "schedule.put", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary: "Create or update a scheduled chat",
	Guidance: "Create one schedule in a workspace's agent-workspace.yaml, or patch one that exists. A new id is appended and needs cron and prompt. " +
		"An existing id is patched in place, field by field: a field you omit keeps its stored value, so re-timing a paused schedule leaves it paused, " +
		"and name: \"\" clears the name. Every other key and comment in the file is kept. The cron and the prompt template are validated before " +
		"anything is written; a rejected edit is invalid and leaves the file as it was. The schedule is live as soon as the call returns. " +
		"A new or re-timed schedule never fires for a time before it existed.",
	Args:   []string{"id"},
	Fields: ops.Constraints{"onMissed": {Enum: []string{"run", "skip"}}},
}

type ScheduleIn struct {
	Workspace string `json:"workspace" jsonschema:"The workspace directory name, as workspace_list reports it in dir. For the workspace this chat runs in, this process's HIVE_AGENT_WORKSPACE holds the workspace's absolute path: pass it as is, or its last path segment."`
	ID        string `json:"id"`
}

var ScheduleDelete = ops.Spec[ScheduleIn]{
	Name: "schedule.delete", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary: "Remove a scheduled chat",
	Guidance: "Delete one schedule from a workspace's agent-workspace.yaml. Its run history stays readable through schedule_run_list. " +
		"An id that matches no schedule is not_found; nothing is ever reported as removed that was not.",
	Args: []string{"id"},
}

type SchedulePreviewIn struct {
	Workspace string `json:"workspace" jsonschema:"The workspace directory name, as workspace_list reports it in dir. For the workspace this chat runs in, this process's HIVE_AGENT_WORKSPACE holds the workspace's absolute path: pass it as is, or its last path segment."`
	Cron      string `json:"cron"`
	Prompt    string `json:"prompt"`
}

var SchedulePreview = ops.Spec[SchedulePreviewIn]{
	Name: "schedule.preview", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "Dry-run a schedule edit",
	Guidance: "Report the next occurrences a cron expression produces and the prompt template rendered against sample data, both with a previous run " +
		"behind it and as the first run sees it with .LastRun unset, without saving anything. " +
		"A cron or template that does not parse comes back in cronError or promptError rather than as a failed call. Use it before schedule_put.",
}

type ScheduleRunListIn struct {
	Workspace string `json:"workspace"       jsonschema:"The workspace directory name, as workspace_list reports it in dir. For the workspace this chat runs in, this process's HIVE_AGENT_WORKSPACE holds the workspace's absolute path: pass it as is, or its last path segment."`
	ID        string `json:"id"`
	Limit     int    `json:"limit,omitempty" jsonschema:"How many newest runs to return. 0 takes the default of 50."`
}

var ScheduleRunList = ops.Spec[ScheduleRunListIn]{
	Name: "schedule_run.list", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "Read a schedule's run history",
	Guidance: "List one schedule's runs, newest first: what each honored, why it ran, whether it launched a chat, and any error. " +
		"An empty list means the schedule has not run. History outlives the schedule, so a removed schedule still answers; a workspace that does not exist, " +
		"or an id that is neither declared nor has ever run, is not_found. A run's session is the chat it launched, gone once that chat has ended. " +
		"Running a schedule now is not a tool here: it spawns an agent CLI, which this server never does. That is the Chats area's \"Run now\".",
	Args: []string{"id"},
}
