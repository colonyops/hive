package mcpsrv

import (
	"context"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// The schedule tools answer times as RFC 3339 in local time, unlike the canvas
// tools' unix milliseconds: cron is local time, and "Friday at 09:00" is what
// a model reasons about. Running a schedule outside its timetable spawns an
// agent CLI, which this surface never does (see the package comment).

type listWorkspacesOutput struct {
	Workspaces []workspaceSummary `json:"workspaces"`
}

type workspaceSummary struct {
	Dir       string   `json:"dir"               jsonschema:"The directory name every schedule tool's workspace argument takes."`
	Name      string   `json:"name"`
	Command   string   `json:"command"`
	Danger    bool     `json:"danger"`
	Problem   string   `json:"problem,omitempty"`
	Schedules []string `json:"schedules"         jsonschema:"The ids of the workspace's schedules."`
}

type workspaceInput struct {
	Workspace string `json:"workspace" jsonschema:"The workspace directory name, as list_workspaces reports it in dir. For the workspace this chat runs in, this process's HIVE_AGENT_WORKSPACE holds the workspace's absolute path: pass it as is, or its last path segment."`
}

type listSchedulesOutput struct {
	Workspace string         `json:"workspace"`
	Schedules []scheduleView `json:"schedules"`
}

type scheduleView struct {
	ID        string           `json:"id"`
	Name      string           `json:"name,omitempty"      jsonschema:"As stored. Absent when the schedule has none; the UI and the launched chat then use the id."`
	Cron      string           `json:"cron"`
	Prompt    string           `json:"prompt"`
	Disabled  bool             `json:"disabled"`
	OnMissed  string           `json:"onMissed"`
	NextRunAt string           `json:"nextRunAt,omitempty" jsonschema:"RFC 3339, local time. Absent when the schedule is disabled or its cron does not parse."`
	LastRun   *scheduleRunView `json:"lastRun,omitempty"   jsonschema:"The newest run whatever its outcome. Absent when the schedule has never run."`
}

type scheduleRunView struct {
	ID           int64  `json:"id"`
	ScheduleID   string `json:"scheduleId"`
	ScheduledFor string `json:"scheduledFor"      jsonschema:"RFC 3339, local time: the occurrence this run honored."`
	StartedAt    string `json:"startedAt"`
	Reason       string `json:"reason"            jsonschema:"due, catch_up, or manual."`
	Status       string `json:"status"            jsonschema:"launched, skipped, or failed."`
	Missed       int    `json:"missed"            jsonschema:"How many earlier occurrences were folded into this run."`
	Session      *int64 `json:"session,omitempty" jsonschema:"The chat the run launched. Absent when it launched none, and once that chat has ended: a scheduled chat deletes itself when its task is done."`
	Error        string `json:"error,omitempty"`
}

// The optional fields are pointers so an omitted field can be told from one
// set to its zero value: omitted keeps what the schedule has.
type putScheduleInput struct {
	Workspace string  `json:"workspace"          jsonschema:"The workspace directory name, as list_workspaces reports it in dir. For the workspace this chat runs in, this process's HIVE_AGENT_WORKSPACE holds the workspace's absolute path: pass it as is, or its last path segment."`
	ID        string  `json:"id"                 jsonschema:"[a-z0-9-]+, unique in the workspace. An existing id is edited in place; a new one is appended."`
	Name      *string `json:"name,omitempty"     jsonschema:"Optional. Shown in the UI and used to name the launched chat; when empty, both fall back to the id, and list_schedules reports no name. Pass \"\" to clear a stored name."`
	Cron      *string `json:"cron,omitempty"     jsonschema:"A 5-field cron expression, or @hourly, @daily, @weekly, @monthly, @every 1h. Local time. Required for a new schedule."`
	Prompt    *string `json:"prompt,omitempty"   jsonschema:"A Go text/template rendered into the agent's opening message. Required for a new schedule. Variables: .Now, .ScheduledFor, .LastRun (unset on the first run: guard it with {{ if .LastRun }}), .Reason, .Missed, .Schedule.ID, .Schedule.Name, .Schedule.Cron, .Workspace.Dir, .Workspace.Name; date \"2006-01-02\" .LastRun formats a time and renders empty when it is unset. Write the task itself: Hive frames it with what started the chat and how to end the session. Use preview_schedule to check both renders."`
	Disabled  *bool   `json:"disabled,omitempty" jsonschema:"true pauses the schedule, false resumes it. Omitted, an existing schedule keeps its state."`
	OnMissed  *string `json:"onMissed,omitempty" jsonschema:"run (a new schedule's default) folds occurrences missed while the app was closed into one catch-up launch; skip records them instead of launching. Omitted, an existing schedule keeps its setting."`
}

type scheduleIDInput struct {
	Workspace string `json:"workspace" jsonschema:"The workspace directory name, as list_workspaces reports it in dir. For the workspace this chat runs in, this process's HIVE_AGENT_WORKSPACE holds the workspace's absolute path: pass it as is, or its last path segment."`
	ID        string `json:"id"`
}

type removeScheduleOutput struct {
	Removed bool `json:"removed"`
}

type previewScheduleInput struct {
	Workspace string `json:"workspace" jsonschema:"The workspace directory name, as list_workspaces reports it in dir. For the workspace this chat runs in, this process's HIVE_AGENT_WORKSPACE holds the workspace's absolute path: pass it as is, or its last path segment."`
	Cron      string `json:"cron"`
	Prompt    string `json:"prompt"`
}

type previewScheduleOutput struct {
	Next           []string `json:"next"                  jsonschema:"The next occurrences the cron produces, RFC 3339 local time. Empty when it does not parse."`
	Prompt         string   `json:"prompt"                jsonschema:"The template rendered against sample data with a previous run behind it. Empty when it does not render."`
	FirstRunPrompt string   `json:"firstRunPrompt"        jsonschema:"The same template as the first run sees it, with .LastRun unset. Empty when it does not render."`
	CronError      string   `json:"cronError,omitempty"`
	PromptError    string   `json:"promptError,omitempty" jsonschema:"Why the template does not render. One that only fails on the first run says so."`
}

type scheduleRunsInput struct {
	Workspace string `json:"workspace"       jsonschema:"The workspace directory name, as list_workspaces reports it in dir. For the workspace this chat runs in, this process's HIVE_AGENT_WORKSPACE holds the workspace's absolute path: pass it as is, or its last path segment."`
	ID        string `json:"id"`
	Limit     int    `json:"limit,omitempty" jsonschema:"How many newest runs to return. 0 takes the default of 50."`
}

type scheduleRunsOutput struct {
	Runs []scheduleRunView `json:"runs"`
}

func (ctrl *Controller) ListWorkspaces(ctx context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, listWorkspacesOutput, error) {
	views, err := ctrl.core.AgentWorkspaces.List(ctx)
	if err != nil {
		return nil, listWorkspacesOutput{}, ctrl.toolError(err)
	}
	out := listWorkspacesOutput{Workspaces: make([]workspaceSummary, 0, len(views))}
	for _, w := range views {
		ids := make([]string, 0, len(w.Schedules))
		for _, s := range w.Schedules {
			ids = append(ids, s.ID)
		}
		out.Workspaces = append(out.Workspaces, workspaceSummary{
			Dir: w.Dir, Name: w.Name, Command: w.Command, Danger: w.Danger, Problem: w.Problem, Schedules: ids,
		})
	}
	return nil, out, nil
}

func (ctrl *Controller) ListSchedules(ctx context.Context, _ *mcp.CallToolRequest, in workspaceInput) (*mcp.CallToolResult, listSchedulesOutput, error) {
	workspace := ctrl.workspaceDir(in.Workspace)
	rows, err := ctrl.core.Schedules.List(ctx, workspace)
	if err != nil {
		return nil, listSchedulesOutput{}, ctrl.toolError(err)
	}
	out := listSchedulesOutput{Workspace: workspace, Schedules: make([]scheduleView, 0, len(rows))}
	for _, row := range rows {
		out.Schedules = append(out.Schedules, scheduleViewFrom(row))
	}
	return nil, out, nil
}

func (ctrl *Controller) PutSchedule(ctx context.Context, _ *mcp.CallToolRequest, in putScheduleInput) (*mcp.CallToolResult, scheduleView, error) {
	row, err := ctrl.core.AgentWorkspaces.PutSchedule(ctx, ctrl.workspaceDir(in.Workspace), app.SchedulePatch{
		ID: in.ID, Name: in.Name, Cron: in.Cron, Prompt: in.Prompt, Disabled: in.Disabled, OnMissed: in.OnMissed,
	})
	if err != nil {
		return nil, scheduleView{}, ctrl.toolError(err)
	}
	return nil, scheduleViewFrom(row), nil
}

func (ctrl *Controller) RemoveSchedule(ctx context.Context, _ *mcp.CallToolRequest, in scheduleIDInput) (*mcp.CallToolResult, removeScheduleOutput, error) {
	if err := ctrl.core.AgentWorkspaces.RemoveSchedule(ctx, ctrl.workspaceDir(in.Workspace), in.ID); err != nil {
		return nil, removeScheduleOutput{}, ctrl.toolError(err)
	}
	return nil, removeScheduleOutput{Removed: true}, nil
}

func (ctrl *Controller) PreviewSchedule(ctx context.Context, _ *mcp.CallToolRequest, in previewScheduleInput) (*mcp.CallToolResult, previewScheduleOutput, error) {
	preview, err := ctrl.core.Schedules.Preview(ctx, app.PreviewRequest{Workspace: ctrl.workspaceDir(in.Workspace), Cron: in.Cron, Prompt: in.Prompt})
	if err != nil {
		return nil, previewScheduleOutput{}, ctrl.toolError(err)
	}
	out := previewScheduleOutput{
		Next: make([]string, 0, len(preview.Next)), Prompt: preview.Prompt, FirstRunPrompt: preview.FirstRunPrompt,
		CronError: preview.CronError, PromptError: preview.PromptError,
	}
	for _, at := range preview.Next {
		out.Next = append(out.Next, localTime(at))
	}
	return nil, out, nil
}

func (ctrl *Controller) ScheduleRuns(ctx context.Context, _ *mcp.CallToolRequest, in scheduleRunsInput) (*mcp.CallToolResult, scheduleRunsOutput, error) {
	runs, err := ctrl.core.Schedules.Runs(ctx, ctrl.workspaceDir(in.Workspace), in.ID, in.Limit)
	if err != nil {
		return nil, scheduleRunsOutput{}, ctrl.toolError(err)
	}
	out := scheduleRunsOutput{Runs: make([]scheduleRunView, 0, len(runs))}
	for _, run := range runs {
		out.Runs = append(out.Runs, scheduleRunViewFrom(run))
	}
	return nil, out, nil
}

// workspaceDir accepts an absolute path where a directory name is expected,
// because HIVE_AGENT_WORKSPACE, which the argument descriptions send an agent
// to, holds the path. Anything but a direct child of the root passes through
// to the app's own validation.
func (ctrl *Controller) workspaceDir(arg string) string {
	if !filepath.IsAbs(arg) {
		return arg
	}
	root := ctrl.core.RuntimePaths().AgentWorkspacesDir
	if root == "" || filepath.Dir(filepath.Clean(arg)) != filepath.Clean(root) {
		return arg
	}
	return filepath.Base(arg)
}

func scheduleViewFrom(row app.ScheduleView) scheduleView {
	view := scheduleView{
		ID: row.ID, Name: row.Name, Cron: row.Cron, Prompt: row.Prompt,
		Disabled: row.Disabled, OnMissed: row.OnMissed,
	}
	if row.NextRunAt != nil {
		view.NextRunAt = localTime(*row.NextRunAt)
	}
	if row.LastRun != nil {
		last := scheduleRunViewFrom(*row.LastRun)
		view.LastRun = &last
	}
	return view
}

func scheduleRunViewFrom(run app.RunView) scheduleRunView {
	return scheduleRunView{
		ID: run.ID, ScheduleID: run.ScheduleID,
		ScheduledFor: localTime(run.ScheduledFor), StartedAt: localTime(run.StartedAt),
		Reason: run.Reason, Status: run.Status, Missed: run.Missed,
		Session: run.SessionID, Error: run.Error,
	}
}

func localTime(unixMilli int64) string {
	return time.UnixMilli(unixMilli).Local().Format(time.RFC3339)
}
