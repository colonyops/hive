package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/hay-kot/httpkit/errchain"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/web"
	"github.com/colonyops/hive/cmd/desktop/internal/web/mid"
)

// Op is one HTTP operation. The operations table is what the mux is built
// from, and Summary/Request/Response/Errors document each route where it is
// declared. They no longer feed a generated document: the agent-facing surface
// this adapter used to describe is the MCP server's now (ADR mcp-replaces-the-agent-facing-http-api), and what
// is left here is the Wails frontend's own transport plus the liveness probe —
// consumers that read hand-written clients, not an OpenAPI spec.
type Op struct {
	Method   string
	Path     string
	Summary  string
	Request  any
	Response any
	Status   int       // success status; 0 means 200
	Errors   []ErrResp // documented non-success statuses beyond the generic default
	Handler  errchain.HandlerFunc
}

// ErrResp documents one non-success status an operation can return, and when.
// The error body is always the shared {kind, message, fields?} shape.
type ErrResp struct {
	Status int
	When   string
}

// pattern is the ServeMux pattern this op registers under. A path ending in "/"
// is anchored with {$} so it matches only that exact path rather than becoming
// a subtree that swallows every otherwise-unmatched /api/… request instead of
// 404ing.
func (op Op) pattern() string {
	path := op.Path
	if strings.HasSuffix(path, "/") {
		path += "{$}"
	}
	return op.Method + " " + path
}

func (ctrl *Controller) operations() []Op {
	ops := ctrl.baseOperations()
	ops = append(ops, ctrl.terminalOperations()...)
	ops = append(ops, ctrl.popupTerminalOperations()...)
	ops = append(ops, ctrl.agentOperations()...)
	return ops
}

// popupTerminalOperations is the ephemeral surface: terminals this process owns
// outright, opened on demand and addressed by an id it mints (ADR ephemeral-popup-terminals). They
// ride the terminal bearer token and CORS policy by sitting under its prefix.
func (ctrl *Controller) popupTerminalOperations() []Op {
	return []Op{
		{
			Method: "POST", Path: PopupTerminalPathPrefix + "open", Summary: "Open an ephemeral terminal and return it. The directory is resolved in order — the launcher's own cwd, sessionSlug's checkout, then dir (a leading ~ is expanded), then the user's home. command is a shell command line run through a login shell, so a user's aliases, functions and PATH resolve it; empty opens an interactive shell. launcher names a configured launcher (the launchers list in actions.yml) to open instead, and brings its own command. The terminal is this process's child: it has no name outside this run, nothing else can attach to it, and it ends when it is closed or when Hive exits. The data plane is a WebSocket served at " + PTYStreamPath + ", outside this operations table.",
			Request: popupOpenRequest{}, Response: popupTerminal{}, Handler: ctrl.PopupTerminalOpen,
			Errors: popupTerminalErrors("no hive session carries that slug, or no launcher carries that id",
				ErrResp{Status: 409, When: "the hive session is not active, so it has no checkout to open a terminal in"}),
		},
		{
			Method: "POST", Path: PopupTerminalPathPrefix + "close", Summary: "End a terminal and every process in it, and report whether there was one to close. An id whose process already exited answers closed=false rather than failing: an exited terminal is dropped, not kept.",
			Request: popupIDRequest{}, Response: popupCloseResponse{}, Handler: ctrl.PopupTerminalClose,
			Errors: popupTerminalErrors(""),
		},
		{
			Method: "POST", Path: PopupTerminalPathPrefix + "list", Summary: "List the open terminals, oldest first. Terminals whose process has exited are absent.",
			Response: popupListResponse{}, Handler: ctrl.PopupTerminalList,
			Errors: popupTerminalErrors(""),
		},
		{
			Method: "POST", Path: PopupTerminalPathPrefix + "resize", Summary: "Set a terminal's size. This is applied, not voted on: one client renders the PTY, so the size asked for is the size the process is told.",
			Request: popupSizeRequest{}, Status: http.StatusNoContent, Handler: ctrl.PopupTerminalResize,
			Errors: popupTerminalErrors("no open terminal carries that id"),
		},
	}
}

// popupTerminalErrors is terminalErrors with the unavailable reason that fits
// this surface: there is no program to install, so a 503 here is the build or
// the platform and nothing a user can act on.
func popupTerminalErrors(notFound string, extra ...ErrResp) []ErrResp {
	errs := []ErrResp{{Status: 401, When: "the Authorization: Bearer token is missing or wrong"}}
	if notFound != "" {
		errs = append(errs, ErrResp{Status: 404, When: notFound})
	}
	errs = append(errs, extra...)
	return append(errs, ErrResp{Status: 503, When: "ephemeral terminals are unavailable: an unsupported platform or a server build"})
}

// agentOperations is the agent-workspace control plane: named, durable
// workspaces where a CLI agent runs against a purpose-built MCP tool set
// (spec-tracked as hc-49x3i833). Like popupTerminalOperations it rides the
// terminal bearer token and CORS policy by sitting under TerminalPathPrefix —
// starting a session spawns an agent CLI, which is arbitrary command
// execution (ADR terminal-transport, ADR a-workspace-declares-its-own-authority).
func (ctrl *Controller) agentOperations() []Op {
	return []Op{
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "workspaces", Summary: "List every recognized agent workspace under the configured root, valid or not. A workspace whose manifest fails to parse still lists with its last-good name and agent, plus a problem explaining what is wrong. available/error report whether ephemeral terminals can run at all in this build; root is the configured workspace root regardless of that answer. presets lists the starter command templates the editor offers (the ones this build ships plus one per agent profile in hive's config); they fill a workspace's command field and never constrain it. editor names the configured open-in-editor command, empty when none is set.",
			Response: agentWorkspacesResponse{}, Handler: ctrl.AgentWorkspaces,
			Errors: agentErrors(""),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "workspaces/open", Summary: "Regenerate a workspace's disposable artifacts (CLAUDE.md, .mcp.json, .codex/config.toml, .claude/, .agents/, an empty docs/) from its manifest and return its sessions. This is the only call that writes into a workspace; missingMcps names declared MCP ids the catalogue does not resolve, missingPackages skill packages skills.yml does not define.",
			Request: agentWorkspaceDirRequest{}, Response: agentWorkspaceOpenResponse{}, Handler: ctrl.AgentWorkspaceOpen,
			Errors: agentErrors("no such workspace, or its manifest is invalid"),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "workspaces/create", Summary: "Create a workspace: a new directory under the root with a fresh agent-workspace.yaml naming the given name, command template, and any scheduled chats.",
			Request: agentWorkspaceEditRequest{}, Response: agentWorkspaceView{}, Handler: ctrl.AgentWorkspaceCreate,
			Errors: agentErrors("", ErrResp{Status: 409, When: "a workspace directory of that name already exists"}),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "workspaces/update", Summary: "Rewrite a workspace manifest's editable fields (name, command, mcps, skills — which names skill packages, not individual skills — and schedules) in place. The command is a Go template over .Dir, .MCPConfig, .SessionID, .Resume and .Prompt, rejected here if it does not render; a workspace with schedules needs one that passes .Prompt. Comments, key order, and keys the editor does not own survive the write; an empty mcps, skills or schedules removes the key. The schedules list is reconciled to exactly what is sent: an entry it no longer names is deleted, and each schedule's id shape, cron expression and prompt template are validated before anything is written.",
			Request: agentWorkspaceEditRequest{}, Response: agentWorkspaceView{}, Handler: ctrl.AgentWorkspaceUpdate,
			Errors: agentErrors("no such workspace"),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "workspaces/delete", Summary: "Delete a workspace: end every live terminal its sessions hold, remove the workspace directory and everything under it — canvases and authored files included — then delete the session records. Refuses a directory the root does not list as a workspace.",
			Request: agentWorkspaceDirRequest{}, Status: http.StatusNoContent, Handler: ctrl.AgentWorkspaceDelete,
			Errors: agentErrors(""),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "workspaces/open-in-editor", Summary: "Launch the configured editor (Settings › General) on the workspace directory, detached. Fails when no editor is configured or the command does not resolve on PATH.",
			Request: agentWorkspaceDirRequest{}, Status: http.StatusNoContent, Handler: ctrl.AgentWorkspaceOpenInEditor,
			Errors: agentErrors("no such workspace", ErrResp{Status: 400, When: "no editor is configured, or its command is not on PATH"}),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "workspaces/reveal", Summary: "Open the workspace directory in the OS file manager.",
			Request: agentWorkspaceDirRequest{}, Status: http.StatusNoContent, Handler: ctrl.AgentWorkspaceReveal,
			Errors: agentErrors("no such workspace"),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "mcps", Summary: "List the merged MCP catalogue: shipped entries plus the user's mcps.yaml, sorted by id, each with its stability, resolved command line, and any problem (a command that does not resolve on PATH). A user id shadowing a shipped one wins and says so.",
			Response: agentMCPCatalogueResponse{}, Handler: ctrl.AgentMCPCatalogue,
			Errors: agentErrors(""),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "mcps/import", Summary: "Parse pasted MCP JSON — claude's {\"mcpServers\": {...}} wrapper or a bare id-to-server map — into mcps.yaml and return the refreshed catalogue. An id already declared in mcps.yaml is a conflict; edit the file to change an existing entry.",
			Request: agentMCPImportRequest{}, Response: agentMCPImportResponse{}, Handler: ctrl.AgentMCPImport,
			Errors: agentErrors("", ErrResp{Status: 409, When: "a pasted id is already declared in mcps.yaml"}),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "mcps/remove", Summary: "Delete a user-declared server from mcps.yaml and return the refreshed catalogue. Shipped entries are refused — a workspace disables one by dropping the id from its own mcps list.",
			Request: agentMCPRemoveRequest{}, Response: agentMCPCatalogueResponse{}, Handler: ctrl.AgentMCPRemove,
			Errors: agentErrors("no user-declared server of that id", ErrResp{Status: 400, When: "the id names a shipped entry"}),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "skills", Summary: "List the skill packages defined in skills.yml, each with the skills its glob patterns currently select. Names come from the skills this build ships (hive-*) and the ones authored under .shared/skills. A workspace carries a package's skills only by naming the package in its own skills list.",
			Response: agentSkillPackagesResponse{}, Handler: ctrl.AgentSkillPackages,
			Errors: agentErrors(""),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "skills/reveal", Summary: "Open skills.yml, where packages are defined, seeding it first if it is missing.",
			Status: http.StatusNoContent, Handler: ctrl.AgentSkillPackagesReveal,
			Errors: agentErrors(""),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "skills/shared", Summary: "Open the shared skills directory (.shared/skills) in the OS file manager, creating it if missing. A skill there is a SKILL.md in a directory named for it, and a package selects it by name.",
			Status: http.StatusNoContent, Handler: ctrl.AgentSharedSkillsReveal,
			Errors: agentErrors(""),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "sessions", Summary: "List a workspace's sessions without regenerating its artifacts, unlike workspaces/open. terminalId is empty for a session with no live terminal.",
			Request: agentSessionsRequest{}, Response: agentSessionsResponse{}, Handler: ctrl.AgentSessions,
			Errors: agentErrors(""),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "sessions/all", Summary: "List every session across every workspace, newest first in stable creation order — the sidebar's cross-workspace read, unlike sessions which scopes to one workspace.",
			Response: agentSessionsAllResponse{}, Handler: ctrl.AgentSessionsAll,
			Errors: agentErrors(""),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "sessions/start", Summary: "Launch a new, named session in a workspace: renders the workspace's command template into a command line, creates a detached tmux session named agentws-<terminal id> running it, and attaches. cols/rows of 0x0 attach unsized. The data plane is the tmux stream at " + TerminalStreamPath + ", outside this operations table; windowId names the pane to frame input/output for.",
			Request: agentSessionStartRequest{}, Response: agentSessionView{}, Handler: ctrl.AgentSessionStart,
			Errors: agentErrors("no such workspace", ErrResp{Status: 503, When: "tmux is unavailable: an unsupported platform, missing tmux, or a server build"}),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "sessions/first-run", Summary: "Open the seeded Hive workspace on the interview that ends first run: regenerates the workspace, then launches a detached session named \"Getting started\" whose opening message is the app's own first-run prompt. The caller attaches by routing to the returned session.",
			Response: agentSessionView{}, Handler: ctrl.AgentSessionStartFirstRun,
			Errors: agentErrors("the seeded Hive workspace is gone", ErrResp{Status: 503, When: "tmux is unavailable: an unsupported platform, missing tmux, or a server build"}),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "sessions/resume", Summary: "Reattach a session's live tmux session if it still has one, or relaunch it — resuming the agent's own conversation when it has a resume form (resumeAttempted), and starting a fresh one with a notice when it does not.",
			Request: agentSessionResumeRequest{}, Response: agentSessionView{}, Handler: ctrl.AgentSessionResume,
			Errors: agentErrors("no such session, or its workspace is gone", ErrResp{Status: 503, When: "tmux is unavailable: an unsupported platform, missing tmux, or a server build"}),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "sessions/close", Summary: "End a session's live tmux session and report whether there was one to close. The session record is untouched, so it still lists afterward.",
			Request: agentSessionIDRequest{}, Response: agentSessionCloseResponse{}, Handler: ctrl.AgentSessionClose,
			Errors: agentErrors("no such session"),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "sessions/rename", Summary: "Set a session's display name. The record is the only thing touched — a live tmux session keeps its agentws-<terminal id> name.",
			Request: agentSessionRenameRequest{}, Status: http.StatusNoContent, Handler: ctrl.AgentSessionRename,
			Errors: agentErrors("no such session"),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "sessions/activity", Summary: "Classify live sessions from their captured tmux panes: ready, active, or approval — approval is the highest-urgency state. workspace scopes to one workspace; empty spans every workspace. A session with no live tmux session is omitted.",
			Request: agentSessionActivityRequest{}, Response: agentSessionActivityResponse{}, Handler: ctrl.AgentSessionActivity,
			Errors: agentErrors(""),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "sessions/resize", Summary: "Vote a size for a session's attached control client, the same renegotiation the terminal pane casts on a host resize; tmux answers on the stream with a layout-changed window event, which is what sets the grid.",
			Request: agentSessionResizeRequest{}, Status: http.StatusNoContent, Handler: ctrl.AgentSessionResize,
			Errors: agentErrors("no such session, or it has no attached terminal"),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "sessions/delete", Summary: "End any live terminal and delete a session's record.",
			Request: agentSessionIDRequest{}, Status: http.StatusNoContent, Handler: ctrl.AgentSessionDelete,
			Errors: agentErrors("no such session"),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "canvas", Summary: "Read one canvas by workspace and name: editable front matter and every block in order, as the agent last wrote them. A name nothing was written under answers an empty canvas. Block and front matter mutations have no HTTP surface — the agent writes through the hive-canvas MCP server.",
			Request: agentCanvasRequest{}, Response: agentCanvasView{}, Handler: ctrl.AgentCanvas,
			Errors: agentErrors(""),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "canvas/delete", Summary: "Delete one canvas selected by the user and return its name. The file is removed from its workspace, repository context, or global canvas directory. A name that does not exist is 404.",
			Request: agentCanvasRequest{}, Response: agentCanvasDeleteResponse{}, Handler: ctrl.AgentCanvasDelete,
			Errors: agentErrors("no such canvas"),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "canvases", Summary: "List a workspace's canvases, most recently updated first — metadata only (name, title, creating session, timestamps, block count), for the canvas pane's picker. Canvases are files in the workspace folder and outlive the chats that made them.",
			Request: agentCanvasListRequest{}, Response: agentCanvasListResponse{}, Handler: ctrl.AgentCanvasList,
			Errors: agentErrors(""),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "canvas/repositories", Summary: "List the repositories that hold a canvas, as owner/repo keys. A hive session's canvases belong to its repository and live in hive's context directory, so they outlive the session; the key is what the other canvas routes take as workspace.",
			Request: struct{}{}, Response: agentCanvasRepositoriesResponse{}, Handler: ctrl.AgentCanvasRepositories,
			Errors: agentErrors(""),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "canvas/markdown", Summary: "Render one canvas as a standalone markdown document with YAML front matter, for the pane's copy-to-clipboard action. A name nothing was written under is 404 — exporting nothing is a mistake worth surfacing.",
			Request: agentCanvasRequest{}, Response: agentCanvasMarkdownResponse{}, Handler: ctrl.AgentCanvasMarkdown,
			Errors: agentErrors("no such canvas"),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "canvas/export", Summary: "Write one canvas's markdown rendering, including YAML front matter, to an absolute path the user chose in the native save dialog. The canvas itself is untouched.",
			Request: agentCanvasExportRequest{}, Response: agentCanvasExportResponse{}, Handler: ctrl.AgentCanvasExport,
			Errors: agentErrors("no such canvas"),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "schedules/run", Summary: "Fire one schedule now, outside its timetable, and return the run it recorded. The cursor is untouched, so the next real occurrence still happens. A run whose previous chat is still open, or whose prompt or launch failed, answers 200 with that outcome on the run rather than an error.",
			Request: agentScheduleIDRequest{}, Response: agentScheduleRunResponse{}, Handler: ctrl.AgentScheduleRun,
			Errors: agentErrors("no schedule of that id in that workspace",
				ErrResp{Status: 503, When: "tmux is unavailable, so whether the previous run's chat is still open cannot be answered"}),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "schedules/runs", Summary: "List one schedule's run history, newest first. A limit of 0 takes the default. History outlives the manifest entry it came from, so a schedule that was deleted, or whose workspace manifest is broken, still answers.",
			Request: agentScheduleRunsRequest{}, Response: agentScheduleRunsResponse{}, Handler: ctrl.AgentScheduleRuns,
			Errors: agentErrors(""),
		},
		{
			Method: "POST", Path: AgentWorkspacesPathPrefix + "schedules/preview", Summary: "Dry-run an unsaved edit: the next occurrences the cron produces, and the prompt rendered against sample data. A cron or template that does not parse comes back in cronError/promptError rather than as a failed call, so the editor can show it beside the field being typed in.",
			Request: agentSchedulePreviewRequest{}, Response: agentSchedulePreviewResponse{}, Handler: ctrl.AgentSchedulePreview,
			Errors: agentErrors(""),
		},
	}
}

// agentErrors documents what every agent-workspace operation can answer
// beyond the generic error: the bearer token these routes require, same as
// every other terminal-prefixed route.
func agentErrors(notFound string, extra ...ErrResp) []ErrResp {
	errs := []ErrResp{{Status: 401, When: "the Authorization: Bearer token is missing or wrong"}}
	if notFound != "" {
		errs = append(errs, ErrResp{Status: 404, When: notFound})
	}
	return append(errs, extra...)
}

// baseOperations is what is left of this adapter's own surface after the
// agent-facing API moved to MCP (ADR mcp-replaces-the-agent-facing-http-api): a liveness probe and the running
// build. Both stay HTTP because they answer the question "is the app up, and
// which build is it?" — one a shell script or a health check asks with a plain
// GET, and a JSON-RPC handshake is the wrong shape for it. Everything an agent
// drives is a tool on the MCP server now, with one exception: the call a chat
// makes about itself, ending its own session, which is a curl from inside the
// agent's shell with the token its launch handed it
// (ADR a-scheduled-chat-ends-itself-through-a-capability-token-its-launch-handed-it).
func (ctrl *Controller) baseOperations() []Op {
	return []Op{
		{
			Method: "GET", Path: "/api/version", Summary: "Report the running build's VCS identity.",
			Response: struct {
				Service string    `json:"service"`
				Build   web.Build `json:"build"`
			}{}, Handler: plain(ctrl.version),
		},
		{
			Method: "GET", Path: "/api/status", Summary: "Report whether the webhook listener is running and on which host and port.",
			Response: statusResponse{}, Handler: ctrl.Status,
		},
		{
			Method: "POST", Path: app.AgentSessionEndPath, Summary: "End the calling chat's own session. The bearer is the HIVE_AGENT_SESSION_TOKEN the launch handed that process, so a chat can end itself and nothing else; a scheduled chat is told to call this when its task is done. Answers 202 with when the chat will be gone: the request arrives from inside the agent's own tool call, and the grace (agent_workspaces.session_end_delay) lets that call return first. The chat is deleted with its session, so a schedule leaves no row per run; its run history keeps the outcome.",
			Response: agentSessionEndResponse{}, Handler: ctrl.AgentSessionEnd,
			Errors: []ErrResp{{Status: 401, When: "the Authorization: Bearer token is missing or is not a session's"}},
		},
	}
}

func (ctrl *Controller) terminalOperations() []Op {
	return []Op{
		{Method: "POST", Path: TerminalPathPrefix + "images/paths", Summary: "Prepare local image paths for terminal paste.", Request: terminalImagePathsRequest{}, Response: terminalImagesResponse{}, Handler: ctrl.TerminalImagePaths},
		{Method: "POST", Path: TerminalPathPrefix + "images/upload", Summary: "Store clipboard images and return separate path pastes.", Response: terminalImagesResponse{}, Handler: ctrl.TerminalImageUpload},
		{Method: "POST", Path: TerminalPathPrefix + "panes/paste", Summary: "Paste text into the named pane without submitting it.", Request: terminalPasteRequest{}, Status: http.StatusNoContent, Handler: ctrl.TerminalPaste},
		{
			Method: "POST", Path: "/api/terminal/attach", Summary: "Attach a tmux control-mode client to a session slug and return its windows. Attaching never spawns: a slug tmux is not running answers 404, and POST /api/terminal/start is what creates it. cols/rows are the opening size vote; 0x0 attaches without setting a client size, leaving the session at the size its other clients gave it. The data plane is a WebSocket served at " + TerminalStreamPath + ", outside this operations table.",
			Request: terminalAttachRequest{}, Response: terminalAttachResponse{}, Handler: ctrl.TerminalAttach,
			Errors: terminalErrors("the slug names no reachable tmux session"),
		},
		{
			Method: "POST", Path: "/api/terminal/start", Summary: "Spawn the tmux session a slug names, from the hive session's own spawn configuration — the windows, working directory and agent command hive itself would use — and report whether this call is what created it. A session tmux is already running answers started=false rather than being respawned. Starting runs the session's agent command, which is why it is a separate call from attach. The slug \"Scratch\" is reserved for the scratch terminal, which belongs to no hive session: starting it opens one window in the user's home directory, and so does every window added to it afterwards.",
			Request: terminalSlugRequest{}, Response: terminalStartResponse{}, Handler: ctrl.TerminalStart,
			Errors: terminalErrors("no hive session carries that slug",
				ErrResp{Status: 409, When: "the hive session is not active, so it has no checkout to open a terminal in"}),
		},
		{
			Method: "POST", Path: "/api/terminal/kill", Summary: "Kill the tmux session a slug names and report whether there was one to kill. This is the terminal's lifecycle only — the hive session, its checkout and its record are untouched — but whatever is running inside it, the agent included, stops. A slug tmux is not running answers killed=false rather than failing.",
			Request: terminalSlugRequest{}, Response: terminalKillResponse{}, Handler: ctrl.TerminalKill,
			Errors: terminalErrors(""),
		},
		{
			Method: "POST", Path: "/api/terminal/resize", Summary: "Vote a size for the attached control client; every client attached to a window renders the same grid and tmux's window-size option decides whose size that is.",
			Request: terminalSizeRequest{}, Status: http.StatusNoContent, Handler: ctrl.TerminalResize,
			Errors: terminalErrors("no terminal is attached for that slug"),
		},
		{
			Method: "POST", Path: "/api/terminal/windows/new", Summary: "Create a window in a session and return its tmux window id. Attaching first is not required: a session with no control client gets its window from a one-shot, opened in the session's own working directory the same way an attached client's would be. A slug tmux is not running answers 404 — POST /api/terminal/start is what creates the session.",
			Request: terminalSlugRequest{}, Response: terminalNewWindowResponse{}, Handler: ctrl.TerminalNewWindow,
			Errors: terminalErrors("the slug names no running tmux session"),
		},
		{
			Method: "POST", Path: "/api/terminal/windows/agent", Summary: "Start a configured agent profile in a peer window of a running Hive session, sharing its checkout. Returns the new tmux window id.",
			Request: terminalAgentWindowRequest{}, Response: terminalNewWindowResponse{}, Handler: ctrl.TerminalNewAgentWindow,
			Errors: terminalErrors("the slug names no active Hive session or running tmux session"),
		},
		{
			Method: "POST", Path: "/api/terminal/windows/close", Summary: "Kill one window of the attached session.",
			Request: terminalWindowRequest{}, Status: http.StatusNoContent, Handler: ctrl.TerminalCloseWindow,
			Errors: terminalErrors("no terminal is attached for that slug, or no such window"),
		},
		{
			Method: "POST", Path: "/api/terminal/windows/foreground", Summary: "Report whether a window is running anything a close would kill. running is false only when every live pane in it is a shell waiting at its prompt; anything else answers true and names the foreground process, so a caller can say what it is about to stop. A pane whose state cannot be read answers true rather than being reported idle.",
			Request: terminalWindowRequest{}, Response: terminalForegroundResponse{}, Handler: ctrl.TerminalWindowForeground,
			Errors: terminalErrors("no terminal is attached for that slug, or no such window"),
		},
		{
			Method: "POST", Path: "/api/terminal/windows/rename", Summary: "Rename one window of the attached session.",
			Request: terminalRenameRequest{}, Status: http.StatusNoContent, Handler: ctrl.TerminalRenameWindow,
			Errors: terminalErrors("no terminal is attached for that slug, or no such window"),
		},
		{
			Method: "POST", Path: "/api/terminal/windows/move", Summary: "Move one window of the attached session to a position in its window order and answer with the order tmux settled on. position is a 0-based index into the resulting order, the way a drop on a tab strip means one; tmux's own indices are renumbered afterwards so they stay contiguous. The active window is preserved, and the move is tmux session state — every other client attached to the session sees it too.",
			Request: terminalMoveRequest{}, Response: terminalWindowsResponse{}, Handler: ctrl.TerminalMoveWindow,
			Errors: terminalErrors("no terminal is attached for that slug, or no such window",
				ErrResp{Status: 400, When: "the position is outside the session's window order"}),
		},
		{
			Method: "POST", Path: "/api/terminal/windows/select", Summary: "Make one window the attached session's active window.",
			Request: terminalWindowRequest{}, Status: http.StatusNoContent, Handler: ctrl.TerminalSelectWindow,
			Errors: terminalErrors("no terminal is attached for that slug, or no such window"),
		},
		{
			Method: "POST", Path: "/api/terminal/panes/split", Summary: "Split one pane of the attached session and return the new pane's id. direction is tmux's: horizontal puts the new pane to the right, vertical below. The new pane opens where the split pane is, and the window's new layout — with the new pane's first paint behind it — arrives on the stream as a layout-changed window event.",
			Request: terminalSplitRequest{}, Response: terminalSplitResponse{}, Handler: ctrl.TerminalSplitPane,
			Errors: terminalErrors("no terminal is attached for that slug, or no such pane",
				ErrResp{Status: 422, When: "the direction is not horizontal or vertical"}),
		},
		{
			Method: "POST", Path: "/api/terminal/panes/select", Summary: "Make one pane its window's active pane — the pane itself, or with direction left/right/up/down, the neighbour tmux's own select-pane would pick from it. The active pane is tmux window state: it is where the next split opens and what every other attached client sees selected, and the stream announces the result as an active-changed window event carrying activePane.",
			Request: terminalSelectPaneRequest{}, Status: http.StatusNoContent, Handler: ctrl.TerminalSelectPane,
			Errors: terminalErrors("no terminal is attached for that slug, or no such pane",
				ErrResp{Status: 422, When: "the direction is not one of left, right, up, down"}),
		},
		{
			Method: "POST", Path: "/api/terminal/panes/close", Summary: "Kill one pane of the attached session. The last pane of a window takes the window with it, announced as a closed window event like any other close.",
			Request: terminalPaneRequest{}, Status: http.StatusNoContent, Handler: ctrl.TerminalClosePane,
			Errors: terminalErrors("no terminal is attached for that slug, or no such pane"),
		},
		{
			Method: "POST", Path: "/api/terminal/panes/foreground", Summary: "Report whether a pane is running anything a close would kill — the same question POST /api/terminal/windows/foreground answers for a whole window, asked of one pane. running is false only when the pane is a shell waiting at its prompt; a pane whose state cannot be read answers true.",
			Request: terminalPaneRequest{}, Response: terminalForegroundResponse{}, Handler: ctrl.TerminalPaneForeground,
			Errors: terminalErrors("no terminal is attached for that slug, or no such pane"),
		},
		{
			Method: "POST", Path: "/api/terminal/panes/resize", Summary: "Set a pane's width and/or height in cells; 0 leaves that axis alone. tmux moves the divider on the far side of the pane's cell in its parent split — the near side for the last cell — and the neighbours give or take the difference, so a caller dragging a divider names the pane before it. The layout tmux settled on arrives on the stream.",
			Request: terminalResizePaneRequest{}, Status: http.StatusNoContent, Handler: ctrl.TerminalResizePane,
			Errors: terminalErrors("no terminal is attached for that slug, or no such pane",
				ErrResp{Status: 400, When: "neither dimension is set, or one is outside 1..1000"}),
		},
		{
			Method: "POST", Path: "/api/terminal/panes/zoom", Summary: "Toggle a pane between filling its window and its place in the layout. tmux makes the pane active on the way in. The window event that follows carries zoomed and the unchanged layout the pane goes back to.",
			Request: terminalPaneRequest{}, Status: http.StatusNoContent, Handler: ctrl.TerminalZoomPane,
			Errors: terminalErrors("no terminal is attached for that slug, or no such pane"),
		},
		{
			Method: "POST", Path: "/api/terminal/windows/list", Summary: "List several sessions' windows without attaching, keyed by slug. The whole set is answered from one tmux call. An attached slug answers from its live client; a slug with no tmux session behind it is absent from the answer rather than an error.",
			Request: terminalSlugsRequest{}, Response: terminalSessionWindowsResponse{}, Handler: ctrl.TerminalListWindows,
			Errors: terminalErrors(""),
		},
		{
			Method: "POST", Path: "/api/terminal/detach", Summary: "Close the control client, leaving the tmux session itself running.",
			Request: terminalSlugRequest{}, Status: http.StatusNoContent, Handler: ctrl.TerminalDetach,
			Errors: terminalErrors("no terminal is attached for that slug"),
		},
	}
}

// terminalErrors documents what every terminal operation can answer beyond the
// generic error: the bearer token these — and only these — routes require, and
// tmux being absent or too old. An empty notFound means the operation has no
// 404: an absent target is one of its answers, not one of its failures. extra
// carries whatever else one operation alone can answer.
func terminalErrors(notFound string, extra ...ErrResp) []ErrResp {
	errs := []ErrResp{{Status: 401, When: "the Authorization: Bearer token is missing or wrong"}}
	if notFound != "" {
		errs = append(errs, ErrResp{Status: 404, When: notFound})
	}
	errs = append(errs, extra...)
	return append(errs, ErrResp{Status: 503, When: "tmux is unavailable: missing, older than 3.2, or an unsupported build"})
}

func (ctrl *Controller) Handler() http.Handler {
	chain := errchain.New(mid.Errors(ctrl.log, mapAppError))

	mux := http.NewServeMux()
	preflighted := map[string]bool{}
	for _, op := range ctrl.operations() {
		handler := chain.ToHandlerFunc(op.Handler)
		if strings.HasPrefix(op.Path, TerminalPathPrefix) {
			handler = ctrl.cors.wrap(handler)
			if !preflighted[op.Path] {
				preflighted[op.Path] = true
				mux.HandleFunc("OPTIONS "+op.Path, ctrl.cors.preflight)
			}
		}
		mux.HandleFunc(op.pattern(), handler)
	}
	return mid.Logger(ctrl.log, "/api/status", "/api/version")(mux)
}

// plain adapts a pre-built handler (the shared version handler) to the error
// chain; it writes its own response and never fails.
func plain(h http.HandlerFunc) errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		h(w, r)
		return nil
	}
}

// mapAppError maps a core error's Kind to an HTTP status exactly once; an
// *app.Error marshals to {kind, message}.
func mapAppError(err error) (int, any, bool) {
	appErr, ok := errors.AsType[*app.Error](err)
	if !ok {
		return 0, nil, false
	}
	return statusForKind(appErr.Kind), appErr, true
}

func statusForKind(k app.Kind) int {
	switch k {
	case app.KindInvalid:
		return http.StatusBadRequest
	case app.KindNotFound:
		return http.StatusNotFound
	case app.KindConflict:
		return http.StatusConflict
	case app.KindUnauthenticated:
		return http.StatusUnauthorized
	case app.KindUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
