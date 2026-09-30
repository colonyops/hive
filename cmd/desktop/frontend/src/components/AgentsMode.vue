<script setup lang="ts">
// The Chats area: AgentsSidebar's tree (workspaces, each holding its own
// chats) beside a pane the terminal owns under a slim status bar naming the
// open session's workspace. Focusing a workspace is neither a filter nor a
// container — it scopes what the strips below describe and what a new chat
// defaults to, and changing it never tears down a live pane nor hides another
// workspace's chats. The one pane the shell itself draws is the zero
// state: no PTY exists yet, so it says what a chat is and offers to start one,
// which NewChatDialog then asks for. What is borrowed from
// TerminalMode.vue is narrower — the aside/main split, plus (since ADR agent-workspace-sessions-are-tmux-sessions)
// the pane's xterm wiring itself: a session is a tmux session, addressed and
// framed exactly like a hive one, just not discovered through hive.
import { computed, markRaw, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDocumentVisibility } from '@vueuse/core'
import { Browser } from '@wailsio/runtime'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal, type IDisposable, type ILinkHandler } from '@xterm/xterm'
import IconArrowDown from '~icons/lucide/arrow-down'
import IconMessagesSquare from '~icons/lucide/messages-square'
import IconLoaderCircle from '~icons/lucide/loader-circle'
import AgentCanvasPane from './AgentCanvasPane.vue'
import AgentsSidebar from './AgentsSidebar.vue'
import AgentWorkspaceEditor from './AgentWorkspaceEditor.vue'
import BaseButton from './BaseButton.vue'
import ChatRenameDialog from './ChatRenameDialog.vue'
import NewChatDialog from './NewChatDialog.vue'
import PaneStatusBar from './PaneStatusBar.vue'
import { useAgentWorkspaces } from '../composables/useAgentWorkspaces'
import { useAgentSessionsAll } from '../composables/useAgentSessionsAll'
import { useTerminalFont } from '../composables/useTerminalFont'
import { useTheme } from '../composables/useTheme'
import { useAgentCanvasRoute } from '../composables/useAgentCanvasRoute'
import { useWailsEvent } from '../composables/useWailsEvent'
import { xtermTheme } from '../lib/terminalTheme'
import { decodeFrame, encodeInputFrames, encodePasteFrames } from '../lib/agentWorkspacesClient'
import { loadTerminalFaces, terminalFontStack } from '../lib/terminalFaces'
import { claimAtlasRenderer } from '../lib/terminalRenderer'
import { setAgentsTreeHandles } from '../lib/agentsTree'
import { isEditableTarget } from '../lib/isEditableTarget'
import { installTerminalImages } from '../lib/terminalImages'
import { pasteTerminalImage } from '../lib/terminalImagesClient'
import { silenceDeviceReports } from '../lib/terminalReports'
import { watchTailPin } from '../lib/terminalTail'
import type { AgentSession, AgentWorkspace, WorkspaceEditRequest } from '../lib/agentWorkspacesClient'
import '@xterm/xterm/css/xterm.css'

/** Poll period for the M2 approval indicator (hc-ou4o02zx §5), while active. */
const ACTIVITY_POLL_MS = 2000

const props = defineProps<{ active?: boolean; sidebarCollapsed?: boolean }>()

const {
  checking, available, reason,
  workspaces, root, editor, missingMCPs, missingPackages,
  client, ready,
  openWorkspaceInEditor, revealWorkspace,
  reloadWorkspaces, openWorkspace, regenerateWorkspace, deleteWorkspace,
  createWorkspace, updateWorkspace,
  startSession, resumeSession, closeSession, renameSession, deleteSession, resetOpenWorkspace,
} = useAgentWorkspaces()
const { recents, reloadRecents } = useAgentSessionsAll()

// An enabled name skills.yml does not define has two causes that produce the
// same bare warning: a manifest written before packages were the enablement
// unit enumerates skill slugs, and a typo names nothing at all. Only the
// first has a fix on screen — a package that already selects that skill — so
// the two are reported as separate lines (#307).
const missingSkillNames = computed(() => missingPackages.value.filter((entry) => entry.skill))
const unknownPackageNames = computed(() => missingPackages.value.filter((entry) => !entry.skill).map((entry) => entry.name))

const missingSkillsNotice = computed(() => {
  const entries = missingSkillNames.value
  if (!entries.length) return ''
  const names = entries.map((entry) => entry.name).join(', ')
  const subject = entries.length === 1 ? 'is a skill, not a package' : 'are skills, not packages'
  const packages = [...new Set(entries.flatMap((entry) => entry.selectedBy))].sort()
  if (!packages.length) {
    return `${names} ${subject}, and no package selects ${entries.length === 1 ? 'it' : 'them'} — define one in skills.yml, then enable it here.`
  }
  const carrier = packages.length === 1 ? `the ${packages[0]} package selects` : `the ${packages.join(' and ')} packages select`
  return `${names} ${subject} — ${carrier} ${entries.length === 1 ? 'it' : 'them'}. Enable that instead in the workspace editor.`
})

const {
  px: fontSizePx, family: fontFamily, weight: fontWeight, weightBold: fontWeightBold, lineHeight, letterSpacing,
} = useTerminalFont()
const { theme } = useTheme()

// The session pane's state lives here, ahead of the route watcher below: that
// watcher's immediate call reaches into teardownPane on first run, and a
// `let`/`const` referenced before its own declaration line has executed is a
// ReferenceError even though the function that closes over it is hoisted.
const RESIZE_DEBOUNCE_MS = 80

const paneHost = ref<HTMLElement | null>(null)
const term = shallowRef<Terminal | null>(null)
const openSessionId = ref<number | null>(null)
const paneStatus = ref<'idle' | 'opening' | 'live'>('idle')
const paneError = ref('')
const paneWorkspaceDir = ref('')
const paneActionError = ref('')
const paneScrolledUp = ref(false)

let socket: WebSocket | null = null
let fit: FitAddon | null = null
let observer: ResizeObserver | null = null
let resizeTimer: ReturnType<typeof setTimeout> | undefined
let rendered = false
const disposers: IDisposable[] = []
// The tmux wire is windowed (ADR agent-workspace-sessions-are-tmux-sessions): every frame in and out of the pane's
// socket names the window it belongs to, so the pane has to know which one is
// its own for the life of one attach.
let paneWindowId = ''
let panePaneId = ''

const paneLaidOut = computed(() => paneStatus.value === 'opening' || term.value !== null)
const paneWorkspaceName = computed(() =>
  workspaces.value.find((ws) => ws.dir === paneWorkspaceDir.value)?.name || paneWorkspaceDir.value)

// ── Route-driven workspace selection ────────────────────────────────────────
const route = useRoute()
const router = useRouter()
const {
  routeChatId,
  canvasRequested,
  canvasVisible,
  canvasName,
  syncCanvasQuery,
  noteCanvasWrite,
  clearCanvasUnseen,
} = useAgentCanvasRoute()
const selectedWorkspace = computed(() =>
  (route.name === 'agents' && typeof route.params.workspace === 'string' ? route.params.workspace : ''))

function selectWorkspace(dir: string): void {
  if (dir === selectedWorkspace.value) return
  // The query rides along: the focused workspace and the open chat (?chat) are
  // independent axes, and moving one must not drop the other.
  if (!dir) {
    void router.push({ name: 'agents', query: route.query })
    return
  }
  void router.push({ name: 'agents', params: { workspace: dir }, query: route.query })
}

watch(() => props.active, (active) => {
  if (!active) return
  void loadWorkspaces()
}, { immediate: true })

async function loadWorkspaces(): Promise<void> {
  await ready()
  if (available.value) void reloadWorkspaces()
}

// Focus is what regenerates a workspace's artifacts and swaps its missing-MCP
// state in. A live pane keeps running across a change — the open session need
// not belong to the focused workspace. The sidebar only ever moves the focus,
// so the empty case here comes from the route or from deleting the workspace
// that held it.
watch(selectedWorkspace, async (dir, previous) => {
  if (dir === previous) return
  resetOpenWorkspace()
  if (!dir) return
  await openWorkspace(dir)
}, { immediate: true })

// ── Activity indicators (hc-ou4o02zx) ────────────────────────────────────────
// Polled while the area is active, across every workspace — the sidebar shows
// every session at once, the way the Code view's status poll covers its whole
// tree. Off-screen, nothing polls: capture-pane is real tmux work per live
// session.
const sessionActivity = ref<Record<number, string>>({})
let activityTimer: ReturnType<typeof setTimeout> | undefined
let activityGeneration = 0

async function pollActivity(generation: number): Promise<void> {
  if (client.value) {
    try {
      const items = await client.value.activity('')
      if (generation !== activityGeneration) return
      sessionActivity.value = Object.fromEntries(items.map((item) => [item.id, item.status]))
    } catch {
      // A transient tmux probe failure must not erase the last dots shown.
    }
  }
  if (generation !== activityGeneration) return
  activityTimer = setTimeout(() => { void pollActivity(generation) }, ACTIVITY_POLL_MS)
}

function haltActivityPolling(): void {
  ++activityGeneration
  clearTimeout(activityTimer)
  activityTimer = undefined
}

function stopActivityPolling(): void {
  haltActivityPolling()
  sessionActivity.value = {}
}

// A hidden window is off-screen too: the poll parks, and the dots it last drew
// stay up for the moment the window shows again.
const visibility = useDocumentVisibility()

watch([() => props.active, visibility], ([active, state]) => {
  if (!active) {
    stopActivityPolling()
    return
  }
  haltActivityPolling()
  if (state === 'visible') {
    const generation = ++activityGeneration
    void pollActivity(generation)
  }
}, { immediate: true })

// ── New chat ─────────────────────────────────────────────────────────────────
// The sidebar's + and the idle pane's button both open the same dialog,
// prefilled with the focused workspace, so the pane never doubles as a form.
// Launching stays the pane's: the dialog closes on submit and the opening
// overlay — then the zero state, if it fails — is what reports it.
const newSessionOpen = ref(false)
const startingSession = ref(false)

const defaultWorkspaceDir = computed(() => selectedWorkspace.value || recents.value[0]?.workspace || workspaces.value[0]?.dir || '')

const DEFAULT_CHAT_NAME = 'New Chat'

function handleNewSessionRequest(): void {
  newSessionOpen.value = true
}

// The sidebar's per-workspace +. It names the workspace, which is the only
// thing the dialog asks that has no sensible default, so there is nothing left
// to ask: the chat takes the default name and launches straight into the pane.
// Renaming it is one entry away on its own row menu.
function handleStartSessionIn(workspace: string): void {
  void startNewSession(workspace, DEFAULT_CHAT_NAME)
}

async function submitNewSession(input: { workspace: string; name: string }): Promise<void> {
  newSessionOpen.value = false
  await startNewSession(input.workspace, input.name || DEFAULT_CHAT_NAME)
}

// Focus follows a new session, which also unfolds the workspace its row lands
// in (AgentsSidebar watches the prop).
async function startNewSession(workspace: string, name: string): Promise<void> {
  if (startingSession.value) return
  startingSession.value = true
  try {
    selectWorkspace(workspace)
    await launchIntoPane(workspace, (size) => startSession({ workspace, name, ...size }))
    void reloadRecents()
  } finally {
    startingSession.value = false
  }
}

async function resumeRow(session: AgentSession): Promise<void> {
  await launchIntoPane(session.workspace, (size) => resumeSession({ id: session.id, ...size }))
}

// ── The open chat rides the route (?chat, ADR the-open-chat-rides-the-route) ──────────────────────────
// A reload keeps the hash and the toggle's remembered path keeps the query,
// so the route names the pane's session the way /terminal names its surface,
// and coming back to the area reattaches it. Only a chat whose tmux session
// the listing reports live is auto-resumed: ResumeSession *relaunches* a dead
// one, and a relaunch must stay a deliberate click on the row, never a side
// effect of a reload. The beat between listing and resuming is an accepted
// race.
function syncChatQuery(id: number | null): void {
  if (route.name !== 'agents') return
  const next = id === null ? undefined : String(id)
  if ((typeof route.query.chat === 'string' ? route.query.chat : undefined) === next) return
  // A closed chat takes its canvas along: ?canvas names a view of the open
  // chat, so it must not linger and reopen against whatever comes next.
  const canvas = next === undefined ? undefined : route.query.canvas
  void router.replace({ name: 'agents', params: route.params, query: { ...route.query, chat: next, canvas } })
}

watch([paneStatus, openSessionId], ([status, id]) => {
  if (status === 'live' && id !== null) syncChatQuery(id)
  else if (status === 'idle') syncChatQuery(null)
})

// A route naming a chat other than the open one is a request to switch. A
// reload with a stale ?chat is the same shape. Only an in-flight launch is left
// alone, so two attaches never race for the pane.
watch([routeChatId, () => props.active], ([id, active]) => {
  if (id === null || !active) return
  if (openSessionId.value === id || paneStatus.value === 'opening') return
  void resumeChatFromRoute(id)
}, { immediate: true })

// ── The canvas pane rides the route too (?canvas[=name]) ────────────────────
// The query itself is owned by useAgentCanvasRoute, shared with the title bar's
// right-panel toggle. This view keeps the two Wails listeners, which are its
// own: both are answers about the chat it has on screen.
useWailsEvent('canvas:updated', (event) => {
  const payload = Array.isArray(event.data) ? event.data[0] : event.data
  noteCanvasWrite(Number(payload))
})
// Viewing a chat with its pane open clears its dot.
watch([canvasVisible, routeChatId], ([visible, id]) => {
  if (visible && id !== null) clearCanvasUnseen(id)
})

// An agent can ask to open or close the pane (open_canvas / close_canvas).
// Honored only for the chat in view: an agent must never drag the user away
// from a different chat — its write already lights the unseen dot there.
useWailsEvent('canvas:toggle', (event) => {
  const payload = (Array.isArray(event.data) ? event.data[0] : event.data) as
    { session: number; name: string; open: boolean } | undefined
  if (!payload || Number(payload.session) !== routeChatId.value) return
  syncCanvasQuery(payload.open, payload.name || undefined)
})

async function resumeChatFromRoute(id: number): Promise<void> {
  await ready()
  if (!available.value) return
  await reloadRecents()
  if (routeChatId.value !== id || openSessionId.value === id || paneStatus.value === 'opening') return
  const session = recents.value.find((row) => row.id === id)
  if (!session?.terminalId) {
    // The route named a chat that cannot be attached, so the query goes back
    // to naming whatever the pane actually holds. Clearing it outright would
    // take a live chat's ?canvas down with it.
    syncChatQuery(paneStatus.value === 'live' ? openSessionId.value : null)
    return
  }
  await resumeRow(session)
}

// ── Sidebar event wiring (AgentsSidebar.vue) ─────────────────────────────────
// The tree spans every workspace, so resuming from it can reach a chat outside
// whatever is currently focused; every mutation reloads the cross-workspace
// list afterward so its rows and dots stay live. Chats keep stable creation
// order inside their workspace — a resume touches last_opened_at without
// moving anything.
async function handleSidebarSelectSession(session: AgentSession): Promise<void> {
  // openSessionId alone would strand a failed attach (id set, pane idle) with no retry click.
  if (paneStatus.value === 'live' && openSessionId.value === session.id) return
  await resumeRow(session)
  void reloadRecents()
}

// A schedule that fires starts a chat nobody clicked for, so the tree only
// learns about it from this wake-up: every other reload here hangs off a user
// action. The workspace list is re-read with it because each workspace carries
// its schedules, and the header tooltip's next run has just moved.
useWailsEvent('schedules:updated', () => {
  void reloadRecents()
  void reloadWorkspaces()
})

// ── Workspace editor (DrawerSheet, like every other editor) ──────────────────
// Create, edit, and delete: the editor is the workspace's whole management
// surface, so the sidebar rows carry no menu of their own.
const workspaceEditorOpen = ref(false)
const editingWorkspace = ref<AgentWorkspace | null>(null)
const workspaceEditorBusy = ref(false)
const workspaceEditorError = ref('')

function openCreateWorkspace(): void {
  editingWorkspace.value = null
  workspaceEditorError.value = ''
  workspaceEditorOpen.value = true
}

function openEditWorkspace(workspace: AgentWorkspace): void {
  editingWorkspace.value = workspace
  workspaceEditorError.value = ''
  workspaceEditorOpen.value = true
}

async function saveWorkspace(request: WorkspaceEditRequest): Promise<void> {
  workspaceEditorBusy.value = true
  workspaceEditorError.value = ''
  try {
    if (editingWorkspace.value) {
      await updateWorkspace(request)
      // Saving re-syncs the workspace's generated files immediately — a
      // changed mcps list only reaches .mcp.json through a regenerate, and
      // neither the focus watcher (already selected) nor anything else
      // (not selected) would fire one. The selected path also refreshes the
      // missing-MCP banner; the unselected one must not touch it.
      if (request.dir === selectedWorkspace.value) void openWorkspace(request.dir)
      else void regenerateWorkspace(request.dir)
    } else {
      await createWorkspace(request)
      selectWorkspace(request.dir)
    }
    workspaceEditorOpen.value = false
  } catch (failure) {
    workspaceEditorError.value = failure instanceof Error ? failure.message : 'The workspace could not be saved.'
  } finally {
    workspaceEditorBusy.value = false
  }
}

// The delete arrives from the editor's own confirm strip, so a failure
// reports back into that strip (the editor stays open) rather than vanishing
// with a closed sheet.
async function deleteWorkspaceFromEditor(dir: string): Promise<void> {
  workspaceEditorBusy.value = true
  workspaceEditorError.value = ''
  try {
    await deleteWorkspace(dir)
    workspaceEditorOpen.value = false
    if (dir === selectedWorkspace.value) selectWorkspace('')
    void reloadRecents()
  } catch (failure) {
    workspaceEditorError.value = failure instanceof Error ? failure.message : 'The workspace could not be deleted.'
  } finally {
    workspaceEditorBusy.value = false
  }
}

// ── Chat rename (ChatRenameDialog.vue) ───────────────────────────────────────
const renamingSession = ref<AgentSession | null>(null)
const renameBusy = ref(false)
const renameError = ref('')

function openRenameSession(session: AgentSession): void {
  renameError.value = ''
  renamingSession.value = session
}

async function renameSessionTo(id: number, name: string): Promise<void> {
  await renameSession(id, name)
  void reloadRecents()
}

async function saveSessionRename(name: string): Promise<void> {
  if (!renamingSession.value || renameBusy.value) return
  renameBusy.value = true
  renameError.value = ''
  try {
    await renameSessionTo(renamingSession.value.id, name)
    renamingSession.value = null
  } catch (failure) {
    renameError.value = failure instanceof Error ? failure.message : 'The chat could not be renamed.'
  } finally {
    renameBusy.value = false
  }
}

// The sidebar's inline editor, not the dialog: by the time an edit commits,
// the row being renamed is the open chat, so a failure surfaces on the pane
// status bar rather than on ChatRenameDialog, which is not open.
async function commitSessionRename(session: AgentSession, name: string): Promise<void> {
  paneActionError.value = ''
  try {
    await renameSessionTo(session.id, name)
  } catch (failure) {
    paneActionError.value = failure instanceof Error ? failure.message : 'The chat could not be renamed.'
  }
}

async function closeRow(session: AgentSession): Promise<void> {
  if (openSessionId.value === session.id) {
    teardownPane()
    paneStatus.value = 'idle'
    openSessionId.value = null
  }
  await closeSession(session.id)
  void reloadRecents()
}

async function removeRow(session: AgentSession): Promise<void> {
  if (openSessionId.value === session.id) {
    teardownPane()
    paneStatus.value = 'idle'
    openSessionId.value = null
  }
  await deleteSession(session.id)
  void reloadRecents()
}

// ── Pane status bar ──────────────────────────────────────────────────────────
// Names the workspace the pane's session belongs to — which the focused one
// need not match — and opens its directory outside the app, reusing the
// workspace editor's control-plane calls.
async function openPaneWorkspaceInEditor(): Promise<void> {
  paneActionError.value = ''
  try {
    await openWorkspaceInEditor(paneWorkspaceDir.value)
  } catch (failure) {
    paneActionError.value = failure instanceof Error ? failure.message : 'The editor could not be opened.'
  }
}

async function revealPaneWorkspace(): Promise<void> {
  paneActionError.value = ''
  try {
    await revealWorkspace(paneWorkspaceDir.value)
  } catch (failure) {
    paneActionError.value = failure instanceof Error ? failure.message : 'The directory could not be opened.'
  }
}

// ── The session pane ─────────────────────────────────────────────────────────
// Mirrors PopupTerminal.vue's xterm wiring over the same wire protocol
// (internal/adapter/httpapi/pty_stream.go); only the launch call and the
// control-plane base differ, per the shared client this composable resolves.

async function launchIntoPane(workspace: string, action: (size: { cols?: number; rows?: number }) => Promise<AgentSession>): Promise<void> {
  if (!client.value || paneStatus.value === 'opening') return
  teardownPane()
  openSessionId.value = null
  paneWorkspaceDir.value = workspace
  paneActionError.value = ''
  // The status bar mounts with the 'opening' flip, ahead of the nextTick that
  // precedes measurePane — so the grid is measured with the bar's height
  // already taken and the attach needs no corrective resize vote.
  paneStatus.value = 'opening'
  paneError.value = ''
  try {
    await loadTerminalFaces(fontFamily.value, fontSizePx.value, fontWeight.value, fontWeightBold.value)
    await nextTick()
    const created = buildPane()
    if (!created) {
      paneError.value = 'The session pane could not be rendered.'
      paneStatus.value = 'idle'
      return
    }
    const size = measurePane()
    const result = await action(size ?? {})
    openSessionId.value = result.id
    if (!result.terminalId || !result.windowId) {
      // The session died before it could be attached to. Nothing is left to
      // render, and the launch's own notice is the only account of why: the
      // listing this row is redrawn from reports no notice at all, so
      // dropping it here is what made an unlaunchable agent look like a click
      // that did nothing. teardownPane clears paneError, so it is set after.
      teardownPane()
      paneStatus.value = 'idle'
      paneError.value = result.notice || 'The session exited before it could be opened.'
      return
    }
    // The grid opens at the size tmux granted at attach — not the size this
    // launch voted, which tmux's window-size option may have overruled — and
    // from here window 'layout-changed' frames are what change it (the Code view's
    // rule: the pane renders tmux's grid, never its own fit). Opening at any
    // other size tears the TUI's cursor-addressed redraws.
    if (result.cols && result.rows) created.resize(result.cols, result.rows)
    else if (size) created.resize(size.cols, size.rows)
    // The launch already voted this measurement; seeding the dedup keeps the
    // observer's first fire from re-casting it.
    lastVote = size ?? null
    attachStream(created, result.terminalId, result.windowId, result.paneId)
    paneStatus.value = 'live'
    // A launch that lands while the user is typing elsewhere must not steal
    // that focus -- the sidebar's inline rename can be one keystroke into a
    // field opened by the same double click that started this launch.
    if (!isEditableTarget(document.activeElement)) created.focus()
  } catch (failure) {
    teardownPane()
    paneStatus.value = 'idle'
    paneError.value = failure instanceof Error ? failure.message : 'The session could not be opened.'
  }
}

function buildPane(): Terminal | null {
  if (!paneHost.value) return null
  const created = markRaw(new Terminal({
    fontFamily: terminalFontStack(fontFamily.value),
    fontSize: fontSizePx.value,
    fontWeight: fontWeight.value,
    fontWeightBold: fontWeightBold.value,
    lineHeight: lineHeight.value,
    letterSpacing: letterSpacing.value,
    scrollback: 5000,
    theme: xtermTheme(),
    linkHandler,
  }))
  const fitAddon = markRaw(new FitAddon())
  created.loadAddon(fitAddon)
  created.loadAddon(markRaw(new WebLinksAddon((_event, uri) => openLink(uri))))
  created.open(paneHost.value)
  loadRenderer(created)
  term.value = created
  fit = fitAddon
  return created
}

function measurePane(): { cols: number; rows: number } | undefined {
  if (!paneHost.value?.clientWidth || !paneHost.value.clientHeight) return undefined
  const proposed = fit?.proposeDimensions()
  if (!proposed?.cols || !proposed.rows) return undefined
  return { cols: proposed.cols, rows: proposed.rows }
}

// The pane renders tmux's grid, never its own fit — the Code view's rule
// (useTerminalWindows.ts): a host resize is a size *vote* posted to
// sessions/resize, and the window 'layout-changed' frame tmux answers with is what
// actually resizes xterm. The fit addon is kept only for proposeDimensions.
function attachStream(created: Terminal, terminalId: string, windowId: string, paneId: string): void {
  if (!client.value || !paneHost.value) return

  paneWindowId = windowId
  panePaneId = paneId
  disposers.push(silenceDeviceReports(created))
  disposers.push(created.onData((data) => send(data)))
  disposers.push({ dispose: installTerminalImages(paneHost.value, {
    pasteText: sendPaste,
    capture: () => {
      const capturedSocket = socket
      const capturedPane = panePaneId
      return {
        current: () => socket === capturedSocket && capturedSocket?.readyState === WebSocket.OPEN && panePaneId === capturedPane && props.active !== false,
        paste: (text, signal) => pasteTerminalImage(terminalId, capturedPane, text, signal),
        focus: () => created.focus(),
      }
    },
  }) })
  disposers.push(watchTailPin(created, paneHost.value, (scrolledUp) => { paneScrolledUp.value = scrolledUp }))

  const opened = client.value.openStream(terminalId)
  opened.onmessage = (event: MessageEvent<ArrayBuffer>) => {
    const frame = decodeFrame(event.data)
    if (!frame) return
    if (frame.type === 'window') {
      // Every window-event kind carries tmux's own size, and the payload doc
      // is explicit about why: the renderer must draw at that size or
      // cursor-addressed output lands wrong. Applying it from any kind also
      // covers a 'layout-changed' that fired before this socket subscribed.
      if (frame.kind !== 'closed' && frame.state.windowId === paneWindowId && frame.state.width && frame.state.height) {
        created.resize(frame.state.width, frame.state.height)
      }
      // Full window events supply the input target when attach returned no
      // pane ID.
      if (frame.kind !== 'closed' && frame.state.windowId === paneWindowId && frame.state.activePane) {
        panePaneId = frame.state.activePane
      }
      return
    }
    if (frame.windowId !== paneWindowId) return
    if (frame.type === 'output') created.write(frame.data)
    else if (frame.kind === 'exited' || frame.kind === 'error') exited()
  }
  opened.onerror = () => fail('The session connection dropped.')
  opened.onclose = () => { if (paneStatus.value === 'live') fail('The session connection closed.') }
  socket = opened

  observer = new ResizeObserver(() => scheduleSizeVote())
  observer.observe(paneHost.value)
}

function send(data: string): void {
  if (socket?.readyState !== WebSocket.OPEN || !panePaneId) return
  for (const frame of encodeInputFrames(panePaneId, data)) socket.send(frame)
}

function sendPaste(text: string): void {
  if (socket?.readyState !== WebSocket.OPEN || !panePaneId) return
  for (const frame of encodePasteFrames(panePaneId, text)) socket.send(frame)
}

let lastVote: { cols: number; rows: number } | null = null

function scheduleSizeVote(): void {
  clearTimeout(resizeTimer)
  resizeTimer = setTimeout(voteSize, RESIZE_DEBOUNCE_MS)
}

function voteSize(): void {
  if (paneStatus.value !== 'live' || openSessionId.value === null) return
  const proposed = measurePane()
  if (!proposed) return
  if (lastVote && proposed.cols === lastVote.cols && proposed.rows === lastVote.rows) return
  lastVote = proposed
  void client.value?.resizeSession(openSessionId.value, proposed.cols, proposed.rows).catch(() => {
    // A vote against a just-closed session is not an error worth surfacing.
  })
}

// The agent exited, so the pane clears to the zero state; the session list is
// refreshed so its row drops the stale terminalId.
function exited(): void {
  teardownPane()
  paneStatus.value = 'idle'
  void reloadRecents()
}

// Any other end of the stream clears the pane the same way — a dead screen is
// not worth keeping — but says why in the zero state.
function fail(why: string): void {
  teardownPane()
  paneStatus.value = 'idle'
  paneError.value = why
  void reloadRecents()
}

function teardownStream(): void {
  if (socket) {
    socket.onmessage = null
    socket.onerror = null
    socket.onclose = null
    socket.close()
    socket = null
  }
}

function teardownPane(): void {
  teardownStream()
  clearTimeout(resizeTimer)
  observer?.disconnect()
  observer = null
  for (const disposer of disposers.splice(0)) disposer.dispose()
  term.value?.dispose()
  term.value = null
  fit = null
  rendered = false
  paneWindowId = ''
  lastVote = null
  paneError.value = ''
  paneScrolledUp.value = false
}

function scrollPaneToBottom(): void {
  term.value?.scrollToBottom()
  term.value?.focus()
}

function openLink(uri: string): void {
  void Browser.OpenURL(uri).catch(() => {})
}

const linkHandler: ILinkHandler = { activate: (_event, uri) => openLink(uri) }

function loadRenderer(target: Terminal): void {
  claimAtlasRenderer(target, (addon) => disposers.push(addon), (claimed) => { rendered = claimed })
}

watch(theme, () => { if (term.value) term.value.options.theme = xtermTheme() })
watch(
  [fontSizePx, fontFamily, fontWeight, fontWeightBold, lineHeight, letterSpacing],
  async ([px, family, weight, weightBold, height, spacing]) => {
    await loadTerminalFaces(family, px, weight, weightBold)
    if (!term.value) return
    term.value.options.fontFamily = terminalFontStack(family)
    term.value.options.fontSize = px
    term.value.options.fontWeight = weight
    term.value.options.fontWeightBold = weightBold
    term.value.options.lineHeight = height
    term.value.options.letterSpacing = spacing
    scheduleSizeVote()
  },
)

// ── Focus handles for the global keymap (agents.focus-sidebar/-pane) ─────────
const sidebarEl = ref<InstanceType<typeof AgentsSidebar> | null>(null)

onMounted(() => {
  setAgentsTreeHandles({
    focusList: () => sidebarEl.value?.focus(),
    focusFilter: () => sidebarEl.value?.focusFilter(),
    focusPane: () => term.value?.focus(),
  })
})
onBeforeUnmount(() => {
  setAgentsTreeHandles(null)
  teardownPane()
  stopActivityPolling()
})
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col bg-app" data-testid="agents-mode">
    <div v-if="checking" class="flex flex-1 items-center justify-center font-mono text-xs text-text-4">Checking…</div>

    <div
      v-else-if="!available"
      class="flex flex-1 flex-col items-center justify-center gap-3 px-10 text-center"
      data-testid="agents-unavailable"
    >
      <IconMessagesSquare class="size-6 text-text-4" />
      <div class="text-[13.5px] font-semibold">Chats area unavailable</div>
      <p class="max-w-[420px] text-xs leading-relaxed text-text-3" data-testid="agents-unavailable-reason">
        {{ reason || 'The Chats area is not available in this build.' }}
      </p>
      <button
        type="button"
        class="mt-1 cursor-pointer rounded border border-strong px-3 py-1.5 text-xs text-text-2 hover:text-text"
        data-testid="agents-retry"
        @click="loadWorkspaces"
      >Try again</button>
    </div>

    <div v-else class="flex min-h-0 min-w-0 flex-1">
      <AgentsSidebar
        v-if="!sidebarCollapsed"
        ref="sidebarEl"
        :active="props.active"
        :selected-workspace="selectedWorkspace"
        :open-session-id="openSessionId"
        :starting-session="startingSession"
        :session-activity="sessionActivity"
        @select-session="handleSidebarSelectSession"
        @request-new-session="handleNewSessionRequest"
        @start-session="handleStartSessionIn"
        @select-workspace="selectWorkspace"
        @create-workspace="openCreateWorkspace"
        @edit-workspace="openEditWorkspace"
        @close-session="closeRow"
        @rename-session="openRenameSession"
        @commit-rename="commitSessionRename"
        @delete-session="removeRow"
      />

      <!-- The pane's chrome is conditional strips: the missing-capability
           warnings, and — while a session is opening or open — a status bar
           naming the session's own workspace, which the focused one need not
           match. -->
      <div class="flex min-h-0 min-w-0 flex-1 flex-col">
        <div
          v-if="selectedWorkspace && missingMCPs.length"
          class="shrink-0 border-b border-border bg-severity-warning-tint px-3 py-1.5 text-[11px] text-severity-warning"
          data-testid="agents-missing-mcps"
        >Missing MCP servers: {{ missingMCPs.join(', ') }}</div>

        <div
          v-if="selectedWorkspace && missingSkillsNotice"
          class="shrink-0 border-b border-border bg-severity-warning-tint px-3 py-1.5 text-[11px] text-severity-warning"
          data-testid="agents-missing-skills"
        >{{ missingSkillsNotice }}</div>

        <div
          v-if="selectedWorkspace && unknownPackageNames.length"
          class="shrink-0 border-b border-border bg-severity-warning-tint px-3 py-1.5 text-[11px] text-severity-warning"
          data-testid="agents-missing-packages"
        >Missing skill packages: {{ unknownPackageNames.join(', ') }}</div>

        <PaneStatusBar
          v-if="paneStatus !== 'idle'"
          testid="agents-pane-statusbar"
          :label="paneWorkspaceName"
          :path="paneWorkspaceDir"
          :error="paneActionError"
          :editor-title="editor.command ? editor.title : ''"
          @open-editor="openPaneWorkspaceInEditor"
          @reveal="revealPaneWorkspace"
        >
        </PaneStatusBar>

        <div class="relative min-h-0 flex-1 bg-app">
          <!-- TerminalTab.vue's shape, for the same reasons: xterm opens in the
               unpadded inner host so the fit measurement reads the content box
               — measuring the padded wrapper over-proposes the grid by a row
               under border-box — and overflow-auto scrolls a grid tmux sized
               larger than this box instead of painting over the UI below. -->
          <div
            v-show="paneLaidOut"
            class="absolute inset-0 overflow-auto px-2 py-1.5"
            data-terminal-input-scope
            data-testid="agents-session-pane"
            @mousedown="term?.focus()"
          >
            <div ref="paneHost" class="size-full" />
          </div>

          <!-- The Code view's pill, placed and worded identically: new output
               keeps landing below the fold while the viewport is scrolled up,
               and this is the way back to the live tail. -->
          <Transition name="tail-pill">
            <button
              v-if="paneScrolledUp && paneStatus === 'live'"
              type="button"
              class="absolute bottom-3 right-5 z-10 flex cursor-pointer items-center gap-1.5 rounded-full border border-strong bg-raised/95 px-3 py-1.5 text-[11.5px] text-text-2 shadow-lg hover:text-text"
              data-testid="agents-scroll-to-bottom"
              @click="scrollPaneToBottom"
            ><IconArrowDown class="size-3" />Scroll to bottom</button>
          </Transition>

          <!-- Covers the whole launch, not just the beat before xterm exists:
               most of a start is spent waiting on tmux after the grid is
               built, and a bare dark pane there reads as nothing happening. -->
          <div
            v-if="paneStatus === 'opening'"
            class="absolute inset-0 z-10 flex items-center justify-center bg-app"
            data-testid="agents-pane-opening"
          >
            <p class="flex items-center gap-2 font-mono text-xs text-text-4">
              <IconLoaderCircle class="size-3.5 animate-spin" aria-hidden="true" />Opening…
            </p>
          </div>
          <!-- The zero state: no PTY exists, so the pane says what a chat is,
               carries whatever ended the last one, and offers the same new-chat
               gesture the sidebar's + does. -->
          <div
            v-else-if="paneStatus === 'idle'"
            class="hive-scroll absolute inset-0 z-10 overflow-y-auto bg-app"
            data-testid="agents-pane-empty"
          >
            <div class="flex min-h-full items-center justify-center px-8 py-10">
              <div class="flex w-full max-w-[380px] flex-col items-center gap-3 text-center">
                <IconMessagesSquare class="size-6 text-text-4" />
                <h2 class="text-[13.5px] font-semibold text-text">No chat open</h2>
                <p class="text-xs leading-relaxed text-text-3">
                  A chat is an agent attached to a workspace's directory. It launches with the
                  workspace's command and MCP servers.
                </p>
                <p v-if="!workspaces.length" class="text-xs leading-relaxed text-text-3">
                  No workspaces yet. Author one under {{ root }}.
                </p>
                <p v-if="paneError" class="text-xs leading-relaxed text-severity-error" data-testid="agents-pane-error">{{ paneError }}</p>
                <BaseButton
                  size="sm"
                  :disabled="!workspaces.length"
                  data-testid="agents-new-session-open"
                  @click="handleNewSessionRequest"
                >New chat</BaseButton>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- A sibling of the pane column, never inside it: the terminal host must
           not be re-keyed or unmounted by the canvas opening, and the
           ResizeObserver absorbs the width change with an ordinary size vote. -->
      <AgentCanvasPane
        v-if="canvasVisible && routeChatId !== null"
        :session="routeChatId"
        :workspace="paneWorkspaceDir"
        :name="canvasName"
        :client="client"
        @close="syncCanvasQuery(false)"
        @open-url="openLink"
        @pick="(name) => syncCanvasQuery(true, name)"
      />
    </div>

    <AgentWorkspaceEditor
      v-if="workspaceEditorOpen"
      :workspace="editingWorkspace"
      :busy="workspaceEditorBusy"
      :error="workspaceEditorError"
      @close="workspaceEditorOpen = false"
      @save="saveWorkspace"
      @delete="deleteWorkspaceFromEditor"
    />

    <NewChatDialog
      v-if="newSessionOpen"
      :workspaces="workspaces"
      :initial-workspace="defaultWorkspaceDir"
      :root="root"
      @close="newSessionOpen = false"
      @submit="submitNewSession"
    />

    <ChatRenameDialog
      v-if="renamingSession"
      :name="renamingSession.name"
      :busy="renameBusy"
      :error="renameError"
      @close="renamingSession = null"
      @save="saveSessionRename"
    />
  </div>
</template>
