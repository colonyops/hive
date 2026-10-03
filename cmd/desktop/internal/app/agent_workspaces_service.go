package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/colonyops/hive/pkg/osopen"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/agentws"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
	"github.com/colonyops/hive/cmd/desktop/internal/app/prompts"
	"github.com/colonyops/hive/cmd/desktop/internal/app/schedule"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/domain/terminal/assess"
	terminalstatus "github.com/colonyops/hive/internal/domain/terminal/status"
	"github.com/colonyops/hive/internal/platform/execenv"
	tmuxcc "github.com/colonyops/hive/internal/platform/tmux/control"
)

const (
	// agentSessionPrefix names every tmux session an agent workspace session
	// owns (sessionName below). It is also the service-level concurrency
	// cap's counting key: the cap moved here from ptyterm's own
	// maxConcurrentSessions when sessions stopped being ptyterm terminals.
	agentSessionPrefix = "agentws-"
	// maxConcurrentAgentSessions bounds live agentws-* tmux sessions across
	// every workspace. Each session is an agent CLI spawning its own copy of
	// every enabled MCP server, so with no idle reaping and no cap this is a
	// fork bomb with a progress bar (ADR ptyterm-terminals-are-caller-addressed point 4, carried over from
	// ptyterm to this service now that sessions outlive the app on purpose).
	maxConcurrentAgentSessions = 8

	// earlyExitWindow is how long a just-created tmux session gets to report
	// that its initial command already exited before StartSession/
	// ResumeSession report it as live. tmux ends a session when its initial
	// pane's command exits (remain-on-exit is not set), so a command not on
	// PATH or an agent CLI that fails to start reads as the session simply
	// vanishing -- usually within milliseconds, well inside this window even
	// accounting for a slow CI machine sourcing shell startup files. A real
	// agent CLI never exits this fast, so the window never misclassifies a
	// live session as dead.
	earlyExitWindow       = 500 * time.Millisecond
	earlyExitPollInterval = 25 * time.Millisecond
)

// AgentWorkspacesService opens agent workspaces and drives the sessions run
// inside them: an agent CLI in a tmux session named agentws-<terminal id>,
// resolved through the workspace's command template (agentws.Resolve) and
// addressed by a durable stores.AgentSession record. Sessions are tmux's, not this process's -- they outlive App.Close
// by design, which is what makes reopening a codex session (no resume form)
// a real reattach instead of a fresh relaunch.
type AgentWorkspacesService struct {
	store     *agentws.Store
	terminals *tmuxcc.Manager
	// tx spans the session and schedule aggregates a workspace delete clears.
	tx        transactionRunner
	sessions  *stores.AgentSessionStore
	schedules *stores.ScheduleStore
	itemLinks *stores.ItemSessionStore
	history   scheduleHistory
	skills    *SkillsService
	// This is a function so config reloads update editor presets. Launches do
	// not read it, so edits cannot change an existing workspace's command
	// (ADR the-workspace-command-is-a-template).
	profileCommands func() map[string]string
	// rootProblem carries EnsureRoot's error, verbatim, when the configured
	// root could not be created or opened at startup -- empty otherwise.
	rootProblem string
	// execEnv resolves the PATH and environment the editor launch runs with
	// (ADR subprocess-environment) — a desktop launch's own environment cannot find a CLI a
	// package manager installed.
	execEnv *execenv.Resolver
	// editorCommand reads the configured editor from settings on every call,
	// so a settings change applies without restarting. Empty means none
	// configured.
	editorCommand EditorCommandReader
	// mcpBase reads this run's own loopback base URL, empty when the server
	// is down. Read per call rather than captured, because the listener's
	// port is not known when this service is built and can change if it
	// rebinds.
	mcpBase  MCPBaseReader
	endDelay SessionEndDelayReader
	events   *events.Bus
	// OnSchedulesChanged is a field rather than a constructor argument because
	// the scheduler it reloads is built after this service, over it.
	OnSchedulesChanged func(workspace string)
	logger             zerolog.Logger
}

type transactionRunner interface {
	WithinTx(ctx context.Context, fn func(context.Context) error) error
}

// EditorCommandReader reads the configured editor on each call, so settings
// changes take effect without a restart. Empty means no configured editor.
type EditorCommandReader interface {
	Editor(ctx context.Context) (string, error)
}

// MCPBaseReader reads this run's loopback base URL. Empty means the server is
// down. Read per call, because the port is unknown when the service is built.
type MCPBaseReader interface {
	MCPBaseURL(ctx context.Context) string
}

type NopMCPBaseReader struct{}

func (NopMCPBaseReader) MCPBaseURL(context.Context) string { return "" }

// SessionEndDelayReader reads agent_workspaces.session_end_delay on each
// call. A non-positive answer takes the shipped value.
type SessionEndDelayReader interface {
	SessionEndDelay(ctx context.Context) time.Duration
}

type FixedSessionEndDelay time.Duration

func (d FixedSessionEndDelay) SessionEndDelay(context.Context) time.Duration {
	return time.Duration(d)
}

type AgentWorkspacesDeps struct {
	Store     *agentws.Store
	Terminals *tmuxcc.Manager
	// Stores supplies the session and schedule stores and the transaction a
	// workspace delete spans them with.
	Stores          *stores.Stores
	Skills          *SkillsService
	ProfileCommands func() map[string]string
	RootProblem     string
	ExecEnv         *execenv.Resolver
	// EditorCommand nil means NopEditorCommandReader.
	EditorCommand EditorCommandReader
	// MCPBase nil means NopMCPBaseReader.
	MCPBase MCPBaseReader
	// EndDelay nil means the shipped defaultEndDelay.
	EndDelay SessionEndDelayReader
	Events   *events.Bus
	Logger   zerolog.Logger
}

func newAgentWorkspacesService(d AgentWorkspacesDeps) *AgentWorkspacesService {
	if d.EditorCommand == nil {
		d.EditorCommand = NopEditorCommandReader{}
	}
	if d.MCPBase == nil {
		d.MCPBase = NopMCPBaseReader{}
	}
	if d.EndDelay == nil {
		d.EndDelay = FixedSessionEndDelay(defaultEndDelay)
	}
	return &AgentWorkspacesService{
		store:           d.Store,
		terminals:       d.Terminals,
		tx:              d.Stores,
		sessions:        d.Stores.AgentSessions,
		schedules:       d.Stores.Schedules,
		itemLinks:       d.Stores.ItemSessions,
		history:         scheduleHistory{store: d.Stores.Schedules, sessions: d.Stores.AgentSessions, logger: d.Logger},
		skills:          d.Skills,
		profileCommands: d.ProfileCommands,
		rootProblem:     d.RootProblem,
		execEnv:         d.ExecEnv,
		editorCommand:   d.EditorCommand,
		mcpBase:         d.MCPBase,
		endDelay:        d.EndDelay,
		events:          d.Events,
		logger:          d.Logger,
	}
}

// catalogue is the merged catalogue with this install's own entries resolved.
// mcpcatalog ships its app-hosted entries carrying no URL, because the
// loopback port is allocated at startup (Descriptor.RuntimeURL), so the live
// base joined with each entry's RuntimePath is substituted here — and an
// entry that cannot be resolved reports why rather than rendering an address
// nothing answers, the same posture problemFor takes for a command that is
// not on PATH.
func (s *AgentWorkspacesService) catalogue(ctx context.Context) []agentws.CatalogueEntry {
	entries := agentws.Catalogue(ctx, s.store.Library().Library, s.lookPath())
	base := s.mcpBase.MCPBaseURL(ctx)
	for i, entry := range entries {
		descriptor, ok := mcpcatalog.Lookup(entry.ID)
		// Shipped is false for a user entry shadowing this id, and the user's
		// own URL is then what they asked for.
		if !ok || !entry.Shipped || !descriptor.RuntimeURL {
			continue
		}
		if base == "" {
			entries[i].Problem = "the local HTTP server is not running; set http.enabled in settings.yaml"
			continue
		}
		entries[i].Server.URL = base + descriptor.RuntimePath
	}
	return entries
}

// lookPath is what the catalogue validates a stdio command against: the PATH
// a session launches with (ADR subprocess-environment), not the launchd one a
// desktop process inherits. Only a test builds this service without a
// resolver, and nil leaves it on this process's own PATH.
func (s *AgentWorkspacesService) lookPath() func(context.Context, string) (string, error) {
	if s.execEnv == nil {
		return nil
	}
	return s.execEnv.LookPath
}

// WorkspaceView is one row of the area's list. Command is on it because a
// workspace that can actuate the physical world says so where it is opened,
// not where it was configured (spec §7.2) — and with a free-form command the
// only honest way to say it is to show the command.
type WorkspaceView struct {
	Dir     string   `json:"dir"`
	Name    string   `json:"name"`
	Command string   `json:"command"`
	MCPs    []string `json:"mcps"`
	Skills  []string `json:"skills"`
	Problem string   `json:"problem"`
	// Danger reports that Command carries a known permission bypass. It
	// replaces the posture enum's self-labelling: `full` used to name itself,
	// a free-form command has to be read
	// (ADR the-workspace-command-is-a-template).
	Danger bool `json:"danger"`
	// Notice mirrors SessionView.Notice's MCP explanation, shown on the
	// workspace row itself: an agent whose wiring cannot bound its tool set to
	// what the workspace declares says so before any session is even started
	// (spec §7.2, ADR a-workspace-declares-its-own-authority).
	Notice string `json:"notice"`
	// Schedules rides the workspace view because the editor that writes it is
	// the workspace editor, and the sidebar names the next run.
	Schedules []ScheduleView `json:"schedules"`
}

// SessionView is one row of a workspace's session list.
type SessionView struct {
	ID           int64  `json:"id"`
	Workspace    string `json:"workspace"`
	Name         string `json:"name"`
	Agent        string `json:"agent"`
	LastOpenedAt int64  `json:"lastOpenedAt"`
	// Slug is the tmux session name (agentws-<terminal id>) this session is addressed by
	// whether or not it is running, so a caller can key a row, a route or an
	// attach pool on it without deriving the name itself. TerminalID, not this,
	// is what reports liveness.
	Slug string `json:"slug"`
	// TerminalID is the tmux session name (agentws-<terminal id>) a live session rides,
	// addressed on the same tmux stream terminal mode uses (ADR terminal-transport); empty
	// when nothing is running.
	TerminalID string `json:"terminalId"`
	// WindowID is TerminalID's active tmux window at the moment this session
	// was attached -- the tmux wire frames output and input by window id,
	// unlike the flat ptyterm wire sessions rode before. It is set only by a
	// call that attaches (StartSession, ResumeSession); a listing read
	// (Sessions, Open) leaves it empty even for a live session, since nothing
	// there attaches.
	WindowID string `json:"windowId"`
	// PaneID is WindowID's active pane at attach and frames client input.
	PaneID string `json:"paneId"`
	// Cols and Rows are tmux's own size for that window at attach — whichever
	// attached client tmux's window-size option picked, not necessarily the
	// caller's cols/rows vote. The pane must open its grid at this size or the
	// TUI's cursor-addressed output tears; 0 means tmux has not reported one.
	// Set only by a call that attaches, like WindowID.
	Cols int `json:"cols"`
	Rows int `json:"rows"`
	// ResumeAttempted is false when this launch could not even try to resume —
	// the agent has no resume form, or it provably persisted nothing under the
	// session's id to resume. It is deliberately not named Resumed: Hive sees
	// only pane content, so a pruned agent history would report a successful
	// resume while the agent prints its own error in the pane, which is the
	// "pretending it was" spec §6.2 forbids.
	ResumeAttempted bool `json:"resumeAttempted"`
	// Notice carries a fresh-launch, unbounded-MCP or missing-MCP explanation.
	Notice string `json:"notice"`
	// ExitedEarly is what the scheduler reads instead of Notice, so the reason
	// it treats a launch as failed does not depend on a notice's wording.
	ExitedEarly bool `json:"exitedEarly"`
	// ScheduleID is a column on the session record rather than a lookup in the
	// run history: session ids are reused after a delete and the history is
	// pruned, so a derivation would mislabel or lose it.
	ScheduleID string `json:"scheduleId"`
}

// SessionActivityItem is one live session's detected activity, keyed by
// session id so the caller does not have to re-derive a tmux session name.
type SessionActivityItem struct {
	ID int64 `json:"id"`
	// Status is one of terminal.Status's simplified values: ready, active, or
	// approval -- approval is the highest-urgency state.
	Status string `json:"status"`
}

// StartSession is one new session's launch.
type StartSession struct {
	Workspace string
	Name      string
	Cols      int
	Rows      int
	// Prompt empty produces exactly the interactive launch line.
	Prompt string
	// Detached skips the control-client attach, so the session has no
	// TerminalID or WindowID. A scheduled launch has no pane to render into.
	Detached   bool
	ScheduleID string
}

// AgentSessionEndPath lives here so the URL a launch writes into the agent's
// environment and the route httpapi serves cannot drift apart.
const AgentSessionEndPath = "/api/sessions/end"

// defaultEndDelay exists because the end request arrives from inside the
// agent's own tool call; killing the pane before that call returns would cut
// the tool result out of the transcript.
const defaultEndDelay = 10 * time.Second

type SessionEnding struct {
	Session SessionView
	EndsAt  time.Time
}

// StartScheduledSession has no cols/rows because it is always detached.
// Prompt is the schedule's own template, rendered; the launch frames it.
type StartScheduledSession struct {
	Workspace    string
	ScheduleID   string
	ScheduleName string
	Name         string
	Prompt       string
}

// OpenResult is what opening a workspace reports back to the UI.
type OpenResult struct {
	Workspace   WorkspaceView
	Sessions    []SessionView
	MissingMCPs []string
	// MissingPackages reports every name the workspace enables that
	// skills.yml does not define, each carrying why it did not resolve.
	MissingPackages []MissingPackageItem
}

// MissingPackageItem is one enabled name skills.yml does not define. Skill
// separates the two causes that produce an identical report: a name that is
// actually a skill (a manifest predating packages) from one that matches
// nothing at all (a typo, or a package since deleted). SelectedBy names the
// packages that already select the skill — the fix, when there is one.
type MissingPackageItem struct {
	Name       string   `json:"name"`
	Skill      bool     `json:"skill"`
	SelectedBy []string `json:"selectedBy"`
}

// Available reports tmux availability -- the same axis terminal mode reports
// (TerminalsService.Available), since a session is now a tmux session rather
// than a ptyterm one.
func (s *AgentWorkspacesService) Available(ctx context.Context) error {
	return terminalError(s.terminals.Available(ctx),
		"agent workspaces need tmux 3.2 or newer. Hive searches PATH and the usual install prefixes; set paths.tmux in settings.yaml if yours is elsewhere.")
}

// RootProblem reports why the configured root could not be created or
// opened at startup, or "" when it is fine. Spec §14: a root on an unmounted
// volume or a signed-out iCloud Drive is reported rather than silently
// replaced with a second, empty root elsewhere -- the UI is what points at
// the setting to fix.
func (s *AgentWorkspacesService) RootProblem(context.Context) string {
	return s.rootProblem
}

// List returns every recognized workspace, valid or not — a broken manifest
// carries its last-good content plus Problem, rather than vanishing.
func (s *AgentWorkspacesService) List(ctx context.Context) ([]WorkspaceView, error) {
	statuses := s.store.Statuses()
	views := make([]WorkspaceView, 0, len(statuses))
	for _, st := range statuses {
		views = append(views, s.workspaceView(ctx, st))
	}
	return views, nil
}

func (s *AgentWorkspacesService) SessionLaunchWorkspaces(context.Context) []dispatch.SessionLaunchWorkspace {
	statuses := s.store.Statuses()
	workspaces := make([]dispatch.SessionLaunchWorkspace, 0, len(statuses))
	for _, st := range statuses {
		if !st.Valid {
			continue
		}
		workspaces = append(workspaces, dispatch.SessionLaunchWorkspace{
			Dir: st.Workspace.Dir, Name: st.Workspace.Name,
			SupportsPrompt: agentws.SupportsPrompt(st.Workspace.Command),
		})
	}
	return workspaces
}

// Open regenerates the workspace's disposable artifacts and returns its
// sessions. It is the only entry point that writes into a workspace.
func (s *AgentWorkspacesService) Open(ctx context.Context, dir string) (OpenResult, error) {
	regen, err := s.regenerate(ctx, dir)
	if err != nil {
		return OpenResult{}, err
	}

	records, err := s.sessions.List(ctx, dir)
	if err != nil {
		return OpenResult{}, Wrap(err, KindInternal, "listing sessions for workspace %q", dir)
	}
	sessions := s.sessionViews(ctx, records)

	view := s.workspaceView(ctx, regen.status)
	if len(regen.generated.Problems) > 0 {
		view.Problem = strings.Join(regen.generated.Problems, "; ")
	}

	return OpenResult{
		Workspace: view, Sessions: sessions,
		MissingMCPs: regen.generated.MissingMCPs, MissingPackages: regen.missingPackages,
	}, nil
}

type regeneration struct {
	status          agentws.WorkspaceStatus
	generated       agentws.Result
	missingPackages []MissingPackageItem
}

// regenerate holds the resolution chain Generate itself stays pure over:
// mcps: [...] resolves through the store's Catalogue, skills: [...] through
// skills.yml's packages (ADR skill-packages-are-the-unit-a-workspace-enables).
func (s *AgentWorkspacesService) regenerate(ctx context.Context, dir string) (regeneration, error) {
	if !validWorkspaceDir(dir) {
		return regeneration{}, Errorf(KindInvalid, "workspace %q is not a valid workspace directory name", dir)
	}
	st, ok := s.store.Status(dir)
	if !ok {
		return regeneration{}, Errorf(KindNotFound, "workspace %q not found", dir)
	}
	if !st.Valid {
		return regeneration{}, Wrap(st.Err, KindInvalid, "workspace %q", dir)
	}
	ws := st.Workspace

	rendered, missingPackages, err := s.resolveSkills(ctx, ws)
	if err != nil {
		return regeneration{}, err
	}

	generated, err := agentws.Generate(agentws.GenerateInput{
		Dir:       filepath.Join(s.store.Root(), dir),
		Workspace: ws,
		Servers:   s.resolveServers(ctx, ws),
		Skills:    rendered,
	})
	if err != nil {
		return regeneration{}, Wrap(err, KindInternal, "generating workspace %q", dir)
	}
	return regeneration{status: st, generated: generated, missingPackages: missingPackages}, nil
}

// Sessions lists a workspace's session rows without regenerating its
// disposable artifacts — the read the Agents area uses to refresh a session
// list after starting, resuming, closing, or deleting one, without paying
// Open's generation cost.
func (s *AgentWorkspacesService) Sessions(ctx context.Context, dir string) ([]SessionView, error) {
	if !validWorkspaceDir(dir) {
		return nil, Errorf(KindInvalid, "workspace %q is not a valid workspace directory name", dir)
	}
	records, err := s.sessions.List(ctx, dir)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing sessions for workspace %q", dir)
	}
	return s.sessionViews(ctx, records), nil
}

// SessionActivity captures each live session's tmux pane and classifies it
// with classifyAgentScreen: capture-pane through Hive's assessment
// engine, the same input SessionStatuses/FetchBatch use for hive's own
// sessions (hc-ou4o02zx). An
// empty dir spans every workspace — the sidebar shows every session at once,
// so its indicators poll the whole set the way the Code view's do; the work
// is bounded by maxConcurrentAgentSessions either way. A session with no
// live tmux session is omitted rather than reported dead -- a row's
// TerminalID already carries that.
func (s *AgentWorkspacesService) SessionActivity(ctx context.Context, dir string) ([]SessionActivityItem, error) {
	var records []stores.AgentSession
	var err error
	if dir == "" {
		records, err = s.sessions.ListAll(ctx)
		if err != nil {
			return nil, Wrap(err, KindInternal, "listing all sessions")
		}
	} else {
		if !validWorkspaceDir(dir) {
			return nil, Errorf(KindInvalid, "workspace %q is not a valid workspace directory name", dir)
		}
		records, err = s.sessions.List(ctx, dir)
		if err != nil {
			return nil, Wrap(err, KindInternal, "listing sessions for workspace %q", dir)
		}
	}
	items := make([]SessionActivityItem, 0, len(records))
	for _, rec := range records {
		screen, err := s.terminals.CapturePane(ctx, sessionName(rec))
		if err != nil {
			// Not running, or a transient tmux error -- omitted rather than
			// reported, the same tolerance sessionView extends to liveness.
			continue
		}
		items = append(items, SessionActivityItem{ID: rec.ID, Status: string(classifyAgentScreen(rec.Agent, screen))})
	}
	return items, nil
}

// LaunchWorkspaceSession refreshes generated files before starting the
// detached chat.
func (s *AgentWorkspacesService) LaunchWorkspaceSession(ctx context.Context, req dispatch.LaunchWorkspaceSessionRequest) (dispatch.SessionExecutionOutcome, error) {
	regen, err := s.regenerate(ctx, req.Workspace)
	if err != nil {
		return dispatch.SessionExecutionOutcome{}, err
	}
	view, err := s.startSession(ctx, regen.status.Workspace, StartSession{
		Workspace: req.Workspace,
		Name:      req.Name,
		Prompt:    req.Prompt,
		Detached:  true,
	})
	if err != nil {
		return dispatch.SessionExecutionOutcome{}, err
	}
	if view.ExitedEarly {
		_ = s.sessions.Delete(ctx, view.ID)
		return dispatch.SessionExecutionOutcome{}, Errorf(KindInternal, "%s", view.Notice)
	}
	// Logged, not returned: the chat exists, and a failed launch would invite a
	// retry that opens a second one.
	for _, origin := range req.Origins {
		if err := s.itemLinks.LinkChat(ctx, view.ID, origin); err != nil {
			s.logger.Warn().Ctx(ctx).Err(err).Int64("chat_id", view.ID).Str("external_id", origin.ExternalID).Msg("linking chat to an inbox item")
		}
	}
	return dispatch.SessionExecutionOutcome{
		ID: strconv.FormatInt(view.ID, 10), Name: view.Name, Slug: view.Slug,
	}, nil
}

func (s *AgentWorkspacesService) StartSession(ctx context.Context, req StartSession) (SessionView, error) {
	if !validWorkspaceDir(req.Workspace) {
		return SessionView{}, Errorf(KindInvalid, "workspace %q is not a valid workspace directory name", req.Workspace)
	}
	st, ok := s.store.Status(req.Workspace)
	if !ok || !st.Valid {
		return SessionView{}, Errorf(KindNotFound, "workspace %q not found", req.Workspace)
	}
	return s.startSession(ctx, st.Workspace, req)
}

func (s *AgentWorkspacesService) startSession(ctx context.Context, ws agentws.Workspace, req StartSession) (SessionView, error) {
	if strings.TrimSpace(req.Prompt) != "" && !agentws.SupportsPrompt(ws.Command) {
		return SessionView{}, promptlessCommandError("start a chat with an opening prompt")
	}
	workspaceDir := filepath.Join(s.store.Root(), ws.Dir)
	agentSessionID := uuid.NewString()
	line, err := agentws.Resolve(resolvedFor(ws, workspaceDir), agentSessionID, false, req.Prompt)
	if err != nil {
		return SessionView{}, agentLaunchError(err, ws.Dir)
	}

	rec, err := s.sessions.Create(ctx, stores.AgentSessionCreate{
		Workspace: ws.Dir, Name: req.Name, Agent: ws.Agent(), AgentSessionID: agentSessionID,
		ScheduleID: req.ScheduleID,
		// The token is minted once and reused by every resume, so the
		// process's environment is the same across launches.
		EndToken: uuid.NewString(),
	})
	if err != nil {
		return SessionView{}, Wrap(err, KindInternal, "creating session %q", req.Name)
	}

	return s.launchTerminal(ctx, rec, terminalLaunch{
		dir: workspaceDir, line: line, cols: req.Cols, rows: req.Rows,
		resumeAttempted: true, detached: req.Detached,
	})
}

// StartScheduledSession regenerates the workspace first, as Open does for the
// UI's launch: otherwise a scheduled run would drive an agent whose .mcp.json
// and skills are stale. The run history keeps the unframed prompt.
func (s *AgentWorkspacesService) StartScheduledSession(ctx context.Context, req StartScheduledSession) (SessionView, error) {
	regen, err := s.regenerate(ctx, req.Workspace)
	if err != nil {
		return SessionView{}, err
	}
	prompt, err := prompts.ScheduledRun(prompts.ScheduledRunData{
		ScheduleName: req.ScheduleName, WorkspaceName: regen.status.Workspace.Name,
		Prompt: req.Prompt, CanEnd: s.sessionEndURL(ctx) != "",
	})
	if err != nil {
		return SessionView{}, Wrap(err, KindInternal, "framing the scheduled prompt")
	}
	return s.startSession(ctx, regen.status.Workspace, StartSession{
		Workspace: req.Workspace, Name: req.Name, Prompt: prompt, Detached: true,
		ScheduleID: req.ScheduleID,
	})
}

const firstRunChatName = "Getting started"

// StartFirstRunChat opens the seeded Hive workspace on the interview that
// ends first run. The workspace is regenerated first, as a scheduled launch
// is: on a fresh install nothing has opened it yet, so its .mcp.json and
// skills do not exist until this does. The launch is detached because the
// onboarding screen has no pane; the route it hands off to attaches.
func (s *AgentWorkspacesService) StartFirstRunChat(ctx context.Context) (SessionView, error) {
	if err := s.ensureHiveWorkspace(); err != nil {
		return SessionView{}, err
	}
	if _, err := s.regenerate(ctx, agentws.HiveWorkspaceDir); err != nil {
		return SessionView{}, err
	}
	prompt, err := prompts.FirstRun(prompts.FirstRunData{DefaultProfile: DefaultProfileName})
	if err != nil {
		return SessionView{}, Wrap(err, KindInternal, "rendering the first-run prompt")
	}
	return s.StartSession(ctx, StartSession{
		Workspace: agentws.HiveWorkspaceDir, Name: firstRunChatName, Prompt: prompt, Detached: true,
	})
}

// Startup seeds only a newly created root, so first run repairs a missing
// Hive workspace. An existing workspace with a broken manifest is left alone;
// regenerate reports it.
func (s *AgentWorkspacesService) ensureHiveWorkspace() error {
	if _, ok := s.store.Status(agentws.HiveWorkspaceDir); ok {
		return nil
	}
	if err := agentws.SeedHiveWorkspace(s.store.Root()); err != nil {
		return Wrap(err, KindInternal, "seeding the Hive workspace")
	}
	if err := s.store.Reload(); err != nil {
		return Wrap(err, KindInternal, "reloading agent workspaces")
	}
	return nil
}

func (s *AgentWorkspacesService) sessionEndURL(ctx context.Context) string {
	base := s.mcpBase.MCPBaseURL(ctx)
	if base == "" {
		return ""
	}
	return strings.TrimSuffix(base, "/") + AgentSessionEndPath
}

// EndOwnSession answers as soon as the token is matched and deletes the chat,
// record included, after the delay in the background: a schedule that runs
// hourly must not leave a row per run in the sidebar.
func (s *AgentWorkspacesService) EndOwnSession(ctx context.Context, token string) (SessionEnding, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return SessionEnding{}, Errorf(KindUnauthenticated, "a session token is required")
	}
	rec, err := s.sessions.GetByEndToken(ctx, token)
	if stores.IsNotFound(err) {
		return SessionEnding{}, Errorf(KindUnauthenticated, "no session holds that token")
	}
	if err != nil {
		return SessionEnding{}, Wrap(err, KindInternal, "looking up the session a token names")
	}

	delay := s.endDelay.SessionEndDelay(ctx)
	if delay <= 0 {
		delay = defaultEndDelay
	}
	view := s.sessionViews(ctx, []stores.AgentSession{rec})[0]
	go s.endAfter(context.WithoutCancel(ctx), delay, view)
	return SessionEnding{Session: view, EndsAt: time.Now().Add(delay)}, nil
}

// A scheduled chat that ended itself is what the Chats area is showing as
// live, so the same wake-up a schedule write sends makes it re-read the list.
func (s *AgentWorkspacesService) endAfter(ctx context.Context, delay time.Duration, view SessionView) {
	time.Sleep(delay)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	switch err := s.DeleteSession(ctx, view.ID); {
	case KindOf(err) == KindNotFound:
		// Deleted by hand in the meantime; there is nothing left to do.
		return
	case err != nil:
		s.logger.Warn().Err(err).Int64("session", view.ID).Msg("a chat asked to end itself but could not be deleted")
		return
	}
	s.logger.Info().Int64("session", view.ID).Msg("a chat ended itself")
	if view.ScheduleID != "" {
		s.events.Publish(ctx, events.SchedulesUpdated{Workspace: view.Workspace})
	}
}

func (s *AgentWorkspacesService) SessionLive(ctx context.Context, id int64) (bool, error) {
	rec, err := s.getSession(ctx, id)
	if KindOf(err) == KindNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	alive, err := s.terminals.HasSession(ctx, sessionName(rec))
	if err != nil {
		return false, terminalError(err, "checking session %d", id)
	}
	return alive, nil
}

// ResumeSession reattaches a session's live tmux session if it still has one
// -- the persistence win: it works identically for codex, which has no
// resume form of its own, because the process itself never stopped -- or
// relaunches it, resuming the agent's own conversation when it has a resume
// form and saying so when it does not (spec §6.2). A session the agent never
// persisted a conversation for (closed before its first message) also
// relaunches fresh, silently, instead of dying on the agent's own
// unknown-session error.
func (s *AgentWorkspacesService) getSession(ctx context.Context, id int64) (stores.AgentSession, error) {
	rec, err := s.sessions.Get(ctx, id)
	if stores.IsNotFound(err) {
		return stores.AgentSession{}, Errorf(KindNotFound, "session %d not found", id)
	}
	return rec, Wrap(err, KindInternal, "loading session %d", id)
}

func (s *AgentWorkspacesService) ResumeSession(ctx context.Context, id int64, cols, rows int) (SessionView, error) {
	rec, err := s.getSession(ctx, id)
	if err != nil {
		return SessionView{}, err
	}

	name := sessionName(rec)
	alive, err := s.terminals.HasSession(ctx, name)
	if err != nil {
		return SessionView{}, terminalError(err, "checking session %q", rec.Name)
	}
	if alive {
		window, err := s.attach(ctx, name, cols, rows)
		if err != nil {
			return SessionView{}, err
		}
		if err := s.sessions.Touch(ctx, rec.ID, time.Now().UnixMilli()); err != nil {
			return SessionView{}, Wrap(err, KindInternal, "recording session %q as opened", rec.Name)
		}
		return SessionView{
			ID: rec.ID, Workspace: rec.Workspace, Name: rec.Name, Agent: rec.Agent,
			LastOpenedAt: rec.LastOpenedAt, Slug: name, TerminalID: name, WindowID: window.ID, PaneID: window.ActivePane,
			Cols: window.Width, Rows: window.Height,
			ResumeAttempted: true,
		}, nil
	}

	st, ok := s.store.Status(rec.Workspace)
	if !ok || !st.Valid {
		return SessionView{}, Errorf(KindNotFound, "workspace %q not found", rec.Workspace)
	}
	ws := st.Workspace

	resumeAttempted := agentws.SupportsResume(ws.Command)
	var resumeNotice string
	switch {
	case !resumeAttempted:
		resumeNotice = "the previous conversation could not be resumed; this is a fresh session"
	case !agentws.HasConversation(ws.Agent(), rec.AgentSessionID):
		// The agent persisted nothing under this id — the session ended before
		// its first message — so its resume form would die in the pane ("No
		// conversation found with session ID"). A fresh launch IS the
		// continuation of an empty conversation, so no notice either.
		resumeAttempted = false
	}
	// A fresh relaunch mints a fresh id rather than reusing the record's: if
	// the conversation probe were ever wrong, relaunching under an id the
	// agent already holds would wedge every future resume on "session id
	// already in use", while abandoning it merely orphans a recoverable file.
	sessionID := rec.AgentSessionID
	if !resumeAttempted {
		sessionID = uuid.NewString()
	}

	workspaceDir := filepath.Join(s.store.Root(), rec.Workspace)
	line, err := agentws.Resolve(resolvedFor(ws, workspaceDir), sessionID, resumeAttempted, "")
	if err != nil {
		return SessionView{}, agentLaunchError(err, ws.Dir)
	}

	if sessionID != rec.AgentSessionID {
		if err := s.sessions.SetAgentID(ctx, rec.ID, sessionID); err != nil {
			return SessionView{}, Wrap(err, KindInternal, "recording session %q", rec.Name)
		}
		rec.AgentSessionID = sessionID
	}

	return s.launchTerminal(ctx, rec, terminalLaunch{
		dir: workspaceDir, line: line, cols: cols, rows: rows,
		resumeAttempted: resumeAttempted, resumeNotice: resumeNotice,
	})
}

// CloseSession ends a session's live tmux session and reports whether there
// was one running. The record is untouched, so it still lists afterward.
func (s *AgentWorkspacesService) CloseSession(ctx context.Context, id int64) (bool, error) {
	rec, err := s.getSession(ctx, id)
	if err != nil {
		return false, err
	}
	closed, err := s.terminals.KillSession(ctx, sessionName(rec))
	if err != nil {
		return false, terminalError(err, "closing session %q", rec.Name)
	}
	return closed, nil
}

// RenameSession sets a session's display name. Purely a record edit: the
// tmux session name comes from an immutable terminal id, so a live terminal keeps running
// under the same name.
func (s *AgentWorkspacesService) RenameSession(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return Errorf(KindInvalid, "a session needs a name")
	}
	if _, err := s.getSession(ctx, id); err != nil {
		return err
	}
	return Wrap(s.sessions.Rename(ctx, id, name), KindInternal, "renaming session %d", id)
}

// DeleteSession ends any live tmux session, then removes the record. The
// session is killed first: its name lives on the record, so deleting the
// record around a live one would orphan a running agent no UI could
// address again until it happened to be found by name.
func (s *AgentWorkspacesService) DeleteSession(ctx context.Context, id int64) error {
	rec, err := s.getSession(ctx, id)
	if err != nil {
		return err
	}
	if _, err := s.terminals.KillSession(ctx, sessionName(rec)); err != nil {
		return terminalError(err, "closing session %q", rec.Name)
	}
	if err := s.sessions.Delete(ctx, id); err != nil {
		return Wrap(err, KindInternal, "deleting session %q", rec.Name)
	}
	return nil
}

// DeleteWorkspace ends every live tmux session the workspace's sessions hold,
// deletes the workspace directory, then removes the session records
// (ADR deleting-a-workspace-deletes-its-directory).
//
// That order is what a failure leaves behind. Terminals go first because the
// directory under their working directory is about to disappear. The
// directory goes before the records so a delete that cannot finish — a
// permission, an unreachable root — still has the chat history to come back
// to, rather than reporting a failure that already threw it away.
func (s *AgentWorkspacesService) DeleteWorkspace(ctx context.Context, dir string) error {
	if _, err := s.knownWorkspaceDir(dir); err != nil {
		return err
	}
	records, err := s.sessions.List(ctx, dir)
	if err != nil {
		return Wrap(err, KindInternal, "listing sessions for workspace %q", dir)
	}
	for _, rec := range records {
		if _, err := s.terminals.KillSession(ctx, sessionName(rec)); err != nil {
			return terminalError(err, "closing session %q", rec.Name)
		}
	}
	if err := agentws.RemoveWorkspace(s.store.Root(), dir); err != nil {
		return Wrap(err, KindInternal, "deleting workspace %q", dir)
	}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.sessions.DeleteByWorkspace(ctx, dir); err != nil {
			return Wrap(err, KindInternal, "deleting sessions for workspace %q", dir)
		}
		return Wrap(s.schedules.DeleteByWorkspace(ctx, dir), KindInternal, "deleting schedule state for workspace %q", dir)
	})
	if err != nil {
		return err
	}
	if err := s.store.Reload(); err != nil {
		return Wrap(err, KindInternal, "reloading workspaces")
	}
	return nil
}

// Presets lists the starter command templates the editor offers: the ones
// this build ships, plus one per agent profile in hive's own config. A hive
// profile contributes its command AND its flags, unlike the old seam that
// stripped them — the flags are the reason to pick that profile, and here
// they land in a manifest field the user reviews and can edit rather than
// being spliced in at every launch (ADR the-workspace-command-is-a-template).
//
// Nothing here gates a launch. A user who ignores every preset and types
// their own command gets exactly that.
func (s *AgentWorkspacesService) Presets(context.Context) []agentws.Preset {
	presets := agentws.BuiltinPresets()
	shipped := make(map[string]bool, len(presets))
	for _, p := range presets {
		shipped[p.Agent] = true
	}
	for agent, command := range s.profileCommands() {
		if shipped[agent] {
			// A shipped agent already has postures spelled out; hive's single
			// profile line would only duplicate one of them, less precisely.
			continue
		}
		// A profile is a command word plus flags and carries no wiring, so a
		// profile running a CLI this build does know (a model-pinned "fable"
		// profile running claude) gets that CLI's wiring appended — otherwise
		// the preset would launch with no MCP servers and a session Hive
		// cannot address.
		command += agentws.WiringTailFor(command)
		presets = append(presets, agentws.Preset{
			ID: "hive-" + agent,
			// A model-pinned "fable" profile keeps its own name and still
			// marks itself as the claude it runs.
			Agent:   agentws.AgentFor(command),
			Label:   agent,
			Command: command,
			Danger:  agentws.CommandIsDangerous(command),
			Source:  agentws.PresetSourceHive,
		})
	}
	agentws.SortPresets(presets)
	return presets
}

// WorkspaceEdit names the manifest fields the in-app editor writes. Anything
// else the manifest says is untouched — WriteManifest edits the document in
// place, so comments and keys the editor does not own stay the user's.
type WorkspaceEdit struct {
	Dir     string
	Name    string
	Command string
	MCPs    []string
	Skills  []string
	// Schedules is the whole list: a write reconciles the manifest to exactly
	// this, so an entry the editor dropped is deleted by the same call.
	Schedules []ScheduleEdit
}

type ScheduleEdit struct {
	ID       string
	Name     string
	Cron     string
	Prompt   string
	Disabled bool
	OnMissed string
}

func (e ScheduleEdit) spec() schedule.Spec {
	return schedule.Spec{
		ID:       strings.TrimSpace(e.ID),
		Name:     strings.TrimSpace(e.Name),
		Cron:     strings.TrimSpace(e.Cron),
		Prompt:   e.Prompt,
		Disabled: e.Disabled,
		OnMissed: schedule.OnMissed(strings.TrimSpace(e.OnMissed)),
	}
}

// A nil SchedulePatch field keeps its stored value, so an agent told to
// re-time a paused schedule does not re-enable it.
type SchedulePatch struct {
	ID       string
	Name     *string
	Cron     *string
	Prompt   *string
	Disabled *bool
	OnMissed *string
}

func (p SchedulePatch) apply(base schedule.Spec) schedule.Spec {
	base.ID = strings.TrimSpace(p.ID)
	if p.Name != nil {
		base.Name = strings.TrimSpace(*p.Name)
	}
	if p.Cron != nil {
		base.Cron = strings.TrimSpace(*p.Cron)
	}
	if p.Prompt != nil {
		base.Prompt = *p.Prompt
	}
	if p.Disabled != nil {
		base.Disabled = *p.Disabled
	}
	if p.OnMissed != nil {
		base.OnMissed = schedule.OnMissed(strings.TrimSpace(*p.OnMissed))
	}
	return base
}

func (e WorkspaceEdit) manifest() agentws.ManifestEdit {
	return agentws.ManifestEdit{
		Name: strings.TrimSpace(e.Name), Command: strings.TrimSpace(e.Command),
		MCPs: e.MCPs, Skills: e.Skills, Schedules: e.specs(),
	}
}

func (e WorkspaceEdit) specs() []schedule.Spec {
	specs := make([]schedule.Spec, 0, len(e.Schedules))
	for _, edit := range e.Schedules {
		specs = append(specs, edit.spec())
	}
	return specs
}

func (s *AgentWorkspacesService) validateEdit(req WorkspaceEdit) error {
	if !validWorkspaceDir(req.Dir) {
		return Errorf(KindInvalid, "workspace %q is not a valid workspace directory name", req.Dir)
	}
	if strings.TrimSpace(req.Name) == "" {
		return Errorf(KindInvalid, "a workspace needs a name")
	}
	command := strings.TrimSpace(req.Command)
	if command == "" {
		return Errorf(KindInvalid, "a workspace needs a command")
	}
	// The template is checked here rather than at launch, so a command that
	// cannot render is refused by the editor that wrote it instead of by the
	// session someone starts a day later.
	if err := agentws.ValidateCommand(command); err != nil {
		return Wrap(err, KindInvalid, "the command template is not valid")
	}
	for _, list := range []struct {
		label  string
		values []string
	}{
		{"mcp", req.MCPs},
		{"skill package", req.Skills},
	} {
		seen := make(map[string]bool, len(list.values))
		for _, id := range list.values {
			if id == "" {
				return Errorf(KindInvalid, "%ss entries must not be empty", list.label)
			}
			if seen[id] {
				return Errorf(KindInvalid, "duplicate %s %q", list.label, id)
			}
			seen[id] = true
		}
	}
	if len(req.Schedules) > 0 && !agentws.SupportsPrompt(command) {
		return promptlessCommandError("run schedules")
	}
	// The id shape, cron and prompt rules are the spec's own; repeating them
	// here would be a second place for them to drift.
	seen := make(map[string]bool, len(req.Schedules))
	for _, spec := range req.specs() {
		if err := spec.Validate(); err != nil {
			return Errorf(KindInvalid, "%s", err)
		}
		if seen[spec.ID] {
			return Errorf(KindInvalid, "duplicate schedule %q", spec.ID)
		}
		seen[spec.ID] = true
	}
	return nil
}

// CreateWorkspace makes a directory under the root with a fresh manifest and
// an AGENTS.md scaffold, and returns its view.
func (s *AgentWorkspacesService) CreateWorkspace(ctx context.Context, req WorkspaceEdit) (WorkspaceView, error) {
	if err := s.validateEdit(req); err != nil {
		return WorkspaceView{}, err
	}
	if err := agentws.CreateWorkspace(s.store.Root(), req.Dir, req.manifest()); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return WorkspaceView{}, Errorf(KindConflict, "workspace %q already exists", req.Dir)
		}
		return WorkspaceView{}, Wrap(err, KindInternal, "creating workspace %q", req.Dir)
	}
	return s.savedView(ctx, req.Dir)
}

// UpdateWorkspace rewrites the editable fields of an existing workspace's
// manifest in place — comments and keys the editor does not own survive.
func (s *AgentWorkspacesService) UpdateWorkspace(ctx context.Context, req WorkspaceEdit) (WorkspaceView, error) {
	if err := s.validateEdit(req); err != nil {
		return WorkspaceView{}, err
	}
	if _, err := s.editableWorkspace(req.Dir); err != nil {
		return WorkspaceView{}, err
	}
	if err := agentws.WriteManifest(s.store.Root(), req.Dir, req.manifest()); err != nil {
		return WorkspaceView{}, Wrap(err, KindInvalid, "updating workspace %q", req.Dir)
	}
	return s.savedView(ctx, req.Dir)
}

// PutSchedule upserts one schedules: entry by id and leaves the rest of the
// manifest alone.
func (s *AgentWorkspacesService) PutSchedule(ctx context.Context, dir string, patch SchedulePatch) (ScheduleView, error) {
	st, err := s.editableWorkspace(dir)
	if err != nil {
		return ScheduleView{}, err
	}
	// Refused before the write rather than found by the loader after it: a
	// schedule saved into a workspace whose command drops the prompt would
	// turn the whole workspace into a problem the agent never asked for.
	if !agentws.SupportsPrompt(st.Workspace.Command) {
		return ScheduleView{}, promptlessCommandError("run schedules")
	}

	id := strings.TrimSpace(patch.ID)
	specs := append([]schedule.Spec(nil), st.Workspace.Schedules...)
	at := -1
	for i := range specs {
		if specs[i].ID == id {
			at = i
		}
	}
	base := schedule.Spec{ID: id}
	if at >= 0 {
		base = specs[at]
	} else if id != "" && (patch.Cron == nil || patch.Prompt == nil) {
		return ScheduleView{}, Errorf(KindInvalid, "schedule %q does not exist in workspace %q yet; a new schedule needs a cron and a prompt", id, dir)
	}
	spec := patch.apply(base)
	if err := spec.Validate(); err != nil {
		return ScheduleView{}, Errorf(KindInvalid, "%s", err)
	}

	if at >= 0 {
		specs[at] = spec
	} else {
		specs = append(specs, spec)
	}
	if err := agentws.WriteSchedules(s.store.Root(), dir, specs); err != nil {
		return ScheduleView{}, Wrap(err, KindInvalid, "writing schedule %q in workspace %q", spec.ID, dir)
	}

	view, err := s.savedView(ctx, dir)
	if err != nil {
		return ScheduleView{}, err
	}
	for _, row := range view.Schedules {
		if row.ID == spec.ID {
			return row, nil
		}
	}
	return ScheduleView{}, Errorf(KindInternal, "schedule %q vanished after writing it", spec.ID)
}

// RemoveSchedule leaves the run history, which outlives its entry on purpose;
// the cursor is pruned by the scheduler's next pass.
func (s *AgentWorkspacesService) RemoveSchedule(ctx context.Context, dir, id string) error {
	st, err := s.editableWorkspace(dir)
	if err != nil {
		return err
	}
	kept := make([]schedule.Spec, 0, len(st.Workspace.Schedules))
	found := false
	for _, spec := range st.Workspace.Schedules {
		if spec.ID == id {
			found = true
			continue
		}
		kept = append(kept, spec)
	}
	if !found {
		return Errorf(KindNotFound, "schedule %q not found in workspace %q", id, dir)
	}
	if err := agentws.WriteSchedules(s.store.Root(), dir, kept); err != nil {
		return Wrap(err, KindInvalid, "removing schedule %q from workspace %q", id, dir)
	}
	_, err = s.savedView(ctx, dir)
	return err
}

func promptlessCommandError(purpose string) error {
	return Errorf(KindInvalid, "the workspace command does not pass a prompt to the agent, so it cannot %s; pick a shipped command or add%s to it", purpose, agentws.PromptTail)
}

// editableWorkspace refuses a manifest that does not parse: on the first load
// of a run there is no last-good snapshot behind a broken file, so a write
// over one would reconcile every list to nothing.
func (s *AgentWorkspacesService) editableWorkspace(dir string) (agentws.WorkspaceStatus, error) {
	if !validWorkspaceDir(dir) {
		return agentws.WorkspaceStatus{}, Errorf(KindInvalid, "workspace %q is not a valid workspace directory name", dir)
	}
	st, ok := s.store.Status(dir)
	if !ok {
		return agentws.WorkspaceStatus{}, Errorf(KindNotFound, "workspace %q not found", dir)
	}
	if !st.Valid {
		return agentws.WorkspaceStatus{}, Errorf(KindInvalid,
			"agent-workspace.yaml has a problem (%s); fix the file before editing it here", st.Err)
	}
	return st, nil
}

// MCPCatalogueItem is one row of the merged MCP catalogue as the UI shows it:
// a shipped entry, a user mcps.yaml entry, or a user entry shadowing a
// shipped one.
type MCPCatalogueItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Shipped     bool   `json:"shipped"`
	Stability   string `json:"stability"`
	Shadows     string `json:"shadows"`
	Transport   string `json:"transport"`
	// Command is the resolved invocation — the command line for stdio, the
	// URL for http/sse. Nothing is enabled whose command the user could not
	// read first (ADR a-workspace-declares-its-own-authority §5).
	Command string `json:"command"`
	Problem string `json:"problem"`
}

// MCPCatalogue lists the merged MCP catalogue — the choices a workspace
// editor offers for its mcps: list.
func (s *AgentWorkspacesService) MCPCatalogue(ctx context.Context) []MCPCatalogueItem {
	entries := s.catalogue(ctx)
	items := make([]MCPCatalogueItem, 0, len(entries))
	for _, e := range entries {
		title := e.Title
		if title == "" {
			title = e.ID
		}
		items = append(items, MCPCatalogueItem{
			ID: e.ID, Title: title, Description: e.Description,
			Shipped: e.Shipped, Stability: string(e.Stability), Shadows: e.Shadows,
			Transport: string(e.Server.Transport),
			Command:   resolvedCommandLine(e.Server),
			Problem:   e.Problem,
		})
	}
	return items
}

// SkillPackageItem is one row of the package catalogue as the UI shows it: a
// package from skills.yml and the skills it currently resolves to. Members
// are what the patterns select right now, not a stored list — adding a
// matching skill changes every workspace that enabled the package.
type SkillPackageItem struct {
	Name        string               `json:"name"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	Members     []SkillPackageMember `json:"members"`
}

// SkillPackageMember is one skill a package selects, with where it came from.
type SkillPackageMember struct {
	Slug    string `json:"slug"`
	Shipped bool   `json:"shipped"`
}

// SkillNameItem is one name in the skills name-space with the packages that
// select it. The editor needs the whole name-space, not just the packages:
// an enabled name it cannot find among the packages is either a skill (fix:
// enable a package in SelectedBy) or nothing at all (fix: a typo), and only
// the name-space tells those apart.
type SkillNameItem struct {
	Slug       string   `json:"slug"`
	Shipped    bool     `json:"shipped"`
	SelectedBy []string `json:"selectedBy"`
}

// SkillPackages lists the packages a workspace editor offers for its skills:
// list, the name-space they select over, and the problem skills.yml itself
// has, if any. A source that cannot be read contributes no names rather than
// failing the listing: an open is where that is reported, because that is the
// call whose result changes.
func (s *AgentWorkspacesService) SkillPackages(ctx context.Context) ([]SkillPackageItem, []SkillNameItem, string) {
	shipped, _ := s.shippedSkills(ctx)
	shared, _ := s.sharedSkills()

	library := s.store.SkillLibrary()
	problem := ""
	if !library.Valid && library.Err != nil {
		problem = library.Err.Error()
	}

	names := agentws.SkillNames(shipped, shared)
	entries := agentws.SkillPackageCatalogue(library.Library, names)
	items := make([]SkillPackageItem, 0, len(entries))
	for _, e := range entries {
		members := make([]SkillPackageMember, 0, len(e.Members))
		for _, m := range e.Members {
			members = append(members, SkillPackageMember{Slug: m.Slug, Shipped: m.Shipped})
		}
		items = append(items, SkillPackageItem{
			Name: e.Name, Title: e.Title, Description: e.Description, Members: members,
		})
	}

	catalogue := agentws.SkillNameCatalogue(library.Library, names)
	skillNames := make([]SkillNameItem, 0, len(catalogue))
	for _, e := range catalogue {
		skillNames = append(skillNames, SkillNameItem{Slug: e.Slug, Shipped: e.Shipped, SelectedBy: e.SelectedBy})
	}
	return items, skillNames, problem
}

// RevealSkillPackages opens skills.yml in the OS file manager's default
// handler — packages are defined by editing that file, which is the authoring
// act this release offers. A missing file is seeded first, so the action
// always lands somewhere real.
func (s *AgentWorkspacesService) RevealSkillPackages(context.Context) error {
	if err := agentws.SeedDefaultsIfMissing(s.store.Root()); err != nil {
		return Wrap(err, KindInternal, "seeding skills.yml")
	}
	path := agentws.SkillLibraryPath(s.store.Root())
	return Wrap(osopen.Open(path), KindInternal, "opening %s", path)
}

// RevealSharedSkills opens the shared skills directory in the OS file
// manager — where a skill a package can select is authored, since it is a
// SKILL.md a user writes. The directory is created if missing: startup seeds
// it, but a user who deleted it must still be able to open it and drop a
// skill back in.
func (s *AgentWorkspacesService) RevealSharedSkills(context.Context) error {
	dir := agentws.SharedSkillsDir(s.store.Root())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Wrap(err, KindInternal, "creating %s", dir)
	}
	return Wrap(osopen.Open(dir), KindInternal, "opening %s", dir)
}

// shippedSkills is the catalogue's shipped half: every listed prompt as an
// installable skill. It renders against an empty prompts.Input, so a prompt
// that needs caller-supplied context the workspace editor does not have (the
// keybindings catalog) is absent from the catalogue rather than offered and
// unresolvable.
func (s *AgentWorkspacesService) shippedSkills(ctx context.Context) ([]agentws.ShippedSkill, error) {
	if s.skills == nil {
		return nil, nil
	}
	return s.skills.ShippedSkills(ctx)
}

func (s *AgentWorkspacesService) sharedSkills() ([]agentws.SharedSkill, error) {
	return agentws.LoadSharedSkills(agentws.SharedSkillsDir(s.store.Root()))
}

// ImportMCPServers parses pasted MCP JSON (claude's mcpServers wrapper or a
// bare id-to-server map) into mcps.yaml and returns the added ids, sorted. An
// id already declared in the library is a conflict — the file is where an
// existing entry is changed — while shadowing a shipped id is allowed, the
// same rule mcps.yaml itself follows.
func (s *AgentWorkspacesService) ImportMCPServers(_ context.Context, raw string) ([]string, error) {
	servers, err := agentws.ParseMCPImport([]byte(raw))
	if err != nil {
		return nil, Wrap(err, KindInvalid, "importing MCP servers")
	}
	if err := agentws.AddLibraryServers(s.store.Root(), servers); err != nil {
		if errors.Is(err, agentws.ErrLibraryServerExists) {
			return nil, Wrap(err, KindConflict, "importing MCP servers")
		}
		return nil, Wrap(err, KindInternal, "importing MCP servers")
	}
	if err := s.store.Reload(); err != nil {
		return nil, Wrap(err, KindInternal, "reloading the MCP catalogue")
	}
	ids := make([]string, 0, len(servers))
	for id := range servers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// RemoveMCPServer deletes a user-declared server from mcps.yaml. A shipped
// entry cannot be removed — a workspace disables it by dropping the id from
// its own mcps: list instead.
func (s *AgentWorkspacesService) RemoveMCPServer(_ context.Context, id string) error {
	found, err := agentws.RemoveLibraryServer(s.store.Root(), id)
	if err != nil {
		return Wrap(err, KindInternal, "removing MCP server %q", id)
	}
	if !found {
		if _, shipped := mcpcatalog.All()[id]; shipped {
			return Errorf(KindInvalid, "%q is a shipped MCP server; remove it from the workspace's own list instead", id)
		}
		return Errorf(KindNotFound, "MCP server %q is not declared in mcps.yaml", id)
	}
	if err := s.store.Reload(); err != nil {
		return Wrap(err, KindInternal, "reloading the MCP catalogue")
	}
	return nil
}

// Editor reports the configured "open in editor" target: the command from
// settings and the display title the UI labels the action with. An empty
// command means none is configured.
func (s *AgentWorkspacesService) Editor(ctx context.Context) (command, title string) {
	command, err := s.editorCommand.Editor(ctx)
	if err != nil || command == "" {
		return "", ""
	}
	return command, editorTitle(command)
}

// OpenWorkspaceInEditor launches the configured editor on the workspace
// directory, detached — the editor outlives the request and Hive never waits
// on it.
func (s *AgentWorkspacesService) OpenWorkspaceInEditor(ctx context.Context, dir string) error {
	workspaceDir, err := s.knownWorkspaceDir(dir)
	if err != nil {
		return err
	}
	command, _ := s.Editor(ctx)
	return launchEditor(ctx, s.execEnv, command, workspaceDir)
}

// RevealWorkspace opens the workspace directory in the OS file manager.
func (s *AgentWorkspacesService) RevealWorkspace(_ context.Context, dir string) error {
	workspaceDir, err := s.knownWorkspaceDir(dir)
	if err != nil {
		return err
	}
	return Wrap(osopen.Open(workspaceDir), KindInternal, "opening %s", workspaceDir)
}

// knownWorkspaceDir resolves dir to its absolute path, refusing anything that
// is not a recognized workspace — the same guard checkAllowed gives
// SystemService's open/reveal, because launching a program on an arbitrary
// request-supplied path is not this API's to offer. A workspace with a broken
// manifest still resolves: opening it in an editor is exactly how it gets
// fixed.
func (s *AgentWorkspacesService) knownWorkspaceDir(dir string) (string, error) {
	if !validWorkspaceDir(dir) {
		return "", Errorf(KindInvalid, "workspace %q is not a valid workspace directory name", dir)
	}
	if _, ok := s.store.Status(dir); !ok {
		return "", Errorf(KindNotFound, "workspace %q not found", dir)
	}
	return filepath.Join(s.store.Root(), dir), nil
}

// resolvedCommandLine renders what an entry actually launches, for display: a
// shell-style command line for stdio, the URL for a remote server.
func resolvedCommandLine(server mcpcatalog.Server) string {
	if server.Transport == mcpcatalog.TransportStdio {
		return strings.Join(append([]string{server.Command}, server.Args...), " ")
	}
	return server.URL
}

// savedView re-reads the store after a manifest write and returns dir's fresh
// view, so the response reflects what actually landed on disk rather than what
// was asked for. The hook fires after the reload because the scheduler takes
// its specs from the same snapshot this read comes out of.
func (s *AgentWorkspacesService) savedView(ctx context.Context, dir string) (WorkspaceView, error) {
	if err := s.store.Reload(); err != nil {
		return WorkspaceView{}, Wrap(err, KindInternal, "reloading workspaces")
	}
	st, ok := s.store.Status(dir)
	if !ok {
		return WorkspaceView{}, Errorf(KindInternal, "workspace %q vanished after writing it", dir)
	}
	view := s.workspaceView(ctx, st)
	if s.OnSchedulesChanged != nil {
		s.OnSchedulesChanged(dir)
	}
	s.events.Publish(ctx, events.SchedulesUpdated{Workspace: dir})
	return view, nil
}

// ResizeSession votes a size for a session's attached control client — the
// same refresh-client vote the Code view's panes cast. tmux answers over the
// stream with a window 'layout-changed' event, which is what actually sets
// the pane's grid; see AgentsMode's resize wiring.
func (s *AgentWorkspacesService) ResizeSession(ctx context.Context, id int64, cols, rows int) error {
	rec, err := s.getSession(ctx, id)
	if err != nil {
		return err
	}
	client, ok := s.terminals.Client(sessionName(rec))
	if !ok {
		return Errorf(KindNotFound, "session %q has no attached terminal", rec.Name)
	}
	return terminalError(client.Resize(ctx, cols, rows), "resizing session %q", rec.Name)
}

type terminalLaunch struct {
	dir  string
	line string
	cols int
	rows int
	// resumeNotice is the caller's own words for a resume that could not
	// happen, empty when there is nothing to announce.
	resumeAttempted bool
	resumeNotice    string
	detached        bool
}

// launchTerminal creates rec's tmux session fresh and attaches to it, gives it
// earlyExitWindow to report that it already died, and assembles the notice
// the UI shows. Both StartSession and ResumeSession's relaunch branch always
// want a fresh session here — ResumeSession's still-alive branch attaches
// directly instead, without going through this method.
func (s *AgentWorkspacesService) launchTerminal(ctx context.Context, rec stores.AgentSession, opts terminalLaunch) (SessionView, error) {
	count, err := s.liveSessionCount(ctx)
	if err != nil {
		return SessionView{}, s.discardDetached(ctx, rec, opts, terminalError(err, "counting live agent sessions"))
	}
	if count >= maxConcurrentAgentSessions {
		return SessionView{}, s.discardDetached(ctx, rec, opts,
			Errorf(KindConflict, "too many agent sessions are running (%d max); close one first", maxConcurrentAgentSessions))
	}

	name := sessionName(rec)
	// The record id is the canvas tools' session argument; handing it to the
	// process at launch is what lets the agent name its own chat without
	// guessing (ADR canvases-are-named-files-in-the-workspace-folder-served-over-their-own-mcp-entry).
	// The token and the URL are what let it end its own chat
	// (ADR a-scheduled-chat-ends-itself-through-a-capability-token-its-launch-handed-it).
	env := []string{
		fmt.Sprintf("HIVE_AGENT_SESSION=%d", rec.ID),
		"HIVE_AGENT_WORKSPACE=" + opts.dir,
	}
	if rec.EndToken != "" {
		env = append(env, "HIVE_AGENT_SESSION_TOKEN="+rec.EndToken)
		if endURL := s.sessionEndURL(ctx); endURL != "" {
			env = append(env, "HIVE_AGENT_SESSION_END_URL="+endURL)
		}
	}
	if err := s.terminals.NewSession(ctx, name, opts.dir, opts.line, env); err != nil {
		return SessionView{}, s.discardDetached(ctx, rec, opts, terminalError(err, "launching session %q", rec.Name))
	}

	if err := s.sessions.Touch(ctx, rec.ID, time.Now().UnixMilli()); err != nil {
		return SessionView{}, Wrap(err, KindInternal, "recording session %q as opened", rec.Name)
	}

	view := SessionView{
		ID: rec.ID, Workspace: rec.Workspace, Name: rec.Name, Agent: rec.Agent,
		LastOpenedAt: rec.LastOpenedAt, Slug: name, ResumeAttempted: opts.resumeAttempted,
		ScheduleID: rec.ScheduleID,
	}

	if s.awaitEarlyExit(ctx, name) {
		view.ExitedEarly = true
		view.Notice = "the session exited immediately; check that the agent CLI is installed and on PATH"
		return view, nil
	}

	if !opts.detached {
		window, err := s.attach(ctx, name, opts.cols, opts.rows)
		if err != nil {
			return SessionView{}, err
		}
		view.TerminalID = name
		view.WindowID = window.ID
		view.PaneID = window.ActivePane
		view.Cols, view.Rows = window.Width, window.Height
	}

	var notices []string
	if opts.resumeNotice != "" {
		notices = append(notices, opts.resumeNotice)
	}
	if notice := mcpNotice(rec.Agent); notice != "" {
		notices = append(notices, notice)
	}
	view.Notice = strings.Join(notices, "; ")
	return view, nil
}

// A UI launch keeps its record as the row the user retries from. A detached
// one has no such reader, and a schedule firing every minute against a
// reached cap would otherwise add a dead row a minute to the sidebar. Failing
// to delete is not worth losing the launch error over.
func (s *AgentWorkspacesService) discardDetached(ctx context.Context, rec stores.AgentSession, opts terminalLaunch, cause error) error {
	if !opts.detached {
		return cause
	}
	_ = s.sessions.Delete(ctx, rec.ID)
	return cause
}

// attach opens the control client for name and returns its active window —
// id plus the size tmux granted, which the caller must surface so the pane
// opens its grid at tmux's size rather than the size it asked for.
func (s *AgentWorkspacesService) attach(ctx context.Context, name string, cols, rows int) (tmuxcc.Window, error) {
	windows, err := s.terminals.Attach(ctx, name, cols, rows)
	if err != nil {
		return tmuxcc.Window{}, terminalError(err, "attaching to session %q", name)
	}
	return firstActiveWindow(windows), nil
}

// firstActiveWindow returns windows' active entry, or the first when none is
// marked active, or a zero Window when there are none. An agent workspace
// session's own launch line never opens more than one window; a second one
// only ever comes from the agent itself running tmux new-window inside its
// own session, which this still addresses sensibly rather than erroring.
func firstActiveWindow(windows []tmuxcc.Window) tmuxcc.Window {
	for _, w := range windows {
		if w.Active {
			return w
		}
	}
	if len(windows) > 0 {
		return windows[0]
	}
	return tmuxcc.Window{}
}

// awaitEarlyExit gives a just-created tmux session earlyExitWindow to exit on
// its own. Unlike ptyterm's Exited event this carries no reason: tmux simply
// stops answering has-session once its initial pane's command has ended.
func (s *AgentWorkspacesService) awaitEarlyExit(ctx context.Context, name string) bool {
	deadline := time.Now().Add(earlyExitWindow)
	for {
		exists, err := s.terminals.HasSession(ctx, name)
		if err != nil || !exists {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(earlyExitPollInterval)
	}
}

// liveSessionCount counts live agentws-* tmux sessions across every
// workspace -- the service-level concurrency cap, since a live session is a
// tmux fact rather than something this service's own records track (a
// session record outlives the app; a tmux session need not).
func (s *AgentWorkspacesService) liveSessionCount(ctx context.Context) (int, error) {
	names, err := s.terminals.SessionNames(ctx, agentSessionPrefix)
	if err != nil {
		return 0, err
	}
	return len(names), nil
}

// resolveServers resolves a workspace's declared MCP ids through the store's
// catalogue. An id with no catalogue entry is simply omitted here; Generate
// itself reports it in MissingMCPs by comparing ws.MCPs against this map, so
// there is nothing to track twice.
func (s *AgentWorkspacesService) resolveServers(ctx context.Context, ws agentws.Workspace) map[string]mcpcatalog.Server {
	byID := make(map[string]mcpcatalog.Server, len(ws.MCPs))
	for _, entry := range s.catalogue(ctx) {
		byID[entry.ID] = entry.Server
	}
	servers := make(map[string]mcpcatalog.Server, len(ws.MCPs))
	for _, id := range ws.MCPs {
		if srv, ok := byID[id]; ok {
			servers[id] = srv
		}
	}
	return servers
}

// resolveSkills expands a workspace's enabled packages into the skills they
// select and renders each: a shared skill installs its authored file
// verbatim, a shipped one is rendered per install through
// SkillsService.RenderSkill, which takes the underlying prompt id ("mcp")
// rather than the installed slug ("hive-mcp") skillSlug mints. It also
// reports every enabled name skills.yml does not define, classified — a
// workspace naming one still opens, the same way a missing MCP id does not
// stop a launch.
func (s *AgentWorkspacesService) resolveSkills(ctx context.Context, ws agentws.Workspace) (rendered []agentws.RenderedSkill, missingPackages []MissingPackageItem, err error) {
	if len(ws.Skills) == 0 {
		return nil, nil, nil
	}

	// Unlike SkillCatalogue, neither read is allowed to degrade here: an
	// unreadable shared directory would silently generate a tree without
	// those skills, which reads as a successful open of a workspace that has
	// quietly lost half its capability.
	shared, err := s.sharedSkills()
	if err != nil {
		return nil, nil, Wrap(err, KindInternal, "workspace %q: reading the shared skills", ws.Dir)
	}
	shipped, err := s.shippedSkills(ctx)
	if err != nil {
		return nil, nil, Wrap(err, KindInternal, "workspace %q: reading the shipped skills", ws.Dir)
	}

	bodies := make(map[string]string, len(shared))
	for _, skill := range shared {
		bodies[skill.Slug] = skill.Body
	}

	library := s.store.SkillLibrary()
	names := agentws.SkillNames(shipped, shared)
	selected, unresolved := agentws.SelectSkills(library.Library, ws.Skills, names)
	for _, u := range agentws.ExplainUnresolved(agentws.SkillNameCatalogue(library.Library, names), unresolved) {
		missingPackages = append(missingPackages, MissingPackageItem{Name: u.Name, Skill: u.Skill, SelectedBy: u.SelectedBy})
	}

	rendered = make([]agentws.RenderedSkill, 0, len(selected))
	for _, name := range selected {
		if body, ok := bodies[name.Slug]; ok {
			rendered = append(rendered, agentws.RenderedSkill{Slug: name.Slug, Body: body})
			continue
		}
		id, ok := skillIDFromSlug(name.Slug)
		if !ok {
			continue
		}
		slug, body, err := s.skills.RenderSkill(ctx, id)
		if err != nil {
			return nil, nil, Wrap(err, KindInternal, "workspace %q: rendering skill %q", ws.Dir, name.Slug)
		}
		rendered = append(rendered, agentws.RenderedSkill{Slug: slug, Body: body})
	}
	return rendered, missingPackages, nil
}

// sessionViews reports read-only rows for records -- unlike launchTerminal's
// view, nothing here launches or attaches, so WindowID, ResumeAttempted and
// Notice stay zero-valued.
func (s *AgentWorkspacesService) sessionViews(ctx context.Context, records []stores.AgentSession) []SessionView {
	views := make([]SessionView, 0, len(records))
	for _, rec := range records {
		name := sessionName(rec)
		live := ""
		if alive, err := s.terminals.HasSession(ctx, name); err == nil && alive {
			live = name
		}
		views = append(views, SessionView{
			ID: rec.ID, Workspace: rec.Workspace, Name: rec.Name, Agent: rec.Agent,
			LastOpenedAt: rec.LastOpenedAt, Slug: name, TerminalID: live,
			ScheduleID: rec.ScheduleID,
		})
	}
	return views
}

func (s *AgentWorkspacesService) workspaceView(ctx context.Context, st agentws.WorkspaceStatus) WorkspaceView {
	problem, notice := "", ""
	if !st.Valid && st.Err != nil {
		problem = st.Err.Error()
	} else if st.Valid {
		// A workspace whose manifest failed to parse has no trustworthy
		// command to derive an agent from, so the MCP notice is skipped rather
		// than shown against whatever the zero value happens to be.
		notice = mcpNotice(st.Workspace.Agent())
	}
	return WorkspaceView{
		Dir: st.Dir, Name: st.Workspace.Name,
		Command: st.Workspace.Command, MCPs: st.Workspace.MCPs,
		Skills: st.Workspace.Skills, Problem: problem,
		Danger:    agentws.CommandIsDangerous(st.Workspace.Command),
		Notice:    notice,
		Schedules: s.history.rows(ctx, st.Workspace.Schedules, time.Now()),
	}
}

// resolvedFor returns a copy of ws whose Dir is the absolute workspace
// directory rather than the bare directory name Workspace normally carries.
// agentws.Resolve threads w.Dir straight into MCPWiring.Args, which needs a
// path that resolves regardless of the launched process's cwd.
func resolvedFor(ws agentws.Workspace, absoluteDir string) agentws.Workspace {
	ws.Dir = absoluteDir
	return ws
}

func sessionName(rec stores.AgentSession) string {
	return agentSessionPrefix + rec.TerminalID
}

// validWorkspaceDir reports whether dir resolves to a direct child of the
// workspace root: non-empty, not "." or "..", and a single path component. A
// workspace directory arrives as a string over the HTTP API (spec §8 hands
// this API to an agent), so it is validated the same way generate.go
// validates a skill slug rather than trusted.
func validWorkspaceDir(dir string) bool {
	if dir == "" || dir == "." {
		return false
	}
	return filepath.Base(dir) == dir && filepath.IsLocal(dir)
}

// mcpNotice reports the MCP-related explanation the UI shows for agent, or ""
// when its wiring bounds the tool set to exactly what the workspace declares.
func mcpNotice(agent string) string {
	bounded, ok := agentws.MCPBounded(agent)
	switch {
	case !ok:
		return "this agent has no known way to receive MCP servers; any declared servers are not passed to it"
	case !bounded:
		return "this agent's tool set is not limited to the workspace's declared MCP servers; it also loads its own global configuration"
	default:
		return ""
	}
}

// skillIDFromSlug reverses skillSlug ("hive-" + id): a workspace's skills:
// list carries the installed slug, but RenderSkill takes the underlying
// prompt id.
func skillIDFromSlug(slug string) (id string, ok bool) {
	const prefix = "hive-"
	if !strings.HasPrefix(slug, prefix) {
		return "", false
	}
	id = strings.TrimPrefix(slug, prefix)
	return id, id != ""
}

// AllSessions lists every session across every workspace, newest record
// first — the sidebar's cross-workspace read, unlike Sessions which scopes
// to one workspace.
func (s *AgentWorkspacesService) AllSessions(ctx context.Context) ([]SessionView, error) {
	records, err := s.sessions.ListAll(ctx)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing all sessions")
	}
	return s.sessionViews(ctx, records), nil
}

// agentLaunchError classifies an agentws launch-resolution failure by
// sentinel rather than by message (architecture.md, Errors). Both cases trace
// back to the workspace's own command template, so both are KindInvalid — and
// both are normally caught by validateEdit long before a launch, leaving this
// for a manifest hand-edited on disk.
func agentLaunchError(err error, dir string) error {
	switch {
	case errors.Is(err, agentws.ErrCommandEmpty):
		return Wrap(err, KindInvalid, "workspace %q has a command that renders to nothing", dir)
	case errors.Is(err, agentws.ErrCommandTemplate):
		return Wrap(err, KindInvalid, "workspace %q has an invalid command template", dir)
	default:
		return Wrap(err, KindInternal, "resolving the launch for workspace %q", dir)
	}
}

// classifyAgentScreen folds questions onto approval because the activity
// vocabulary has one user-blocked state.
func classifyAgentScreen(agent, screen string) terminal.Status {
	assessment := assess.NewEngine().Assess(assess.Snapshot{Content: screen, Tool: strings.ToLower(agent)})
	return terminalstatus.MapState(assessment.State).Simplified()
}
