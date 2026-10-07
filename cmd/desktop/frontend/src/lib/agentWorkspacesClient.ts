// The Agents area's control-plane transport: plain fetch calls against
// /api/terminal/agents/*, using the bearer token AgentsService.Endpoint()
// hands the webview (cmd/desktop/internal/adapter/httpapi/ctrl_agent_workspaces.go). The
// data plane is the tmux stream terminal mode uses (ADR agent-workspace-sessions-are-tmux-sessions) — a session is
// a tmux session, just not a hive one — so this module reuses
// terminalClient's openStream(name)/decodeFrame/encodeInputFrames as-is
// rather than reimplementing the windowed wire: openStream is a stateless
// closure over whichever endpoint it was built from, and
// TerminalEndpoint/AgentsEndpoint are the same shape. Start/Resume attach
// server-side (AgentWorkspacesService) and return windowId, so this module
// never calls the generic /api/terminal/attach control route, which is gated
// by experimental.terminal — a flag the Agents area must not depend on.

import { createTerminalClient, decodeFrame, encodeInputFrames, encodePasteFrames } from './terminalClient'
import type { TerminalEndpoint } from './terminalClient'
import { Endpoint } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice'
import type { AgentsEndpoint } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

export type { AgentsEndpoint }
export { decodeFrame, encodeInputFrames, encodePasteFrames }
export type { TerminalFrame } from './terminalClient'

/** One row of the area's workspace list. */
export interface AgentWorkspace {
  dir: string
  name: string
  /** The launch template — the whole invocation, rendered at spawn. */
  command: string
  mcps: string[]
  skills: string[]
  /** The manifest's schedules, each joined with its next and last run. */
  schedules: AgentSchedule[]
  problem: string
  /** The command carries a permission bypass this build recognizes. */
  danger: boolean
  /** An unbounded-MCP or missing-manifest explanation, empty when neither applies. */
  notice: string
}

/** A starter command template the editor offers. */
export interface AgentPreset {
  id: string
  /** The CLI the command actually runs, for the brand mark beside the row. */
  agent: string
  /** The row's name: the posture for a shipped preset, the profile key for a hive-seeded one. */
  label: string
  command: string
  danger: boolean
  /** "builtin" for a shipped preset, "hive" for one seeded from hive's agents: profiles. */
  source: string
}

/** The configured "open in editor" target; an empty command means none is configured. */
export interface AgentEditor {
  command: string
  title: string
}

export interface AgentWorkspacesPayload {
  root: string
  /** Non-empty when the configured root itself could not be created or opened. */
  rootProblem: string
  available: boolean
  error: string
  workspaces: AgentWorkspace[]
  /** Starter command templates. They fill the command field; they never constrain it. */
  presets: AgentPreset[]
  editor: AgentEditor
}

/** The manifest fields the in-app workspace editor writes. */
export interface WorkspaceEditRequest {
  dir: string
  name: string
  command: string
  mcps: string[]
  /** Skill package names from skills.yml, not individual skills. */
  skills: string[]
  /**
   * The whole `schedules:` list. The manifest is reconciled to it, so an entry
   * left out here is removed from the workspace.
   */
  schedules: ScheduleEdit[]
}

/** One row of the merged MCP catalogue: shipped entries plus the user's mcps.yaml. */
export interface MCPCatalogueEntry {
  id: string
  title: string
  description: string
  shipped: boolean
  stability: string
  /** The shipped id this user entry replaces, empty otherwise. */
  shadows: string
  transport: string
  /** What the entry launches: the command line for stdio, the URL for http/sse. */
  command: string
  /** Why the entry will not work — a command that does not resolve on PATH. */
  problem: string
}

/**
 * One skill a package selects, with where it came from: shipped by this build
 * (rendered per install) or authored under .shared/skills.
 */
export interface SkillPackageMember {
  slug: string
  shipped: boolean
}

/**
 * One skill package from skills.yml, with the skills its glob patterns select
 * right now. Members are resolved, not stored — a new skill matching the
 * pattern joins every workspace that enabled the package.
 */
export interface SkillPackage {
  name: string
  title: string
  description: string
  members: SkillPackageMember[]
}

/**
 * One name in the skills name-space, with the packages that select it. The
 * editor needs the whole name-space to tell an enabled name that is really a
 * skill from one that matches nothing at all.
 */
export interface SkillName {
  slug: string
  shipped: boolean
  selectedBy: string[]
}

/**
 * The package catalogue, the name-space it selects over, and why skills.yml
 * could not be read, if it could not.
 */
export interface SkillPackagesPayload {
  packages: SkillPackage[]
  skills: SkillName[]
  problem: string
}

/** One row of a workspace's session list. */
export interface AgentSession {
  id: string
  /** Numeric ID from before the UUID migration, present only on migrated chats. */
  legacyId?: string
  workspace: string
  name: string
  agent: string
  lastOpenedAt: number
  /**
   * The tmux session name this chat is addressed by whether or not it is
   * running — the key a row, a route or an attach pool is safe to hold.
   * `terminalId`, not this, is what reports liveness.
   */
  slug: string
  /** The tmux session name on the shared terminal stream; empty when nothing is running. */
  terminalId: string
  /**
   * terminalId's active tmux window, needed to frame input/output on the
   * windowed tmux wire. Set only by startSession/resumeSession, which attach;
   * a plain sessions() listing leaves it empty even for a live session.
   */
  windowId: string
  /** Active tmux pane used to frame input; set with windowId by start/resume. */
  paneId: string
  /**
   * tmux's own size for that window at attach — the grid the pane must open
   * at, which may differ from the cols/rows voted (tmux's window-size option
   * picks whose size wins). 0 when tmux has not reported one. Set only by
   * startSession/resumeSession, like windowId.
   */
  cols: number
  rows: number
  resumeAttempted: boolean
  notice: string
  /** The schedule that launched this chat, empty when a person started it. */
  scheduleId: string
}

/** One live session's detected activity — ready, active, or approval. */
export interface AgentSessionActivity {
  id: string
  status: 'ready' | 'active' | 'approval' | string
}

/**
 * One enabled name skills.yml does not define. `skill` marks the name as a
 * skill rather than a package — what a manifest written before packages
 * carries — and `selectedBy` names the packages that already select it, which
 * is the fix. Neither is set for a name that matches nothing: that is a typo.
 */
export interface MissingSkillPackage {
  name: string
  skill: boolean
  selectedBy: string[]
}

/**
 * One block on a canvas; kind decides which content field is set. An `html`
 * block's `body` arrives sanitized by the Go side, never as the agent wrote it.
 */
export interface CanvasBlock {
  id: string
  kind: 'markdown' | 'html' | 'link' | string
  title: string
  body: string
  url: string
  createdAt: number
  updatedAt: number
}

/**
 * One named canvas, blocks in display order. `workspace` is its owner key: a
 * workspace's directory name, or `owner/repo` for a repository's canvases.
 * `session` is the chat that created it and `hiveSession` the hive session;
 * one of the two is empty.
 */
export interface WorkspaceCanvas {
  workspace: string
  name: string
  title: string
  session: string
  hiveSession: string
  createdAt: number
  updatedAt: number
  blocks: CanvasBlock[]
}

/** One row of an owner's canvas listing — metadata only, for the picker. */
export interface WorkspaceCanvasMeta {
  workspace: string
  name: string
  title: string
  session: string
  hiveSession: string
  createdAt: number
  updatedAt: number
  blockCount: number
}

/**
 * One `schedules:` entry from a workspace manifest, joined with the app-local
 * run state the Go side keeps: when it fires next and how it went last time.
 * `nextRunAt` is null when the schedule is disabled or its cron does not parse.
 */
export interface AgentSchedule {
  id: string
  name: string
  cron: string
  prompt: string
  disabled: boolean
  onMissed: 'run' | 'skip'
  nextRunAt: number | null
  lastRun: AgentScheduleRun | null
}

/** One execution of a schedule. */
export interface AgentScheduleRun {
  id: number
  startedAt: number
  reason: 'due' | 'catch_up' | 'manual'
  status: 'launched' | 'failed' | 'skipped'
  /** Earlier occurrences this run stands in for, 0 when it fired on time. */
  missed: number
  error: string
}

/**
 * A dry run of an unsaved edit: the next occurrences its cron produces. A bad
 * cron or template is reported in `cronError`/`promptError` rather than as a
 * failed call, so the editor can show it beside the field the user is still
 * typing in.
 */
export interface AgentSchedulePreview {
  next: number[]
  cronError: string
  promptError: string
}

/** One `schedules:` entry as the workspace editor writes it. */
export interface ScheduleEdit {
  id: string
  name: string
  cron: string
  prompt: string
  disabled: boolean
  onMissed: AgentSchedule['onMissed']
}

export interface SchedulePreviewRequest {
  workspace: string
  cron: string
  prompt: string
}

export interface AgentWorkspaceOpenResult {
  workspace: AgentWorkspace
  sessions: AgentSession[]
  missingMcps: string[]
  /** Enabled names skills.yml does not define, each saying why. */
  missingPackages: MissingSkillPackage[]
}

export interface StartSessionRequest {
  workspace: string
  name: string
  cols?: number
  rows?: number
}

export interface ResumeSessionRequest {
  id: string
  cols?: number
  rows?: number
}

/**
 * A control-plane failure carrying the core's own classification, mirroring
 * PopupTerminalRequestError so a caller can tell "no such workspace" from
 * "unavailable" without reading the message.
 */
export class AgentRequestError extends Error {
  constructor(
    message: string,
    readonly kind: string,
  ) {
    super(message)
    this.name = 'AgentRequestError'
  }
}

export interface AgentWorkspacesClient {
  workspaces(): Promise<AgentWorkspacesPayload>
  openWorkspace(dir: string): Promise<AgentWorkspaceOpenResult>
  createWorkspace(request: WorkspaceEditRequest): Promise<AgentWorkspace>
  updateWorkspace(request: WorkspaceEditRequest): Promise<AgentWorkspace>
  deleteWorkspace(dir: string): Promise<void>
  /** Launches the configured editor on the workspace directory, detached. */
  openWorkspaceInEditor(dir: string): Promise<void>
  /** Opens the workspace directory in the OS file manager. */
  revealWorkspace(dir: string): Promise<void>
  mcpCatalogue(): Promise<MCPCatalogueEntry[]>
  /** Imports pasted MCP JSON into mcps.yaml; returns the added ids and the refreshed catalogue. */
  importMCPServers(json: string): Promise<{ added: string[]; servers: MCPCatalogueEntry[] }>
  /** Removes a user-declared server from mcps.yaml; returns the refreshed catalogue. */
  removeMCPServer(id: string): Promise<MCPCatalogueEntry[]>
  skillPackages(): Promise<SkillPackagesPayload>
  /** Opens skills.yml, where packages are defined. */
  revealSkillPackages(): Promise<void>
  /** Opens the shared skills directory, where a skill a package selects is authored. */
  revealSharedSkills(): Promise<void>
  sessions(workspace: string): Promise<AgentSession[]>
  /** Polled while the area is active; '' spans every workspace. Omits a session with no live tmux session. */
  activity(workspace: string): Promise<AgentSessionActivity[]>
  /** Votes a size for a live session's pane; tmux answers with a window 'layout-changed' frame on the stream. */
  resizeSession(id: string, cols: number, rows: number): Promise<void>
  /** Sets a session's display name; the live terminal, if any, is untouched. */
  renameSession(id: string, name: string): Promise<void>
  /** Every session across every workspace, newest first in stable creation order. */
  allSessions(): Promise<AgentSession[]>
  startSession(request: StartSessionRequest): Promise<AgentSession>
  /** Opens the seeded Hive workspace on the interview that ends first run; detached, so route to it to attach. */
  startFirstRunChat(): Promise<AgentSession>
  resumeSession(request: ResumeSessionRequest): Promise<AgentSession>
  closeSession(id: string): Promise<{ closed: boolean }>
  deleteSession(id: string): Promise<void>
  /** One canvas by workspace and name; a name nothing was written under answers empty. */
  canvas(workspace: string, name: string): Promise<WorkspaceCanvas>
  /** An owner's canvases, most recently updated first — metadata only. */
  canvases(workspace: string): Promise<WorkspaceCanvasMeta[]>
  /** The owner key of every repository that holds a canvas. */
  canvasRepositories(): Promise<string[]>
  /** One canvas rendered as a standalone markdown document — the copy action. */
  canvasMarkdown(workspace: string, name: string): Promise<string>
  /** Write one canvas's markdown rendering to an absolute path from the save dialog. */
  exportCanvas(workspace: string, name: string, path: string): Promise<void>
  /** Fires a schedule now, outside its timetable; the cursor is untouched. */
  runSchedule(workspace: string, id: string): Promise<AgentScheduleRun>
  /** One schedule's run history, newest first. */
  scheduleRuns(workspace: string, id: string, limit: number): Promise<AgentScheduleRun[]>
  /** Validates an unsaved edit and reports what it would do. */
  previewSchedule(request: SchedulePreviewRequest): Promise<AgentSchedulePreview>
  /** The shared tmux stream a session's terminalId addresses (ADR agent-workspace-sessions-are-tmux-sessions). */
  openStream(name: string): WebSocket
}

/** Resolves the transport the webview was handed, or rejects with the reason. */
export async function getAgentsEndpoint(): Promise<AgentsEndpoint> {
  return await Endpoint()
}

export function createAgentWorkspacesClient(endpoint: AgentsEndpoint): AgentWorkspacesClient {
  async function post<T>(path: string, body: unknown): Promise<T | null> {
    const response = await fetch(`${endpoint.httpBaseURL}/api/terminal/agents${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${endpoint.token}` },
      body: JSON.stringify(body),
    })
    if (!response.ok) throw await failure(response)
    if (response.status === 204) return null
    return (await response.json()) as T
  }

  const { openStream } = createTerminalClient(endpoint as TerminalEndpoint)

  return {
    async workspaces() {
      const body = await post<AgentWorkspacesPayload>('/workspaces', {})
      if (!body)
        return {
          root: '',
          rootProblem: '',
          available: false,
          error: '',
          workspaces: [],
          presets: [],
          editor: { command: '', title: '' },
        }
      return {
        ...body,
        workspaces: (body.workspaces ?? []).map(normalizeWorkspace),
        presets: body.presets ?? [],
        editor: body.editor ?? { command: '', title: '' },
      }
    },
    async createWorkspace(request) {
      const body = await post<AgentWorkspace>('/workspaces/create', request)
      if (!body) throw new AgentRequestError('the workspace was not created', '')
      return normalizeWorkspace(body)
    },
    async updateWorkspace(request) {
      const body = await post<AgentWorkspace>('/workspaces/update', request)
      if (!body) throw new AgentRequestError('the workspace was not updated', '')
      return normalizeWorkspace(body)
    },
    async openWorkspace(dir) {
      const body = await post<AgentWorkspaceOpenResult>('/workspaces/open', { dir })
      if (!body) return { workspace: emptyWorkspace(dir), sessions: [], missingMcps: [], missingPackages: [] }
      return {
        workspace: normalizeWorkspace(body.workspace),
        sessions: body.sessions ?? [],
        missingMcps: body.missingMcps ?? [],
        missingPackages: body.missingPackages ?? [],
      }
    },
    async deleteWorkspace(dir) {
      await post('/workspaces/delete', { dir })
    },
    async openWorkspaceInEditor(dir) {
      await post('/workspaces/open-in-editor', { dir })
    },
    async revealWorkspace(dir) {
      await post('/workspaces/reveal', { dir })
    },
    async mcpCatalogue() {
      const body = await post<{ servers: MCPCatalogueEntry[] | null }>('/mcps', {})
      return body?.servers ?? []
    },
    async importMCPServers(json) {
      const body = await post<{ added: string[] | null; servers: MCPCatalogueEntry[] | null }>('/mcps/import', { json })
      return { added: body?.added ?? [], servers: body?.servers ?? [] }
    },
    async removeMCPServer(id) {
      const body = await post<{ servers: MCPCatalogueEntry[] | null }>('/mcps/remove', { id })
      return body?.servers ?? []
    },
    async skillPackages() {
      const body = await post<{ packages: SkillPackage[] | null; skills: SkillName[] | null; problem: string }>(
        '/skills',
        {},
      )
      return { packages: body?.packages ?? [], skills: body?.skills ?? [], problem: body?.problem ?? '' }
    },
    async revealSkillPackages() {
      await post('/skills/reveal', {})
    },
    async revealSharedSkills() {
      await post('/skills/shared', {})
    },
    async sessions(workspace) {
      const body = await post<{ sessions: AgentSession[] | null }>('/sessions', { workspace })
      return body?.sessions ?? []
    },
    async activity(workspace) {
      const body = await post<{ items: AgentSessionActivity[] | null }>('/sessions/activity', { workspace })
      return body?.items ?? []
    },
    async resizeSession(id, cols, rows) {
      await post('/sessions/resize', { id, cols, rows })
    },
    async renameSession(id, name) {
      await post('/sessions/rename', { id, name })
    },
    async allSessions() {
      const body = await post<{ sessions: AgentSession[] | null }>('/sessions/all', {})
      return body?.sessions ?? []
    },
    async startSession(request) {
      const body = await post<AgentSession>('/sessions/start', request)
      if (!body) throw new AgentRequestError('the session did not start', '')
      return body
    },
    async startFirstRunChat() {
      const body = await post<AgentSession>('/sessions/first-run', {})
      if (!body) throw new AgentRequestError('the session did not start', '')
      return body
    },
    async resumeSession(request) {
      const body = await post<AgentSession>('/sessions/resume', request)
      if (!body) throw new AgentRequestError('the session did not resume', '')
      return body
    },
    async closeSession(id) {
      const body = await post<{ closed: boolean }>('/sessions/close', { id })
      return { closed: !!body?.closed }
    },
    async deleteSession(id) {
      await post('/sessions/delete', { id })
    },
    async canvas(workspace, name) {
      const body = await post<WorkspaceCanvas>('/canvas', { workspace, name })
      if (!body) throw new AgentRequestError('the canvas could not be read', '')
      return { ...body, blocks: body.blocks ?? [] }
    },
    async canvases(workspace) {
      const body = await post<{ canvases: WorkspaceCanvasMeta[] | null }>('/canvases', { workspace })
      return body?.canvases ?? []
    },
    async canvasRepositories() {
      const body = await post<{ repositories: string[] | null }>('/canvas/repositories', {})
      return body?.repositories ?? []
    },
    async canvasMarkdown(workspace, name) {
      const body = await post<{ markdown: string }>('/canvas/markdown', { workspace, name })
      if (!body) throw new AgentRequestError('the canvas could not be rendered', '')
      return body.markdown
    },
    async exportCanvas(workspace, name, path) {
      await post('/canvas/export', { workspace, name, path })
    },
    async runSchedule(workspace, id) {
      const body = await post<{ run: AgentScheduleRun }>('/schedules/run', { workspace, id })
      if (!body?.run) throw new AgentRequestError('the schedule did not run', '')
      return body.run
    },
    async scheduleRuns(workspace, id, limit) {
      const body = await post<{ runs: AgentScheduleRun[] | null }>('/schedules/runs', { workspace, id, limit })
      return body?.runs ?? []
    },
    async previewSchedule(request) {
      const body = await post<AgentSchedulePreview>('/schedules/preview', request)
      if (!body) throw new AgentRequestError('the schedule could not be previewed', '')
      return { ...body, next: body.next ?? [] }
    },
    openStream,
  }
}

function emptyWorkspace(dir: string): AgentWorkspace {
  return { dir, name: '', command: '', danger: false, mcps: [], skills: [], schedules: [], problem: '', notice: '' }
}

// normalizeWorkspace guards against a null mcps, skills or schedules array on
// the wire: the Go side now always sends [], but this is the client boundary,
// so a template or composable can trust all three are iterable without its own
// null check regardless.
function normalizeWorkspace(w: AgentWorkspace): AgentWorkspace {
  return { ...w, mcps: w.mcps ?? [], skills: w.skills ?? [], schedules: w.schedules ?? [] }
}

async function failure(response: Response): Promise<AgentRequestError> {
  try {
    const body = (await response.json()) as { message?: string; kind?: string }
    if (body?.message) return new AgentRequestError(body.message, body.kind ?? '')
  } catch {
    // fall through to the status line
  }
  return new AgentRequestError(`${response.status} ${response.statusText}`.trim(), '')
}
