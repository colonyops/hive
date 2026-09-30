package httpapi

import (
	"errors"
	"net/http"

	"github.com/hay-kot/criterio"
	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/agentws"
)

// The agent-workspace control plane sits under TerminalPathPrefix for exactly
// the reason the pop-up surface does: starting a session spawns an agent CLI,
// which is arbitrary command execution just as a pop-up shell is (ADR terminal-transport),
// and mounting it here is what gives it the terminal bearer token and CORS
// policy without a second rule. Sessions are tmux sessions named
// agentws-<terminal id> and ride the shared tmux data plane at
// TerminalStreamPath (ADR agent-workspace-sessions-are-tmux-sessions) — there is no agent-specific stream. Start and
// Resume attach server-side and return the active window id alongside the
// session name, so the area never has to drive the generic
// /api/terminal/attach control route to reach its own sessions.
const AgentWorkspacesPathPrefix = TerminalPathPrefix + "agents/"

// agentWorkspaceView is one row of the area's workspace list. Command and
// MCPs are on it because a workspace that can actuate the physical world says
// so where it is opened, not where it was configured (spec §7.2,
// ADR the-workspace-command-is-a-template).
type agentWorkspaceView struct {
	Dir     string   `json:"dir"`
	Name    string   `json:"name"`
	Command string   `json:"command"`
	MCPs    []string `json:"mcps"`
	Skills  []string `json:"skills"`
	Problem string   `json:"problem"`
	// Danger reports that Command carries a permission bypass this build
	// recognizes, so the row can warn without the user reading the flags.
	Danger bool `json:"danger"`
	// Notice explains an agent whose MCP wiring cannot bound its tool set to
	// the workspace's declared servers, empty when the wiring is bounded or
	// the workspace's manifest failed to parse.
	Notice string `json:"notice"`
	// Schedules is the workspace's schedules: list, each joined with its next
	// run and its last one. Never null on the wire.
	Schedules []agentScheduleView `json:"schedules"`
}

// agentSessionView is one row of a workspace's session list.
type agentSessionView struct {
	ID           int64  `json:"id"`
	Workspace    string `json:"workspace"`
	Name         string `json:"name"`
	Agent        string `json:"agent"`
	LastOpenedAt int64  `json:"lastOpenedAt"`
	// Slug is the tmux session name (agentws-<terminal id>) this session is addressed by
	// whether or not it is running; terminalId is what reports liveness.
	Slug string `json:"slug"`
	// TerminalID is the tmux session name (agentws-<terminal id>) addressed on
	// TerminalStreamPath, empty when nothing is running.
	TerminalID string `json:"terminalId"`
	// WindowID is TerminalID's active tmux window, needed to frame input and
	// output on the windowed tmux wire. Set only by Start/Resume, which
	// attach; a listing read leaves it empty even for a live session.
	WindowID string `json:"windowId"`
	// PaneID is WindowID's active pane at attach and frames client input.
	PaneID string `json:"paneId"`
	// Cols and Rows are tmux's own size for that window at attach — the grid
	// the pane must open at, which may differ from the cols/rows voted. 0
	// means tmux has not reported one. Set only by Start/Resume, like
	// windowId.
	Cols int `json:"cols"`
	Rows int `json:"rows"`
	// ResumeAttempted is false when this launch could not even try to resume —
	// the agent has no resume form.
	ResumeAttempted bool `json:"resumeAttempted"`
	// Notice carries a fresh-launch, unbounded-MCP, or missing-MCP explanation
	// for the UI to show beside the session.
	Notice string `json:"notice"`
	// ScheduleID names the schedule whose run started this chat, and is what
	// marks a scheduled chat in the sidebar. Empty when a person started it.
	ScheduleID string `json:"scheduleId"`
}

func toAgentWorkspaceView(w app.WorkspaceView) agentWorkspaceView {
	schedules := make([]agentScheduleView, 0, len(w.Schedules))
	for _, s := range w.Schedules {
		schedules = append(schedules, toAgentScheduleView(s))
	}
	return agentWorkspaceView{
		Dir: w.Dir, Name: w.Name, Command: w.Command,
		MCPs: nonNilStrings(w.MCPs), Skills: nonNilStrings(w.Skills), Problem: w.Problem,
		Danger: w.Danger, Notice: w.Notice, Schedules: schedules,
	}
}

// nonNilStrings normalizes nil to an empty, non-nil slice. encoding/json
// marshals a nil slice as the JSON literal null rather than [], and the
// frontend calls .length on every array field unconditionally — a workspace
// with no mcps: entries, or an open with nothing missing, must still answer
// with an empty array on the wire, not null.
func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func toAgentWorkspaceViews(in []app.WorkspaceView) []agentWorkspaceView {
	out := make([]agentWorkspaceView, 0, len(in))
	for _, w := range in {
		out = append(out, toAgentWorkspaceView(w))
	}
	return out
}

func toAgentSessionView(s app.SessionView) agentSessionView {
	return agentSessionView{
		ID: s.ID, Workspace: s.Workspace, Name: s.Name, Agent: s.Agent, LastOpenedAt: s.LastOpenedAt,
		Slug: s.Slug, TerminalID: s.TerminalID, WindowID: s.WindowID, PaneID: s.PaneID, Cols: s.Cols, Rows: s.Rows,
		ResumeAttempted: s.ResumeAttempted, Notice: s.Notice, ScheduleID: s.ScheduleID,
	}
}

func toAgentSessionViews(in []app.SessionView) []agentSessionView {
	out := make([]agentSessionView, 0, len(in))
	for _, s := range in {
		out = append(out, toAgentSessionView(s))
	}
	return out
}

// agentWorkspacesResponse is AgentWorkspaces' payload. Available/Error report
// whether ephemeral terminals can run at all in this build (the same axis
// PopupTerminalAvailability reports); RootProblem is a distinct axis — the
// configured root itself could not be created or opened at startup (spec
// §14, e.g. an unmounted volume or a signed-out iCloud Drive). Root is the
// configured path regardless of either answer, so the UI can point at the
// setting even when nothing can run.
type agentWorkspacesResponse struct {
	Root        string               `json:"root"`
	RootProblem string               `json:"rootProblem"`
	Available   bool                 `json:"available"`
	Error       string               `json:"error"`
	Workspaces  []agentWorkspaceView `json:"workspaces"`
	// Presets are the starter command templates the editor offers: the ones
	// this build ships plus one per agent profile in hive's config. They fill
	// the command field; they never constrain it.
	Presets []agentws.Preset `json:"presets"`
	// Editor is the configured "open in editor" target; an empty command means
	// none is configured and the UI says so instead of offering the action.
	Editor agentEditorView `json:"editor"`
}

// agentEditorView labels the workspaces/open-in-editor action: the configured
// command and the display title the UI shows ("Open in Zed").
type agentEditorView struct {
	Command string `json:"command"`
	Title   string `json:"title"`
}

// AgentWorkspaces lists every recognized workspace under the configured root.
func (ctrl *Controller) AgentWorkspaces(w http.ResponseWriter, r *http.Request) error {
	if _, err := terminalBody[struct{}](ctrl, w, r); err != nil {
		return err
	}
	workspaces, err := ctrl.core.AgentWorkspaces.List(r.Context())
	if err != nil {
		return err
	}
	available, errMsg := true, ""
	if avErr := ctrl.core.AgentWorkspaces.Available(r.Context()); avErr != nil {
		available, errMsg = false, agentErrorMessage(avErr)
	}
	editorCommand, editorTitle := ctrl.core.AgentWorkspaces.Editor(r.Context())
	presets := ctrl.core.AgentWorkspaces.Presets(r.Context())
	return server.JSON(w, http.StatusOK, agentWorkspacesResponse{
		Root:        ctrl.core.RuntimePaths().AgentWorkspacesDir,
		RootProblem: ctrl.core.AgentWorkspaces.RootProblem(r.Context()),
		Available:   available,
		Error:       errMsg,
		Workspaces:  toAgentWorkspaceViews(workspaces),
		Presets:     presets,
		Editor:      agentEditorView{Command: editorCommand, Title: editorTitle},
	})
}

// agentWorkspaceEditRequest carries the manifest fields the in-app editor
// writes; anything else the manifest says is preserved in place.
type agentWorkspaceEditRequest struct {
	Dir     string   `json:"dir"`
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Mcps    []string `json:"mcps"`
	Skills  []string `json:"skills"`
	// Schedules is the whole schedules: list. Omitting it clears the key, the
	// same way an omitted mcps does: the editor holds every entry while it is
	// open, so what it sends is what the manifest ends up saying.
	Schedules []agentWorkspaceScheduleEdit `json:"schedules"`
}

// agentWorkspaceScheduleEdit is one schedules: entry as the editor sends it.
// The cron expression is built by the editor's calendar form; the manifest
// stores cron either way.
type agentWorkspaceScheduleEdit struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Cron     string `json:"cron"`
	Prompt   string `json:"prompt"`
	Disabled bool   `json:"disabled"`
	OnMissed string `json:"onMissed"`
}

// Validate covers only the fields whose absence makes the request meaningless.
// A schedule's own shape -- its id, cron expression and prompt template -- is
// the core's to judge, so those failures come back as one classified error
// with the reason in it rather than as a field list assembled twice.
func (b agentWorkspaceEditRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("dir", b.Dir, criterio.Required),
		criterio.Run("name", b.Name, criterio.Required),
		criterio.Run("command", b.Command, criterio.Required),
	)
}

func (b agentWorkspaceEditRequest) toEdit() app.WorkspaceEdit {
	schedules := make([]app.ScheduleEdit, 0, len(b.Schedules))
	for _, s := range b.Schedules {
		schedules = append(schedules, app.ScheduleEdit{
			ID: s.ID, Name: s.Name, Cron: s.Cron, Prompt: s.Prompt,
			Disabled: s.Disabled, OnMissed: s.OnMissed,
		})
	}
	return app.WorkspaceEdit{
		Dir: b.Dir, Name: b.Name, Command: b.Command,
		MCPs: b.Mcps, Skills: b.Skills, Schedules: schedules,
	}
}

// AgentWorkspaceCreate makes a directory under the root with a fresh
// manifest and returns its view.
func (ctrl *Controller) AgentWorkspaceCreate(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentWorkspaceEditRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	view, err := ctrl.core.AgentWorkspaces.CreateWorkspace(r.Context(), body.toEdit())
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, toAgentWorkspaceView(view))
}

// AgentWorkspaceUpdate rewrites the editable fields of a workspace's manifest
// in place — comments and keys the editor does not own survive.
func (ctrl *Controller) AgentWorkspaceUpdate(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentWorkspaceEditRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	view, err := ctrl.core.AgentWorkspaces.UpdateWorkspace(r.Context(), body.toEdit())
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, toAgentWorkspaceView(view))
}

// agentWorkspaceDirRequest names a workspace by its directory — the id every
// per-workspace operation addresses it by.
type agentWorkspaceDirRequest struct {
	Dir string `json:"dir"`
}

func (b agentWorkspaceDirRequest) Validate() error {
	return criterio.Run("dir", b.Dir, criterio.Required)
}

type agentWorkspaceOpenResponse struct {
	Workspace agentWorkspaceView `json:"workspace"`
	Sessions  []agentSessionView `json:"sessions"`
	// MissingMCPs names ids the workspace declares that the catalogue does
	// not resolve; they are simply omitted from what was generated.
	MissingMCPs []string `json:"missingMcps"`
	// MissingPackages reports every name the workspace enables that
	// skills.yml does not define, each saying why it did not resolve.
	MissingPackages []agentMissingPackageView `json:"missingPackages"`
}

// agentMissingPackageView is one enabled name skills.yml does not define.
// Skill separates a name that is really a skill — what a manifest written
// before packages carries — from one that matches nothing; selectedBy names
// the packages that already select the skill, which is the fix.
type agentMissingPackageView struct {
	Name       string   `json:"name"`
	Skill      bool     `json:"skill"`
	SelectedBy []string `json:"selectedBy"`
}

// AgentWorkspaceOpen regenerates a workspace's disposable artifacts and
// returns its sessions. It is the only call that writes into a workspace.
func (ctrl *Controller) AgentWorkspaceOpen(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentWorkspaceDirRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	result, err := ctrl.core.AgentWorkspaces.Open(r.Context(), body.Dir)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, agentWorkspaceOpenResponse{
		Workspace:       toAgentWorkspaceView(result.Workspace),
		Sessions:        toAgentSessionViews(result.Sessions),
		MissingMCPs:     nonNilStrings(result.MissingMCPs),
		MissingPackages: toAgentMissingPackageViews(result.MissingPackages),
	})
}

func toAgentMissingPackageViews(in []app.MissingPackageItem) []agentMissingPackageView {
	out := make([]agentMissingPackageView, 0, len(in))
	for _, item := range in {
		out = append(out, agentMissingPackageView{
			Name: item.Name, Skill: item.Skill, SelectedBy: nonNilStrings(item.SelectedBy),
		})
	}
	return out
}

// AgentWorkspaceDelete ends every live terminal a workspace's sessions hold,
// deletes the workspace directory, and removes the session records
// (ADR deleting-a-workspace-deletes-its-directory).
func (ctrl *Controller) AgentWorkspaceDelete(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentWorkspaceDirRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.AgentWorkspaces.DeleteWorkspace(r.Context(), body.Dir); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// AgentWorkspaceOpenInEditor launches the configured editor (Settings ›
// General) on the workspace directory.
func (ctrl *Controller) AgentWorkspaceOpenInEditor(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentWorkspaceDirRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.AgentWorkspaces.OpenWorkspaceInEditor(r.Context(), body.Dir); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// AgentWorkspaceReveal opens the workspace directory in the OS file manager.
func (ctrl *Controller) AgentWorkspaceReveal(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentWorkspaceDirRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.AgentWorkspaces.RevealWorkspace(r.Context(), body.Dir); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// agentMCPServerView is one row of the merged MCP catalogue: id, provenance,
// and the resolved command line — nothing is enabled whose command the user
// cannot read first (ADR a-workspace-declares-its-own-authority).
type agentMCPServerView struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Shipped     bool   `json:"shipped"`
	Stability   string `json:"stability"`
	// Shadows is the shipped id this user entry replaces, empty otherwise.
	Shadows   string `json:"shadows"`
	Transport string `json:"transport"`
	// Command is what the entry launches: the command line for stdio, the URL
	// for http/sse.
	Command string `json:"command"`
	// Problem reports why the entry will not work — a stdio command that does
	// not resolve on PATH.
	Problem string `json:"problem"`
}

func (ctrl *Controller) mcpCatalogueViews(r *http.Request) []agentMCPServerView {
	items := ctrl.core.AgentWorkspaces.MCPCatalogue(r.Context())
	out := make([]agentMCPServerView, 0, len(items))
	for _, item := range items {
		out = append(out, agentMCPServerView{
			ID: item.ID, Title: item.Title, Description: item.Description,
			Shipped: item.Shipped, Stability: item.Stability, Shadows: item.Shadows,
			Transport: item.Transport, Command: item.Command, Problem: item.Problem,
		})
	}
	return out
}

type agentMCPCatalogueResponse struct {
	Servers []agentMCPServerView `json:"servers"`
}

// AgentMCPCatalogue lists the merged MCP catalogue: shipped entries plus the
// user's mcps.yaml, a user id shadowing a shipped one of the same name.
func (ctrl *Controller) AgentMCPCatalogue(w http.ResponseWriter, r *http.Request) error {
	if _, err := terminalBody[struct{}](ctrl, w, r); err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, agentMCPCatalogueResponse{Servers: ctrl.mcpCatalogueViews(r)})
}

type agentMCPImportRequest struct {
	// JSON is a pasted MCP configuration: claude's {"mcpServers": {...}}
	// wrapper or a bare {"<id>": {...}} map of servers.
	JSON string `json:"json"`
}

func (b agentMCPImportRequest) Validate() error {
	return criterio.Run("json", b.JSON, criterio.Required)
}

type agentMCPImportResponse struct {
	// Added names the imported server ids, sorted.
	Added   []string             `json:"added"`
	Servers []agentMCPServerView `json:"servers"`
}

// AgentMCPImport parses pasted MCP JSON into mcps.yaml and returns the
// refreshed catalogue.
func (ctrl *Controller) AgentMCPImport(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentMCPImportRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	added, err := ctrl.core.AgentWorkspaces.ImportMCPServers(r.Context(), body.JSON)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, agentMCPImportResponse{Added: nonNilStrings(added), Servers: ctrl.mcpCatalogueViews(r)})
}

type agentMCPRemoveRequest struct {
	ID string `json:"id"`
}

func (b agentMCPRemoveRequest) Validate() error {
	return criterio.Run("id", b.ID, criterio.Required)
}

// AgentMCPRemove deletes a user-declared server from mcps.yaml and returns
// the refreshed catalogue. Shipped entries are refused — a workspace disables
// one by dropping the id from its own mcps: list.
func (ctrl *Controller) AgentMCPRemove(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentMCPRemoveRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.AgentWorkspaces.RemoveMCPServer(r.Context(), body.ID); err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, agentMCPCatalogueResponse{Servers: ctrl.mcpCatalogueViews(r)})
}

// agentSkillPackageView is one row of the package catalogue: a package from
// skills.yml and the skills its patterns currently select.
type agentSkillPackageView struct {
	Name        string                 `json:"name"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Members     []agentSkillMemberView `json:"members"`
}

// agentSkillMemberView is one skill a package selects, with its provenance:
// shipped by this build (rendered per install) or authored in the shared
// skills directory.
type agentSkillMemberView struct {
	Slug    string `json:"slug"`
	Shipped bool   `json:"shipped"`
}

// agentSkillNameView is one name in the skills name-space with the packages
// selecting it. The editor needs it to tell an enabled name that is really a
// skill from one that matches nothing.
type agentSkillNameView struct {
	Slug       string   `json:"slug"`
	Shipped    bool     `json:"shipped"`
	SelectedBy []string `json:"selectedBy"`
}

type agentSkillPackagesResponse struct {
	Packages []agentSkillPackageView `json:"packages"`
	// Skills is the name-space the packages select over: every shipped skill
	// plus every one authored in the shared skills directory.
	Skills []agentSkillNameView `json:"skills"`
	// Problem is why skills.yml could not be read, empty when it is fine. The
	// packages listed are then the last-good set.
	Problem string `json:"problem"`
}

// AgentSkillPackages lists the skill packages a workspace can enable, each
// with the names its glob patterns currently select.
func (ctrl *Controller) AgentSkillPackages(w http.ResponseWriter, r *http.Request) error {
	if _, err := terminalBody[struct{}](ctrl, w, r); err != nil {
		return err
	}
	items, names, problem := ctrl.core.AgentWorkspaces.SkillPackages(r.Context())
	out := make([]agentSkillPackageView, 0, len(items))
	for _, item := range items {
		members := make([]agentSkillMemberView, 0, len(item.Members))
		for _, m := range item.Members {
			members = append(members, agentSkillMemberView{Slug: m.Slug, Shipped: m.Shipped})
		}
		out = append(out, agentSkillPackageView{
			Name: item.Name, Title: item.Title, Description: item.Description, Members: members,
		})
	}
	skills := make([]agentSkillNameView, 0, len(names))
	for _, name := range names {
		skills = append(skills, agentSkillNameView{
			Slug: name.Slug, Shipped: name.Shipped, SelectedBy: nonNilStrings(name.SelectedBy),
		})
	}
	return server.JSON(w, http.StatusOK, agentSkillPackagesResponse{Packages: out, Skills: skills, Problem: problem})
}

// AgentSkillPackagesReveal opens skills.yml, where packages are defined.
func (ctrl *Controller) AgentSkillPackagesReveal(w http.ResponseWriter, r *http.Request) error {
	if _, err := terminalBody[struct{}](ctrl, w, r); err != nil {
		return err
	}
	if err := ctrl.core.AgentWorkspaces.RevealSkillPackages(r.Context()); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// AgentSharedSkillsReveal opens the shared skills directory, where a skill a
// package can select is authored.
func (ctrl *Controller) AgentSharedSkillsReveal(w http.ResponseWriter, r *http.Request) error {
	if _, err := terminalBody[struct{}](ctrl, w, r); err != nil {
		return err
	}
	if err := ctrl.core.AgentWorkspaces.RevealSharedSkills(r.Context()); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type agentSessionsRequest struct {
	Workspace string `json:"workspace"`
}

func (b agentSessionsRequest) Validate() error {
	return criterio.Run("workspace", b.Workspace, criterio.Required)
}

type agentSessionsResponse struct {
	Sessions []agentSessionView `json:"sessions"`
}

// AgentSessions lists a workspace's sessions without regenerating its
// artifacts, unlike AgentWorkspaceOpen.
func (ctrl *Controller) AgentSessions(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentSessionsRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	sessions, err := ctrl.core.AgentWorkspaces.Sessions(r.Context(), body.Workspace)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, agentSessionsResponse{Sessions: toAgentSessionViews(sessions)})
}

// agentSessionStartRequest's Cols/Rows carry no Validate: 0x0 opens at the
// terminal's own default, the same convention popupOpenRequest follows.
type agentSessionStartRequest struct {
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

func (b agentSessionStartRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("workspace", b.Workspace, criterio.Required),
		criterio.Run("name", b.Name, criterio.Required),
	)
}

// AgentSessionStart launches a new, named session in a workspace: rendering
// its command template into a command line and opening it on a Hive-owned
// PTY.
func (ctrl *Controller) AgentSessionStart(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentSessionStartRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	view, err := ctrl.core.AgentWorkspaces.StartSession(r.Context(), app.StartSession{
		Workspace: body.Workspace, Name: body.Name, Cols: body.Cols, Rows: body.Rows,
	})
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, toAgentSessionView(view))
}

// AgentSessionStartFirstRun opens the seeded Hive workspace on the first-run
// interview. The workspace, the name and the prompt are the app's, so the
// request carries nothing.
func (ctrl *Controller) AgentSessionStartFirstRun(w http.ResponseWriter, r *http.Request) error {
	if _, err := terminalBody[struct{}](ctrl, w, r); err != nil {
		return err
	}
	view, err := ctrl.core.AgentWorkspaces.StartFirstRunChat(r.Context())
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, toAgentSessionView(view))
}

type agentSessionResumeRequest struct {
	ID   int64 `json:"id"`
	Cols int   `json:"cols"`
	Rows int   `json:"rows"`
}

func (b agentSessionResumeRequest) Validate() error {
	return criterio.Run("id", b.ID, criterio.Positive[int64]())
}

// AgentSessionResume reattaches a session's live terminal if it still has
// one, or relaunches it — resuming the agent's own conversation when it has a
// resume form (resumeAttempted), and starting fresh with a notice when it
// does not (spec §6.2).
func (ctrl *Controller) AgentSessionResume(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentSessionResumeRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	view, err := ctrl.core.AgentWorkspaces.ResumeSession(r.Context(), body.ID, body.Cols, body.Rows)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, toAgentSessionView(view))
}

type agentSessionIDRequest struct {
	ID int64 `json:"id"`
}

func (b agentSessionIDRequest) Validate() error {
	return criterio.Run("id", b.ID, criterio.Positive[int64]())
}

type agentSessionCloseResponse struct {
	Closed bool `json:"closed"`
}

// AgentSessionClose ends a session's live terminal and reports whether there
// was one to close. The record is untouched, so it still lists afterward.
func (ctrl *Controller) AgentSessionClose(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentSessionIDRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	closed, err := ctrl.core.AgentWorkspaces.CloseSession(r.Context(), body.ID)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, agentSessionCloseResponse{Closed: closed})
}

type agentSessionRenameRequest struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (b agentSessionRenameRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("id", b.ID, criterio.Positive[int64]()),
		criterio.Run("name", b.Name, criterio.Required),
	)
}

// AgentSessionRename sets a session's display name. The record is the only
// thing touched — a live tmux session keeps its agentws-<terminal id> name.
func (ctrl *Controller) AgentSessionRename(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentSessionRenameRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.AgentWorkspaces.RenameSession(r.Context(), body.ID, body.Name); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// agentSessionActivityRequest scopes activity classification to one
// workspace, or — with workspace empty — to every live session: the sidebar
// lists all sessions at once, so its indicators poll the whole set.
type agentSessionActivityRequest struct {
	Workspace string `json:"workspace"`
}

func (b agentSessionActivityRequest) Validate() error { return nil }

type agentSessionActivityItem struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

type agentSessionActivityResponse struct {
	Items []agentSessionActivityItem `json:"items"`
}

// AgentSessionActivity classifies each of a workspace's live sessions from
// its captured tmux pane: ready, active, or approval — approval is the
// highest-urgency state. A session with no live tmux session is omitted
// rather than reported dead; its row's terminalId already carries that.
func (ctrl *Controller) AgentSessionActivity(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentSessionActivityRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	items, err := ctrl.core.AgentWorkspaces.SessionActivity(r.Context(), body.Workspace)
	if err != nil {
		return err
	}
	out := make([]agentSessionActivityItem, 0, len(items))
	for _, it := range items {
		out = append(out, agentSessionActivityItem{ID: it.ID, Status: it.Status})
	}
	return server.JSON(w, http.StatusOK, agentSessionActivityResponse{Items: out})
}

// agentSessionResizeRequest votes a size for a session's attached control
// client. Like start/resume's cols/rows this is a vote, not an applied size:
// tmux answers over the stream with a window 'layout-changed' event, and that
// event is what sets the pane's grid.
type agentSessionResizeRequest struct {
	ID   int64 `json:"id"`
	Cols int   `json:"cols"`
	Rows int   `json:"rows"`
}

func (b agentSessionResizeRequest) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("id", b.ID, criterio.Positive[int64]()),
		criterio.Run("cols", b.Cols, criterio.Positive[int]()),
		criterio.Run("rows", b.Rows, criterio.Positive[int]()),
	)
}

// AgentSessionResize votes a size for a session's attached control client —
// the same renegotiation the Code view's panes cast on a host resize.
func (ctrl *Controller) AgentSessionResize(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentSessionResizeRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.AgentWorkspaces.ResizeSession(r.Context(), body.ID, body.Cols, body.Rows); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// AgentSessionDelete ends any live terminal and deletes a session's record.
func (ctrl *Controller) AgentSessionDelete(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[agentSessionIDRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if err := ctrl.core.AgentWorkspaces.DeleteSession(r.Context(), body.ID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type agentSessionsAllResponse struct {
	Sessions []agentSessionView `json:"sessions"`
}

// AgentSessionsAll lists every session across every workspace, most recently
// opened first — the cross-workspace read the Recents list uses, unlike
// AgentSessions which scopes to one workspace.
func (ctrl *Controller) AgentSessionsAll(w http.ResponseWriter, r *http.Request) error {
	if _, err := terminalBody[struct{}](ctrl, w, r); err != nil {
		return err
	}
	sessions, err := ctrl.core.AgentWorkspaces.AllSessions(r.Context())
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, agentSessionsAllResponse{Sessions: toAgentSessionViews(sessions)})
}

// agentErrorMessage takes the user-facing message off a core error, matching
// wailsui's reasonFor: anything not an *app.Error is reported verbatim.
func agentErrorMessage(err error) string {
	if appErr, ok := errors.AsType[*app.Error](err); ok {
		return appErr.Msg
	}
	return err.Error()
}
