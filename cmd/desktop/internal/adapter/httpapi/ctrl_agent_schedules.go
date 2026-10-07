package httpapi

import (
	"net/http"

	"github.com/hay-kot/criterio"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// The schedule routes sit under the terminal prefix because a schedule
// launches an agent CLI, which is arbitrary command execution (ADR
// terminal-transport). Schedules are read and written through the workspace
// view; there is no per-schedule list or write route.

type agentScheduleView struct {
	Workspace string                `json:"workspace"`
	ID        string                `json:"id"`
	Name      string                `json:"name"`
	Cron      string                `json:"cron"`
	Prompt    string                `json:"prompt"`
	Disabled  bool                  `json:"disabled"`
	OnMissed  string                `json:"onMissed"`
	NextRunAt *int64                `json:"nextRunAt"`
	LastRun   *agentScheduleRunView `json:"lastRun"`
}

type agentScheduleRunView struct {
	ID           int64   `json:"id"`
	Workspace    string  `json:"workspace"`
	ScheduleID   string  `json:"scheduleId"`
	ScheduleName string  `json:"scheduleName"`
	ScheduledFor int64   `json:"scheduledFor"`
	StartedAt    int64   `json:"startedAt"`
	Reason       string  `json:"reason"`
	Status       string  `json:"status"`
	Missed       int     `json:"missed"`
	SessionID    *string `json:"sessionId"`
	Prompt       string  `json:"prompt"`
	Error        string  `json:"error"`
}

func toAgentScheduleView(s app.ScheduleView) agentScheduleView {
	view := agentScheduleView{
		Workspace: s.Workspace, ID: s.ID, Name: s.Name, Cron: s.Cron, Prompt: s.Prompt,
		Disabled: s.Disabled, OnMissed: s.OnMissed, NextRunAt: s.NextRunAt,
	}
	if s.LastRun != nil {
		last := toAgentScheduleRunView(*s.LastRun)
		view.LastRun = &last
	}
	return view
}

func toAgentScheduleRunView(r app.RunView) agentScheduleRunView {
	return agentScheduleRunView{
		ID: r.ID, Workspace: r.Workspace, ScheduleID: r.ScheduleID, ScheduleName: r.ScheduleName,
		ScheduledFor: r.ScheduledFor, StartedAt: r.StartedAt, Reason: r.Reason, Status: r.Status,
		Missed: r.Missed, SessionID: r.SessionID, Prompt: r.Prompt, Error: r.Error,
	}
}

type agentScheduleIDRequest struct {
	Workspace string `json:"workspace"`
	ID        string `json:"id"`
}

func (b agentScheduleIDRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("workspace", b.Workspace, criterio.Required),
		criterio.Run("id", b.ID, criterio.Required),
	)
}

type agentScheduleRunResponse struct {
	Run agentScheduleRunView `json:"run"`
}

func (ctrl *Controller) AgentScheduleRun(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentScheduleIDRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	run, err := ctrl.core.Schedules.RunNow(r.Context(), body.Workspace, body.ID)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, agentScheduleRunResponse{Run: toAgentScheduleRunView(run)})
}

type agentScheduleRunsRequest struct {
	Workspace string `json:"workspace"`
	ID        string `json:"id"`
	// Limit of 0 takes the core's default.
	Limit int `json:"limit"`
}

func (b agentScheduleRunsRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("workspace", b.Workspace, criterio.Required),
		criterio.Run("id", b.ID, criterio.Required),
	)
}

type agentScheduleRunsResponse struct {
	Runs []agentScheduleRunView `json:"runs"`
}

func (ctrl *Controller) AgentScheduleRuns(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentScheduleRunsRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	runs, err := ctrl.core.Schedules.Runs(r.Context(), body.Workspace, body.ID, body.Limit)
	if err != nil {
		return err
	}
	views := make([]agentScheduleRunView, 0, len(runs))
	for _, run := range runs {
		views = append(views, toAgentScheduleRunView(run))
	}
	return server.JSON(w, http.StatusOK, agentScheduleRunsResponse{Runs: views})
}

type agentSchedulePreviewRequest struct {
	Workspace string `json:"workspace"`
	Cron      string `json:"cron"`
	Prompt    string `json:"prompt"`
}

// Validate requires only the workspace: the editor previews a half-typed cron
// and prompt on every keystroke, and their errors are the answer rather than a
// reason to refuse the call.
func (b agentSchedulePreviewRequest) Validate() error {
	return criterio.Run("workspace", b.Workspace, criterio.Required)
}

type agentSchedulePreviewResponse struct {
	Next        []int64 `json:"next"`
	Prompt      string  `json:"prompt"`
	CronError   string  `json:"cronError"`
	PromptError string  `json:"promptError"`
}

func (ctrl *Controller) AgentSchedulePreview(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentSchedulePreviewRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	preview, err := ctrl.core.Schedules.Preview(r.Context(), app.PreviewRequest{
		Workspace: body.Workspace, Cron: body.Cron, Prompt: body.Prompt,
	})
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, agentSchedulePreviewResponse{
		Next: preview.Next, Prompt: preview.Prompt, CronError: preview.CronError, PromptError: preview.PromptError,
	})
}
