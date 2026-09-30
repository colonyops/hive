import { ref, shallowRef, type Ref, type ShallowRef } from 'vue'
import {
  createAgentWorkspacesClient,
  getAgentsEndpoint,
  type AgentEditor,
  type AgentPreset,
  type AgentSession,
  type AgentWorkspace,
  type AgentWorkspaceOpenResult,
  type AgentWorkspacesClient,
  type MCPCatalogueEntry,
  type MissingSkillPackage,
  type ResumeSessionRequest,
  type SkillName,
  type SkillPackage,
  type StartSessionRequest,
  type WorkspaceEditRequest,
} from '../lib/agentWorkspacesClient'
import { Available as AgentsAvailable } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/agentsservice'

// Module singletons, mirroring useTerminalSessions.ts's shape: the rows are
// the area's workspace/session sets, not one view's, so the lists render the
// last-known rows while a reload revalidates rather than emptying and popping
// back in. `loaded` is distinct from `loading` for the same reason it is
// there — false both before the first load and after it lands, and the list
// needs the difference to tell an empty result from one it has not read yet.

const checking = ref(true)
const available = ref(false)
const reason = ref('')
const client: ShallowRef<AgentWorkspacesClient | null> = shallowRef(null)
let probe: Promise<void> | null = null

const workspaces = ref<AgentWorkspace[]>([])
const workspacesLoading = ref(false)
const workspacesLoaded = ref(false)
const workspacesError = ref<string | null>(null)
const root = ref('')
const editor = ref<AgentEditor>({ command: '', title: '' })
const presets = ref<AgentPreset[]>([])
const mcpCatalogue = ref<MCPCatalogueEntry[]>([])
const skillPackages = ref<SkillPackage[]>([])
const skillNames = ref<SkillName[]>([])
const skillPackagesProblem = ref('')
// rootProblem is the one signal from the workspaces payload this composable
// tracks separately from the top-level available/reason: it is the
// configured root path itself being unreachable (spec §14), a distinct axis
// from payload.available/error, which repeats the same ptyterm build/platform
// check AgentsService.Available() already gates the whole mode on.
const rootProblem = ref('')

const missingMCPs = ref<string[]>([])
const missingPackages = ref<MissingSkillPackage[]>([])

// The availability answer and the transport are resolved once per run: like
// the pop-up terminal's probe, there is no program to install and nothing
// about either can change while the app is running.
function ensureProbed(): Promise<void> {
  probe ??= (async () => {
    try {
      const availability = await AgentsAvailable()
      available.value = availability.available
      reason.value = availability.reason
      if (availability.available) client.value = createAgentWorkspacesClient(await getAgentsEndpoint())
    } catch (error) {
      available.value = false
      reason.value = error instanceof Error ? error.message : 'The Agents area is unavailable.'
    } finally {
      checking.value = false
    }
  })()
  return probe
}

async function reloadWorkspaces(): Promise<void> {
  await ensureProbed()
  if (!client.value) return
  workspacesLoading.value = true
  workspacesError.value = null
  try {
    const payload = await client.value.workspaces()
    root.value = payload.root
    editor.value = payload.editor
    presets.value = payload.presets
    rootProblem.value = payload.rootProblem
    workspaces.value = payload.workspaces
  } catch (e) {
    // Keep the last-good rows: a failed revalidation reports itself without
    // collapsing the list it could not refresh.
    workspacesError.value = e instanceof Error && e.message ? e.message : 'Could not list workspaces.'
  } finally {
    workspacesLoading.value = false
    workspacesLoaded.value = true
  }
}

// Regenerates dir's disposable artifacts and records what the open reported:
// the workspace's fresh view (folded into the list rather than requiring a
// second round trip) and its missing MCPs. The session rows themselves come
// from the cross-workspace list (useAgentSessionsAll), which the sidebar
// filters — this composable keeps no per-workspace session list. A failed
// open keeps the last-good state; the workspace row's own problem field is
// where a broken manifest reports itself.
async function openWorkspace(dir: string): Promise<void> {
  const result = await regenerateWorkspace(dir)
  if (!result) return
  missingMCPs.value = result.missingMcps
  missingPackages.value = result.missingPackages
}

// regenerateWorkspace re-syncs a workspace's generated files and folds its
// fresh view into the list, without touching the focused workspace's
// missing-MCP state — the save path for a workspace that is not currently
// selected. Returns null on failure, keeping the last-good rows.
async function regenerateWorkspace(dir: string): Promise<AgentWorkspaceOpenResult | null> {
  await ensureProbed()
  if (!client.value) return null
  try {
    const result = await client.value.openWorkspace(dir)
    const idx = workspaces.value.findIndex((w) => w.dir === dir)
    if (idx >= 0) workspaces.value = [...workspaces.value.slice(0, idx), result.workspace, ...workspaces.value.slice(idx + 1)]
    return result
  } catch {
    // Keep the last-good rows, matching the reload functions above.
    return null
  }
}

async function deleteWorkspace(dir: string): Promise<void> {
  if (!client.value) return
  await client.value.deleteWorkspace(dir)
  workspaces.value = workspaces.value.filter((w) => w.dir !== dir)
  void reloadWorkspaces()
}

// Create/update fold the returned view straight into the list the same way
// openWorkspace does, then revalidate in the background — the row is correct
// immediately without waiting on a second round trip.
async function createWorkspace(request: WorkspaceEditRequest): Promise<AgentWorkspace> {
  if (!client.value) throw new Error('The Agents area is unavailable.')
  const view = await client.value.createWorkspace(request)
  workspaces.value = [...workspaces.value.filter((w) => w.dir !== view.dir), view]
  void reloadWorkspaces()
  return view
}

async function updateWorkspace(request: WorkspaceEditRequest): Promise<AgentWorkspace> {
  if (!client.value) throw new Error('The Agents area is unavailable.')
  const view = await client.value.updateWorkspace(request)
  const idx = workspaces.value.findIndex((w) => w.dir === view.dir)
  workspaces.value = idx >= 0
    ? [...workspaces.value.slice(0, idx), view, ...workspaces.value.slice(idx + 1)]
    : [...workspaces.value, view]
  return view
}

// The MCP catalogue is loaded on demand — the workspace editor is its only
// reader — and refreshed in place by an import or removal, whose responses
// carry the merged set so no second round trip is needed.
async function reloadMCPCatalogue(): Promise<void> {
  await ensureProbed()
  if (!client.value) return
  try {
    mcpCatalogue.value = await client.value.mcpCatalogue()
  } catch {
    // Keep the last-good rows, matching the reload functions above.
  }
}

async function importMCPServers(json: string): Promise<string[]> {
  if (!client.value) throw new Error('The Agents area is unavailable.')
  const result = await client.value.importMCPServers(json)
  mcpCatalogue.value = result.servers
  return result.added
}

async function removeMCPServer(id: string): Promise<void> {
  if (!client.value) throw new Error('The Agents area is unavailable.')
  mcpCatalogue.value = await client.value.removeMCPServer(id)
}

// The package catalogue loads on demand beside the MCP one, and for the same
// reason: the workspace editor is its only reader. It is re-read rather than
// cached across opens because both halves — skills.yml and the shared skills
// directory — are files the user can change under the app.
async function reloadSkillPackages(): Promise<void> {
  await ensureProbed()
  if (!client.value) return
  try {
    const payload = await client.value.skillPackages()
    skillPackages.value = payload.packages
    skillNames.value = payload.skills
    skillPackagesProblem.value = payload.problem
  } catch {
    // Keep the last-good rows, matching the reload functions above.
  }
}

async function revealSkillPackages(): Promise<void> {
  if (!client.value) throw new Error('The Agents area is unavailable.')
  await client.value.revealSkillPackages()
}

async function revealSharedSkills(): Promise<void> {
  if (!client.value) throw new Error('The Agents area is unavailable.')
  await client.value.revealSharedSkills()
}

async function openWorkspaceInEditor(dir: string): Promise<void> {
  if (!client.value) throw new Error('The Agents area is unavailable.')
  await client.value.openWorkspaceInEditor(dir)
}

async function revealWorkspace(dir: string): Promise<void> {
  if (!client.value) throw new Error('The Agents area is unavailable.')
  await client.value.revealWorkspace(dir)
}

async function startSession(request: StartSessionRequest): Promise<AgentSession> {
  if (!client.value) throw new Error('The Agents area is unavailable.')
  return await client.value.startSession(request)
}

// Probes first: first run calls this before the Agents area has ever been
// opened, so unlike the other launches nothing has resolved the transport yet.
async function startFirstRunChat(): Promise<AgentSession> {
  await ensureProbed()
  if (!client.value) throw new Error(reason.value || 'The Agents area is unavailable.')
  return await client.value.startFirstRunChat()
}

async function resumeSession(request: ResumeSessionRequest): Promise<AgentSession> {
  if (!client.value) throw new Error('The Agents area is unavailable.')
  return await client.value.resumeSession(request)
}

async function closeSession(id: number): Promise<boolean> {
  if (!client.value) return false
  const result = await client.value.closeSession(id)
  return result.closed
}

async function renameSession(id: number, name: string): Promise<void> {
  if (!client.value) throw new Error('The Agents area is unavailable.')
  await client.value.renameSession(id, name)
}

async function deleteSession(id: number): Promise<void> {
  if (!client.value) return
  await client.value.deleteSession(id)
}

/** Clears what openWorkspace recorded — called when the focus changes. */
function resetOpenWorkspace(): void {
  missingMCPs.value = []
  missingPackages.value = []
}

export function useAgentWorkspaces(): {
  checking: Ref<boolean>
  available: Ref<boolean>
  reason: Ref<string>
  client: ShallowRef<AgentWorkspacesClient | null>
  workspaces: Ref<AgentWorkspace[]>
  workspacesLoading: Ref<boolean>
  workspacesLoaded: Ref<boolean>
  workspacesError: Ref<string | null>
  root: Ref<string>
  rootProblem: Ref<string>
  editor: Ref<AgentEditor>
  presets: Ref<AgentPreset[]>
  mcpCatalogue: Ref<MCPCatalogueEntry[]>
  skillPackages: Ref<SkillPackage[]>
  skillNames: Ref<SkillName[]>
  skillPackagesProblem: Ref<string>
  missingMCPs: Ref<string[]>
  missingPackages: Ref<MissingSkillPackage[]>
  ready: () => Promise<void>
  reloadWorkspaces: () => Promise<void>
  openWorkspace: (dir: string) => Promise<void>
  regenerateWorkspace: (dir: string) => Promise<AgentWorkspaceOpenResult | null>
  createWorkspace: (request: WorkspaceEditRequest) => Promise<AgentWorkspace>
  updateWorkspace: (request: WorkspaceEditRequest) => Promise<AgentWorkspace>
  deleteWorkspace: (dir: string) => Promise<void>
  reloadMCPCatalogue: () => Promise<void>
  importMCPServers: (json: string) => Promise<string[]>
  removeMCPServer: (id: string) => Promise<void>
  reloadSkillPackages: () => Promise<void>
  revealSkillPackages: () => Promise<void>
  revealSharedSkills: () => Promise<void>
  openWorkspaceInEditor: (dir: string) => Promise<void>
  revealWorkspace: (dir: string) => Promise<void>
  startSession: (request: StartSessionRequest) => Promise<AgentSession>
  startFirstRunChat: () => Promise<AgentSession>
  resumeSession: (request: ResumeSessionRequest) => Promise<AgentSession>
  closeSession: (id: number) => Promise<boolean>
  renameSession: (id: number, name: string) => Promise<void>
  deleteSession: (id: number) => Promise<void>
  resetOpenWorkspace: () => void
} {
  return {
    checking, available, reason, client,
    workspaces, workspacesLoading, workspacesLoaded, workspacesError,
    root, rootProblem, editor, presets, mcpCatalogue,
    skillPackages, skillNames, skillPackagesProblem, missingMCPs, missingPackages,
    ready: ensureProbed,
    reloadWorkspaces, openWorkspace, regenerateWorkspace,
    createWorkspace, updateWorkspace, deleteWorkspace,
    reloadMCPCatalogue, importMCPServers, removeMCPServer,
    reloadSkillPackages, revealSkillPackages, revealSharedSkills,
    openWorkspaceInEditor, revealWorkspace,
    startSession, startFirstRunChat, resumeSession, closeSession, renameSession, deleteSession, resetOpenWorkspace,
  }
}

export function resetAgentWorkspacesForTests(): void {
  checking.value = true
  available.value = false
  reason.value = ''
  client.value = null
  probe = null
  workspaces.value = []
  workspacesLoading.value = false
  workspacesLoaded.value = false
  workspacesError.value = null
  root.value = ''
  rootProblem.value = ''
  editor.value = { command: '', title: '' }
  presets.value = []
  mcpCatalogue.value = []
  skillPackages.value = []
  skillNames.value = []
  skillPackagesProblem.value = ''
  missingMCPs.value = []
  missingPackages.value = []
}
