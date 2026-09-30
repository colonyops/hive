<script setup lang="ts">
// The Chats area's sidebar: workspaces as parent rows with their chats nested
// beneath, in one scroll region. Position is what states which workspace a chat
// belongs to, so it holds for every workspace at once; focusing a workspace no
// longer hides the others.
//
// The row language is the hub sidebar's (SideBar.vue, SidebarFeedRow.vue), not
// the Code view's: single-line rounded rows, an 18px bordered icon chip leading
// each one, a tinted row for the selection, and actions revealed in a slot the
// row already reserves. A workspace row is SideBar's folder header — its chip
// swaps to a fold chevron on hover — and a chat row is a feed row. What the
// Code view still lends is the activity vocabulary: the spinner, the approval
// alert, and the liveness dot mean here exactly what they mean there.
//
// This component owns no pane state and no router — those stay in
// AgentsMode.vue. Every action that reaches the pane (resuming, closing,
// starting a chat) or the manifest (create/edit workspace — delete lives in
// the editor) is an emitted event; only tree state, the chat row menus, and
// the chat delete confirmation live here.
import { computed, nextTick, ref, shallowRef, watch, type Component } from 'vue'
import { useStorage } from '@vueuse/core'
import IconCalendarClock from '~icons/lucide/calendar-clock'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconChevronsDownUp from '~icons/lucide/chevrons-down-up'
import IconChevronsUpDown from '~icons/lucide/chevrons-up-down'
import IconCircleAlert from '~icons/lucide/circle-alert'
import IconEllipsisVertical from '~icons/lucide/ellipsis-vertical'
import IconFolderPlus from '~icons/lucide/folder-plus'
import IconLoaderCircle from '~icons/lucide/loader-circle'
import IconMessageSquare from '~icons/lucide/message-square'
import IconPencil from '~icons/lucide/pencil'
import IconPin from '~icons/lucide/pin'
import IconPinOff from '~icons/lucide/pin-off'
import IconPlay from '~icons/lucide/play'
import IconPlus from '~icons/lucide/plus'
import IconPower from '~icons/lucide/power'
import IconRotateCw from '~icons/lucide/rotate-cw'
import IconSearch from '~icons/lucide/search'
import IconTrash2 from '~icons/lucide/trash-2'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import AppMenu from './AppMenu.vue'
import ConfirmationDialog from './ConfirmationDialog.vue'
import PanelResizeHandle from './PanelResizeHandle.vue'
import { useAgentWorkspaces } from '../composables/useAgentWorkspaces'
import { useAgentSessionsAll } from '../composables/useAgentSessionsAll'
import { useResizablePanel } from '../composables/useResizablePanel'
import { useTerminalPinnedChats } from '../composables/useTerminalPinnedChats'
import { relativeAge } from '../lib/age'
import type { AgentSession, AgentWorkspace } from '../lib/agentWorkspacesClient'
import type { MenuEntry } from '../types/menu'

const props = withDefaults(defineProps<{
  active?: boolean
  /**
   * The workspace the route currently has focused, '' when none. It scopes what
   * AgentsMode regenerates and reports above the pane, and unfolds its row here;
   * the sidebar draws no mark for it — see .sidebar-entry-selected below.
   */
  selectedWorkspace?: string
  /** The chat currently attached to the pane, for highlighting its row. */
  openSessionId?: number | null
  /** True while AgentsMode is launching a chat into the pane. */
  startingSession?: boolean
  /** Per-chat activity status ('approval' | 'active' | 'ready') across every workspace. */
  sessionActivity?: Record<number, string>
}>(), {
  active: false,
  selectedWorkspace: '',
  openSessionId: null,
  startingSession: false,
  sessionActivity: () => ({}),
})

const emit = defineEmits<{
  'select-session': [session: AgentSession]
  'request-new-session': []
  /** Start a chat in this workspace immediately, with no dialog. */
  'start-session': [dir: string]
  'select-workspace': [dir: string]
  'create-workspace': []
  'edit-workspace': [workspace: AgentWorkspace]
  'close-session': [session: AgentSession]
  'rename-session': [session: AgentSession]
  'delete-session': [session: AgentSession]
  'commit-rename': [session: AgentSession, name: string]
}>()

const {
  workspaces, workspacesLoading, workspacesLoaded, workspacesError, rootProblem, reloadWorkspaces,
} = useAgentWorkspaces()
const {
  recents, recentsLoading, recentsError, reloadRecents,
} = useAgentSessionsAll()
// Pinning is what puts a chat in the Code view's own sidebar; this row's menu is
// where it is turned on and off, and the mark below is how a row says it is on.
// It stays a Code-view arrangement rather than an ordering rule here — a pin
// changes which sidebar lists the chat, not where this one puts it.
const { isPinned, togglePin } = useTerminalPinnedChats()

// Both lists are module singletons (ADR a-workspace-declares-its-own-authority's shared-composable pattern),
// so this and AgentsMode's own workspaces reload can race harmlessly on
// activation — last response wins, and both fetch the same idempotent read.
watch(() => props.active, (active) => {
  if (!active) return
  void reloadWorkspaces()
  void reloadRecents()
}, { immediate: true })

// ── The tree ─────────────────────────────────────────────────────────────
// One node per workspace, carrying its own chats. They keep the order the
// cross-workspace read hands over (newest record first, stable under a resume —
// internal/app/data/queries: ListAllAgentWorkspaceSessions), so grouping costs
// no ordering. `live` is here for the fold default, which opens a workspace with
// something running in it.
interface WorkspaceNode {
  dir: string
  name: string
  /** null for a directory the manifest listing no longer knows about. */
  workspace: AgentWorkspace | null
  sessions: AgentSession[]
  live: boolean
}

const tree = computed<WorkspaceNode[]>(() => {
  const byWorkspace = new Map<string, AgentSession[]>()
  for (const session of recents.value) {
    const bucket = byWorkspace.get(session.workspace)
    if (bucket) bucket.push(session)
    else byWorkspace.set(session.workspace, [session])
  }

  const nodes: WorkspaceNode[] = []
  function claim(dir: string, workspace: AgentWorkspace | null): void {
    const sessions = byWorkspace.get(dir) ?? []
    byWorkspace.delete(dir)
    nodes.push({
      dir,
      name: workspace?.name || dir,
      workspace,
      sessions,
      live: sessions.some((session) => !!session.terminalId),
    })
  }

  for (const workspace of workspaces.value) claim(workspace.dir, workspace)
  // A directory removed outside the app leaves its chats in the database with
  // nowhere to hang. They get a row of their own rather than vanishing from a
  // list whose whole claim is that every chat is on it.
  for (const dir of [...byWorkspace.keys()]) claim(dir, null)
  return nodes
})

// The focus handle for the global keymap (agents.focus-sidebar), and where
// Escape puts the caret when it leaves the filter.
const rootEl = ref<HTMLElement | null>(null)

// ── The filter ───────────────────────────────────────────────────────────
// The Code view's narrowing, over this tree: a workspace whose name matches
// carries all its chats, so typing a workspace name is how to narrow to it,
// and otherwise a workspace survives on the chats of its own that matched.
const filter = ref('')
const filterInput = ref<HTMLInputElement | null>(null)

const filteredTree = computed<WorkspaceNode[]>(() => {
  const query = filter.value.trim().toLowerCase()
  if (!query) return tree.value
  const nodes: WorkspaceNode[] = []
  for (const node of tree.value) {
    if (node.name.toLowerCase().includes(query) || node.dir.toLowerCase().includes(query)) {
      nodes.push(node)
      continue
    }
    const sessions = node.sessions.filter((session) => session.name.toLowerCase().includes(query))
    if (sessions.length) nodes.push({ ...node, sessions })
  }
  return nodes
})

const emptyNote = computed(() => {
  const query = filter.value.trim()
  return query ? `Nothing matches “${query}”.` : 'No workspaces yet. Create one from the list menu.'
})

// Escape clears the field, and leaves it once there is nothing left to clear,
// so it is never a keystroke that appears to do nothing.
function escapeFilter(): void {
  if (filter.value) filter.value = ''
  else rootEl.value?.focus()
}

// Expand/collapse is transient view state, not configuration — localStorage,
// the same call the hub sidebar's folder collapse and the Code view's group
// collapse make. A workspace the user has never toggled has no entry and takes
// the default, which is open only where something is live or the open chat
// sits: a root's worth of dormant workspaces would otherwise bury the one being
// worked in. An explicit toggle always wins, including over the open chat,
// which is what makes a deliberate fold stay folded.
const expansion = useStorage<Record<string, boolean>>('hive.agents.sidebar.workspaces', {})

function expanded(node: WorkspaceNode): boolean {
  // A filter overrides the stored state: a workspace is only in the list
  // because something in it matched, and a folded one would hide the match.
  if (filter.value.trim()) return true
  return expansion.value[node.dir]
    ?? (node.live || node.sessions.some((session) => session.id === props.openSessionId))
}

function toggleExpanded(node: WorkspaceNode): void {
  expansion.value[node.dir] = !expanded(node)
}

// Every workspace, not the ones the tree happens to be drawing: a bulk fold
// that the filtered-away workspaces escaped would spring back open the moment
// the query came off.
function setAllExpanded(open: boolean): void {
  for (const node of tree.value) expansion.value[node.dir] = open
}

function editWorkspace(node: WorkspaceNode): void {
  if (node.workspace) emit('edit-workspace', node.workspace)
}

// Unfolded here rather than left to the focus watcher: starting a chat moves
// the focus too, but not when the workspace already holds it, and the row it
// lands on has to be visible either way.
function startSessionIn(node: WorkspaceNode): void {
  if (!node.workspace) return
  unfold(node.dir)
  emit('start-session', node.dir)
}

// The workspace this row's own click focused, read and cleared by the focus
// watcher below.
let focusedHere = ''

// A click folds, as the Code view's group row does — the focus it also moves
// changes nothing on screen except the missing-capability strips above the
// pane, so on its own the row read as inert.
function activateWorkspace(node: WorkspaceNode): void {
  toggleExpanded(node)
  // A directory the listing cannot see cannot be focused, so its row folds and
  // nothing more.
  if (!node.workspace) return
  focusedHere = node.dir
  emit('select-workspace', node.dir)
}

// Only a workspace that is actually folded gets an entry written, so focusing
// one the default already opened leaves it on the default rather than pinning
// it open for good.
function unfold(dir: string): void {
  const node = tree.value.find((candidate) => candidate.dir === dir)
  if (node && !expanded(node)) expansion.value[dir] = true
}

// Focus that arrives from outside unfolds the workspace it names: the Code view
// deep-links into a chat by workspace, and the row it lands on has to be
// visible. A click is excluded because the click already decided the fold —
// without this the watcher would re-open a workspace the user just folded.
watch(() => props.selectedWorkspace, (dir) => {
  const fromClick = dir === focusedHere
  focusedHere = ''
  if (dir && !fromClick) unfold(dir)
}, { immediate: true })

// ── What a row's tooltip says ────────────────────────────────────────────
// Everything a workspace row used to stack under its name. A brand mark stood
// in for the agent here for a while and earned nothing: the workspaces under
// one root normally run the same agent, so the column was one glyph repeated
// down the sidebar.
function workspaceTooltip(node: WorkspaceNode): string {
  if (!node.workspace) return `${node.dir}\nThis directory is no longer in the workspace root.`
  const parts = [node.name, node.workspace.command]
  const next = nextScheduleLine(node.workspace)
  if (next) parts.push(next)
  if (node.workspace.problem) parts.push(node.workspace.problem)
  else if (node.workspace.notice) parts.push(node.workspace.notice)
  return parts.join('\n')
}

// A manifest that will not parse and a directory the root no longer lists are
// both states no action on the row can recover from, so they get a mark of
// their own. The MCP notice deliberately gets none: it reports a property of
// the agent rather than anything wrong here, so it would sit lit forever.
function workspaceBroken(node: WorkspaceNode): boolean {
  return !node.workspace || !!node.workspace.problem
}

// A header draws no rollup of its own, so the schedule that fires next is a
// line on the tooltip rather than a mark on the row. A disabled or unparseable
// schedule has no nextRunAt and is not a candidate.
function nextScheduleLine(workspace: AgentWorkspace): string {
  let soonest = ''
  let soonestAt = Number.POSITIVE_INFINITY
  for (const schedule of workspace.schedules) {
    if (schedule.disabled || schedule.nextRunAt === null || schedule.nextRunAt >= soonestAt) continue
    soonest = schedule.name || schedule.id
    soonestAt = schedule.nextRunAt
  }
  if (!soonest) return ''
  const when = new Date(soonestAt).toLocaleString([], { weekday: 'short', hour: '2-digit', minute: '2-digit', hour12: false })
  return `Next: ${soonest}, ${when}`
}

// The Code view's vocabulary, unchanged: a spinning loader while the agent
// works, an alert while it waits on approval. Any other live chat gets the
// green liveness dot — ready-and-waiting is still live — and an idle chat an
// explicit hollow ring the same size, so live-vs-idle is always stated rather
// than implied by absence.
interface StatusIndicator {
  icon: Component
  cls: string
  label: string
  animated?: boolean
}

const sessionIndicators = computed<Record<number, StatusIndicator>>(() => {
  const out: Record<number, StatusIndicator> = {}
  for (const [id, status] of Object.entries(props.sessionActivity)) {
    switch (status) {
      case 'active':
        out[Number(id)] = { icon: IconLoaderCircle, cls: 'text-severity-success', label: 'Agent is working', animated: true }
        break
      case 'approval':
        out[Number(id)] = { icon: IconCircleAlert, cls: 'text-severity-warning', label: 'Agent needs approval' }
        break
    }
  }
  return out
})

// A dormant chat says how long ago it was opened; a live one does not, because
// its mark in the trailing slot already says the only thing an age would
// compete with.
function chatAge(session: AgentSession): string {
  if (session.terminalId || sessionIndicators.value[session.id]) return ''
  const age = relativeAge(session.lastOpenedAt)
  return age === 'now' ? 'just now' : `${age} ago`
}

function chatTooltip(session: AgentSession): string {
  const parts = [session.name]
  const indicator = sessionIndicators.value[session.id]
  if (indicator) parts.push(indicator.label)
  else if (session.terminalId) parts.push('Agent running')
  if (session.scheduleId) parts.push(`Started by schedule ${session.scheduleId}`)
  if (session.notice) parts.push(session.notice)
  return parts.join('\n')
}

// ── Chat row menus (the hub sidebar's ellipsis + AppMenu shape) ──────────
// One menu is ever open, keyed 's:<id>'. The toggle shares the trailing slot
// with the status mark, so neither hovering a row nor opening its menu moves
// the name. The menu anchors to the row element and teleports (AppMenu's
// anchored mode): the tree scrolls, and an absolute panel inside a scroll
// region would extend it instead of floating over the sidebar — which is also
// why the panel is a child of the row rather than of the 18px trailing cell,
// so the un-teleported fallback has a sane box to position against. Workspace
// rows carry no menu — the editor is a workspace's whole management surface,
// delete included, so their reserved action is a single edit button.
const openMenu = ref('')
// Resolved from the event that opened the menu rather than from template refs
// held in a Map. The Map was keyed per row and written by inline `:ref`
// arrows, which Vue re-invokes on every re-render — so it could hand AppMenu a
// row that had since been unmounted by a fold, and a detached element measures
// as a zero rect: the panel then took `left: 0; width: 0` and drew nothing.
// One menu is ever open, so one anchor is all there is to track, and taking it
// from `currentTarget` at open time means it is attached by construction.
const menuAnchor = shallowRef<HTMLElement | null>(null)
const menuToggle = shallowRef<HTMLElement | null>(null)

function openSessionMenu(session: AgentSession, event: Event): void {
  const key = `s:${session.id}`
  if (openMenu.value === key) {
    closeSessionMenu()
    return
  }
  const target = event.currentTarget instanceof HTMLElement ? event.currentTarget : null
  // The kebab opens it from the button; a right-click opens it from the row.
  menuToggle.value = target instanceof HTMLButtonElement ? target : null
  menuAnchor.value = target?.closest<HTMLElement>('[data-testid="agents-sidebar-session-row"]') ?? null
  openMenu.value = key
}

function closeSessionMenu(): void {
  openMenu.value = ''
  menuAnchor.value = null
  menuToggle.value = null
}

// ── The list menu ────────────────────────────────────────────────────────
// The bar's fourth control, as in the Code view: what acts on the list rather
// than on a row. Creating a workspace lives here because it is the rarer of
// the two creates — a root holds a handful of workspaces and many chats — and
// the bar's + is the one the user reaches for. Reload is not here: the bar's
// own control does it, and it doubles as the indicator that says it is
// running.
const listMenuOpen = ref(false)
const listMenuToggle = shallowRef<HTMLElement | null>(null)

const listMenuEntries = computed<MenuEntry[]>(() => [
  { kind: 'action', id: 'new-workspace', label: 'New workspace…', icon: IconFolderPlus, testid: 'agents-sidebar-new-workspace' },
  { kind: 'separator' },
  { kind: 'action', id: 'collapse-all', label: 'Collapse all', icon: IconChevronsDownUp, testid: 'agents-sidebar-collapse-all' },
  { kind: 'action', id: 'expand-all', label: 'Expand all', icon: IconChevronsUpDown, testid: 'agents-sidebar-expand-all' },
])

function onListMenuSelect(id: string): void {
  listMenuOpen.value = false
  if (id === 'new-workspace') emit('create-workspace')
  else if (id === 'collapse-all') setAllExpanded(false)
  else if (id === 'expand-all') setAllExpanded(true)
}

const listLoading = computed(() => workspacesLoading.value || recentsLoading.value)

function reloadAll(): void {
  void reloadWorkspaces()
  void reloadRecents()
}

function sessionMenuEntries(session: AgentSession): MenuEntry[] {
  const entries: MenuEntry[] = [
    { kind: 'action', id: 'rename', label: 'Rename…', icon: IconPencil, testid: 'agents-sidebar-session-rename' },
    isPinned(session.id)
      ? { kind: 'action', id: 'pin', label: 'Unpin from Code', icon: IconPinOff, testid: 'agents-sidebar-session-unpin' }
      : { kind: 'action', id: 'pin', label: 'Pin to Code', icon: IconPin, testid: 'agents-sidebar-session-pin' },
  ]
  // "Start agent" rather than "Start chat": the chat outlives its agent — the
  // row is here either way — and this is the entry that mirrors Stop agent.
  entries.push(session.terminalId
    ? { kind: 'action', id: 'stop', label: 'Stop agent', icon: IconPower, testid: 'agents-sidebar-session-close' }
    : { kind: 'action', id: 'start', label: 'Start agent', icon: IconPlay, testid: 'agents-sidebar-session-start-menu' })
  entries.push(
    { kind: 'separator' },
    { kind: 'action', id: 'delete', label: 'Delete', icon: IconTrash2, testid: 'agents-sidebar-session-delete' },
  )
  return entries
}

function onSessionMenuSelect(session: AgentSession, id: string): void {
  closeSessionMenu()
  if (id === 'rename') emit('rename-session', session)
  else if (id === 'pin') togglePin(session.id)
  else if (id === 'start') emit('select-session', session)
  else if (id === 'stop') emit('close-session', session)
  else if (id === 'delete') pendingDeleteSession.value = session
}

// ── Inline chat rename ────────────────────────────────────────────────────
// Same idiom as the Code view's tmux window rename (TerminalMode.vue).
const renamingSessionId = ref<number | null>(null)
const renameDraft = ref('')

function startRename(session: AgentSession): void {
  if (renamingSessionId.value === session.id) return
  renamingSessionId.value = session.id
  renameDraft.value = session.name
}

function commitRename(): void {
  // Read and clear the id before anything else: Escape sets it to null and
  // unmounts the input, which fires blur, which calls commitRename again.
  // Clearing first makes that second call a no-op instead of a save.
  const id = renamingSessionId.value
  if (id === null) return
  renamingSessionId.value = null
  const session = tree.value.flatMap((node) => node.sessions).find((s) => s.id === id)
  if (!session) return
  const name = renameDraft.value.trim()
  if (!name || name === session.name) return
  emit('commit-rename', session, name)
}

// ── Chat delete confirmation ─────────────────────────────────────────────
// A stacked ConfirmationDialog, deliberately not InlineConfirm: the inline
// strip is reserved for surfaces the user already opened (the workspace
// editor); a row menu has no such surface to expand inside.
const pendingDeleteSession = ref<AgentSession | null>(null)

function confirmDeleteSession(): void {
  if (!pendingDeleteSession.value) return
  if (renamingSessionId.value === pendingDeleteSession.value.id) renamingSessionId.value = null
  emit('delete-session', pendingDeleteSession.value)
  pendingDeleteSession.value = null
}

// ── Panel sizing ─────────────────────────────────────────────────────────
// The sidebar's width, persisted like every other panel (useResizablePanel).
// There is no second handle: the workspaces/chats divider existed only because
// two lists competed for the same height, and one region has nothing to split.
const { size: sidebarWidth, startResize: startSidebarResize, step: stepSidebar } = useResizablePanel({
  storageKey: 'hive.panel.agents.sidebar', defaultSize: 260, min: 180, max: 400, edge: 'right',
})

// ── The selection rail (the Code view's traveling mark, TerminalMode.vue) ─
// Measured off the open chat's row rather than drawn by it, so changing the
// selection reads as the same mark relocating rather than a second one
// appearing where the first went out.
interface SelectionRail { y: number; height: number; shown: boolean }
const treeContent = ref<HTMLElement | null>(null)
const rail = ref<SelectionRail>({ y: 0, height: 0, shown: false })

// A rail with no row to sit on fades out where it stands rather than resetting,
// so it does not travel from a stale origin when one reappears.
function measureRail(): void {
  const content = treeContent.value
  const row = content?.querySelector<HTMLElement>('[data-testid="agents-sidebar-session-row"][data-open="true"]')
  if (!content || !row) {
    rail.value = { ...rail.value, shown: false }
    return
  }
  rail.value = {
    y: row.getBoundingClientRect().top - content.getBoundingClientRect().top,
    height: row.offsetHeight,
    shown: true,
  }
}

// The stored fold map mutates in place, so the fold state reaches this watch as
// a key rather than by identity.
const foldKey = computed(() => tree.value.map((node) => `${node.dir}:${expanded(node) ? 1 : 0}`).join('|'))

watch(
  () => [props.openSessionId, tree.value, foldKey.value] as const,
  () => void nextTick(measureRail),
  { immediate: true },
)

// Rows also move without a selection change — a workspace refilling after a
// reload, an error line appearing — and a content resize is every one of those.
watch(treeContent, (el, _previous, onCleanup) => {
  if (!el || typeof ResizeObserver === 'undefined') return
  const observer = new ResizeObserver(() => measureRail())
  observer.observe(el)
  onCleanup(() => observer.disconnect())
})

defineExpose({
  focus: () => rootEl.value?.focus(),
  focusFilter: () => filterInput.value?.focus(),
})
</script>

<template>
  <aside
    ref="rootEl"
    class="relative flex shrink-0 flex-col border-r border-border bg-sidebar"
    :style="{ width: `${sidebarWidth}px` }"
    data-testid="agents-workspace-sidebar"
    tabindex="-1"
  >
    <!-- The Code view's sidebar bar, control for control (TerminalMode.vue):
         the two areas are the same app, and a tree with its own vocabulary of
         chrome does not read that way. The filter is flush rather than boxed
         for that view's reason too — the sidebar resizes down narrow, and a
         bordered field beside three controls leaves the bar looking like
         nothing but chrome. -->
    <div class="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3">
      <IconSearch class="size-3 shrink-0" :class="filter ? 'text-text-3' : 'text-text-4'" />
      <input
        ref="filterInput"
        v-model="filter"
        type="text"
        placeholder="Filter…"
        aria-label="Filter workspaces and chats"
        class="min-w-0 flex-1 bg-transparent text-[12.5px] text-text outline-none placeholder:text-text-4"
        autocapitalize="off"
        autocorrect="off"
        spellcheck="false"
        data-testid="agents-sidebar-filter"
        @keydown.esc.prevent="escapeFilter"
      >
      <!-- Doubles as the staleness indicator, as the Code view's does: both
           lists render from their last-good rows, and the spin is what says a
           re-read is still in flight. -->
      <button
        type="button"
        class="flex size-6 cursor-pointer items-center justify-center rounded-[7px] text-text-3 hover:bg-chip hover:text-text disabled:cursor-default"
        data-testid="agents-sidebar-reload"
        aria-label="Reload workspaces"
        :aria-busy="listLoading"
        :disabled="listLoading"
        @click="reloadAll"
      ><IconRotateCw class="size-3.5" :class="{ 'animate-spin': listLoading }" /></button>
      <button
        type="button"
        class="flex size-6 cursor-pointer items-center justify-center rounded-[7px] text-text-3 hover:bg-chip hover:text-text disabled:cursor-default"
        data-testid="agents-sidebar-new-session"
        aria-label="New chat"
        title="New chat"
        :disabled="startingSession"
        :aria-busy="startingSession"
        @click="emit('request-new-session')"
      >
        <IconLoaderCircle v-if="startingSession" class="size-3.5 animate-spin" />
        <IconPlus v-else class="size-3.5" />
      </button>
      <!-- List-wide operations; a workspace's own live on its row. -->
      <div class="relative flex">
        <button
          ref="listMenuToggle"
          type="button"
          class="flex size-6 cursor-pointer items-center justify-center rounded-[7px] text-text-3 hover:bg-chip hover:text-text"
          data-testid="agents-sidebar-menu-toggle"
          aria-label="Workspace list actions"
          aria-haspopup="menu"
          :aria-expanded="listMenuOpen"
          @click="listMenuOpen = !listMenuOpen"
        ><IconEllipsisVertical class="size-3.5" /></button>
        <AppMenu
          v-if="listMenuOpen"
          :entries="listMenuEntries"
          :ignore="[listMenuToggle]"
          testid="agents-sidebar-menu"
          @close="listMenuOpen = false"
          @select="onListMenuSelect"
        />
      </div>
    </div>

    <div class="hive-scroll min-h-0 flex-1 overflow-y-auto pb-4" data-testid="agents-sidebar-tree">
      <p v-if="workspacesError" class="px-3 py-2 text-xs text-severity-error" data-testid="agents-sidebar-workspaces-error">{{ workspacesError }}</p>
      <div
        v-else-if="rootProblem"
        class="flex flex-col gap-2 px-3 py-2 text-xs text-text-3"
        data-testid="agents-sidebar-root-missing"
      >
        <p class="leading-relaxed">The configured workspace root is unavailable:</p>
        <p class="font-mono text-[11px] text-severity-error">{{ rootProblem }}</p>
        <p class="leading-relaxed">Point <code>agent_workspaces.dir</code> in settings.yaml at a reachable folder; Settings ▸ Chats shows where it resolves.</p>
      </div>
      <p v-else-if="!workspacesLoaded" class="px-3 py-2 font-mono text-xs text-text-4" data-testid="agents-sidebar-workspaces-loading">Loading…</p>
      <p v-else-if="!filteredTree.length" class="px-3 py-2 text-xs text-text-3" data-testid="agents-sidebar-workspaces-empty">
        {{ emptyNote }}
      </p>
      <template v-else>
        <!-- The chat read can fail on its own, which leaves every workspace row
             correct and every count wrong; say so rather than draw an empty
             tree. -->
        <p v-if="recentsError" class="px-3 pb-1 text-[11px] text-severity-error" data-testid="agents-sidebar-sessions-error">{{ recentsError }}</p>

        <!-- The rails' positioning context, and the box whose resize tells them
             a row has moved. -->
        <div ref="treeContent" class="relative">
        <!-- One workspace reads as one block, exactly as one repository does in
             the Code view's tree: the header keeps the sidebar's own surface and
             its chats sit in a recessed panel under it, so a long run of chats
             cannot bleed into the next workspace's. -->
        <div
          v-for="(node, index) in filteredTree"
          :key="node.dir"
          class="ws-block"
          :class="{ 'ws-block-first': index === 0 }"
          data-testid="agents-sidebar-workspace-block"
          :data-dir="node.dir"
        >
          <!-- Not a <button>: the fold chevron and the edit button are real
               buttons, which are invalid nested inside one. The div keeps the
               row focusable and Enter/Space focus the workspace like a button
               would (`.self`, so the chevron's own keystrokes do not also
               focus). -->
          <div
            class="ws-row"
            role="button"
            tabindex="0"
            data-testid="agents-sidebar-workspace-row"
            :data-dir="node.dir"
            :data-focused="node.dir === selectedWorkspace"
            :data-expanded="expanded(node)"
            :title="workspaceTooltip(node)"
            @click="activateWorkspace(node)"
            @keydown.enter.self.prevent="activateWorkspace(node)"
            @keydown.space.self.prevent="activateWorkspace(node)"
            @contextmenu.prevent="editWorkspace(node)"
          >
            <!-- Leading, not trailing: the trailing pitch is the three
                 controls, which are revealed on hover, and a fault has to
                 read at rest. The row's tooltip carries the wording. -->
            <IconTriangleAlert
              v-if="workspaceBroken(node)"
              class="size-3.5 shrink-0 text-severity-error"
              data-testid="agents-sidebar-workspace-problem"
              aria-hidden="true"
            />
            <span class="min-w-0 flex-1 truncate">{{ node.name }}</span>
            <!-- Three controls on one pitch, revealed together: the header
                 says nothing at rest but its own name and whether it is open. -->
            <button
              v-if="node.workspace"
              type="button"
              class="row-action"
              title="Edit workspace"
              aria-label="Edit workspace"
              data-testid="agents-sidebar-workspace-edit"
              @click.stop="editWorkspace(node)"
            ><IconPencil class="size-3" /></button>
            <button
              v-if="node.workspace"
              type="button"
              class="row-action"
              :title="`New chat in ${node.name}`"
              :aria-label="`New chat in ${node.name}`"
              :disabled="startingSession"
              data-testid="agents-sidebar-workspace-new-session"
              @click.stop="startSessionIn(node)"
            ><IconPlus class="size-3" /></button>
            <!-- The chevron trails the row, where the Code view's group chevron
                 sits, and says the same thing: the row itself folds, and this
                 is the affordance for it. -->
            <button
              type="button"
              class="ws-toggle"
              :aria-label="expanded(node) ? `Collapse ${node.name}` : `Expand ${node.name}`"
              :aria-expanded="expanded(node)"
              data-testid="agents-sidebar-workspace-toggle"
              @click.stop="toggleExpanded(node)"
            ><component :is="expanded(node) ? IconChevronDown : IconChevronRight" class="size-3" /></button>
          </div>

          <div v-if="expanded(node)" class="ws-well" data-testid="agents-sidebar-workspace-well">
            <p
              v-if="!node.sessions.length"
              class="chat-empty"
              data-testid="agents-sidebar-workspace-no-chats"
            >No chats yet.</p>
            <template v-else>
              <div
                v-for="session in node.sessions"
                :key="session.id"
                class="sidebar-entry"
                :class="{ 'sidebar-entry-selected': session.id === openSessionId, 'menu-open': openMenu === `s:${session.id}` }"
                role="button"
                tabindex="0"
                data-testid="agents-sidebar-session-row"
                :data-session-id="session.id"
                :data-workspace="node.dir"
                :data-open="session.id === openSessionId"
                :title="chatTooltip(session)"
                @click="emit('select-session', session)"
                @dblclick="startRename(session)"
                @keydown.enter.self.prevent="emit('select-session', session)"
                @keydown.space.self.prevent="emit('select-session', session)"
                @contextmenu.prevent="openSessionMenu(session, $event)"
              >
                <!-- A chat nobody clicked for wears the clock in the same
                     leading cell, so the tree says where it came from without
                     a second column; the tooltip names the schedule. -->
                <span
                  class="nav-icon"
                  :class="{ 'nav-icon-notice': !!session.notice }"
                  :data-testid="session.notice ? 'agents-sidebar-session-notice' : undefined"
                >
                  <IconCalendarClock
                    v-if="session.scheduleId"
                    class="size-3.5"
                    data-testid="agents-sidebar-session-scheduled"
                  />
                  <IconMessageSquare v-else class="size-3.5" />
                </span>
                <!-- @click.stop / @dblclick.stop: without them the row's own
                     handlers fire through the field and re-select the chat
                     mid-edit. -->
                <input
                  v-if="renamingSessionId === session.id"
                  v-model="renameDraft"
                  class="min-w-0 flex-1 bg-transparent text-[13px] text-text outline-none"
                  data-testid="agents-sidebar-session-rename-input"
                  autocapitalize="off"
                  autocorrect="off"
                  spellcheck="false"
                  autofocus
                  @click.stop
                  @dblclick.stop
                  @keydown.enter="commitRename"
                  @keydown.esc="renamingSessionId = null"
                  @blur="commitRename"
                >
                <span v-else class="min-w-0 flex-1 truncate">{{ session.name }}</span>
                <!-- The pin mark rides the name's line rather than the trailing
                     slot, which the status mark and the menu toggle already
                     share. -->
                <IconPin
                  v-if="isPinned(session.id)"
                  class="size-2.5 shrink-0 text-text-4"
                  title="Pinned to Code"
                  data-testid="agents-sidebar-session-pinned"
                />
                <span v-if="chatAge(session)" class="entry-age">{{ chatAge(session) }}</span>
                <!-- The way from stopped to running, which the kebab alone did
                     not state. It goes through the same select the row's own
                     click does — selecting a stopped chat resumes it. -->
                <button
                  v-if="!session.terminalId"
                  type="button"
                  class="entry-action"
                  :title="`Start ${session.name}`"
                  :aria-label="`Start ${session.name}`"
                  :disabled="startingSession"
                  data-testid="agents-sidebar-session-start"
                  @click.stop="emit('select-session', session)"
                ><IconPlay class="size-3" /></button>
                <!-- One cell, two occupants: the status is what the row says at
                     rest, the menu what it offers under the pointer. Neither
                     ever moves the name. -->
                <div class="entry-slot" @click.stop>
                  <span class="entry-status" aria-hidden="true">
                    <component
                      :is="sessionIndicators[session.id].icon"
                      v-if="sessionIndicators[session.id]"
                      class="size-3"
                      :class="[sessionIndicators[session.id].cls, { 'animate-spin': sessionIndicators[session.id].animated }]"
                      :title="sessionIndicators[session.id].label"
                      aria-hidden="true"
                    />
                    <span
                      v-else-if="session.terminalId"
                      class="size-2.5 rounded-full bg-severity-success"
                      title="Agent running"
                      data-testid="agents-sidebar-session-liveness"
                    />
                    <span
                      v-else
                      class="size-2.5 rounded-full border border-text-4"
                      title="Not running"
                      data-testid="agents-sidebar-session-idle"
                    />
                  </span>
                  <button
                    type="button"
                    class="entry-menu"
                    title="Chat actions"
                    aria-label="Chat actions"
                    aria-haspopup="menu"
                    :aria-expanded="openMenu === `s:${session.id}`"
                    data-testid="agents-sidebar-session-menu"
                    @click="openSessionMenu(session, $event)"
                  ><IconEllipsisVertical class="size-3" /></button>
                </div>
                <AppMenu
                  v-if="openMenu === `s:${session.id}`"
                  :entries="sessionMenuEntries(session)"
                  :anchor="menuAnchor"
                  :ignore="[menuToggle]"
                  testid="agents-sidebar-session-menu-panel"
                  @select="onSessionMenuSelect(session, $event)"
                  @close="closeSessionMenu"
                />
              </div>
              </template>
          </div>
        </div>
        <span
          class="tree-rail"
          :class="{ 'tree-rail-shown': rail.shown }"
          :style="{ transform: `translateY(${rail.y}px)`, height: `${rail.height}px` }"
          data-testid="agents-sidebar-session-rail"
          :data-shown="rail.shown"
          aria-hidden="true"
        />
        </div>
      </template>
    </div>
    <PanelResizeHandle edge="right" name="agents-sidebar" :start="startSidebarResize" :step="stepSidebar" />
  </aside>

  <ConfirmationDialog
    v-if="pendingDeleteSession"
    title="Delete chat?"
    :description="`Any live agent in ${pendingDeleteSession.name} is stopped and the chat is removed from the list.`"
    confirm-label="Delete chat"
    testid="agents-sidebar-delete-session-confirmation"
    @confirm="confirmDeleteSession"
    @cancel="pendingDeleteSession = null"
  />
</template>

<style scoped>
/* One workspace is one block, borrowed from the Code view's tree
   (TerminalMode.vue): the header keeps the sidebar's own surface, its chats sit
   in a recessed panel under it, and a rule closes each block off from the next.
   That banding is what separates the levels here, which is why the chats are
   not also indented under their header — the well already says what they belong
   to, and an indent would be the same statement made twice at the cost of a
   name's width in a sidebar this narrow. */
.ws-block { border-top: 1px solid var(--color-border); }
.ws-block-first { border-top: 0; }

.ws-row { display: flex; height: 40px; align-items: center; gap: 8px; padding: 0 12px; color: var(--color-text); font-size: 13px; font-weight: 500; cursor: pointer; }
.ws-row:hover { background: var(--color-chip); }
.ws-row:focus-visible { outline: 2px solid var(--color-accent); outline-offset: -2px; }

.ws-toggle { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 18px; height: 18px; border-radius: 5px; color: var(--color-text-4); cursor: pointer; }
.ws-toggle:hover { background: var(--color-app); color: var(--color-text); }

.ws-well { border-top: 1px solid var(--color-border); background: var(--color-app); padding: 6px 0; }

/* A chat row: SidebarFeedRow's .sidebar-entry stripped of its inset and its
   radius, because inside the well a row is full-bleed. Its leading glyph stays
   bare rather than framed in that component's bordered tile — a column of tiles
   reads as a stack of boxes before it reads as a list. */
.sidebar-entry { position: relative; display: flex; height: 34px; align-items: center; gap: 8px; padding: 0 12px; color: var(--color-text-2); font-size: 13px; cursor: pointer; }
.sidebar-entry:hover, .sidebar-entry.menu-open { background: var(--color-chip); color: var(--color-text); }
/* Kept where the Code view sets `outline: none`: that tree has a keyboard walk
   which activates the row it lands on, so its rail is already the mark
   following the cursor. Nothing walks this one — Tab moves through rows without
   selecting them, and dropping the ring would make that invisible. */
.sidebar-entry:focus-visible { outline: 2px solid var(--color-accent); outline-offset: -2px; }
/* The Code view's attached-row mark, unchanged: accent text and medium weight,
   and no fill at all. The rail below is what finds the row, and leaving the
   surface alone is also what lets a selected row keep its hover feedback.
   This is the sidebar's ONLY selection mark. A focused workspace deliberately
   draws nothing: focus is sticky — it lives on the route's :workspace param and
   the sidebar only ever moves it — so a header that showed it read as one row
   stuck lit from some earlier visit rather than as anything the user had just
   done. What focus actually changes is above the pane, not in here. */
.sidebar-entry-selected { color: var(--color-accent); font-weight: 500; }
.sidebar-entry-selected .nav-icon { color: var(--color-accent); }

/* A fixed cell rather than a shrink-wrapped glyph, so the chat names line up
   down the column whatever mark a row is showing. */
.nav-icon { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 18px; height: 18px; color: var(--color-text-4); }
.nav-icon-notice { color: var(--color-severity-warning); }

/* Revealed by opacity, not display, so every trailing column stays reserved:
   hovering a row never reflows the name or hides the count. */
.row-action { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 18px; height: 18px; border-radius: 5px; color: var(--color-text-4); cursor: pointer; opacity: 0; }
/* Darkening, not lightening: the row itself hovers to --color-chip, so a
   chip-coloured button would vanish into it. */
.row-action:hover { background: var(--color-app); color: var(--color-text); }
.ws-row:hover .row-action, .row-action:focus-visible { opacity: 1; }
.ws-row:hover .row-action:disabled { opacity: .4; cursor: default; }
.row-action:disabled:hover { background: none; color: var(--color-text-4); }

.entry-age { flex: none; font-family: var(--font-mono); font-size: 10.5px; color: var(--color-text-4); }

/* One 18px cell holding the status mark and the menu toggle, overlapped on the
   grid so swapping between them costs no layout anywhere on the row. Both
   stretch to fill the cell rather than shrink to their glyph, so the toggle is
   the size it looks in a 34px row.

   `pointer-events` and `z-index` below are load-bearing and easy to "clean up"
   into a bug. These two fade in and out with opacity, and **an element with
   opacity < 1 forms a stacking context**, which paints after in-flow
   block-level content. So the faded-out status mark paints ON TOP of the
   visible toggle and eats every click aimed at it — the click then dies on this
   cell's own @click.stop and nothing happens at all, while right-click still
   opens the menu because contextmenu is not stopped and reaches the row. The
   status mark is decoration (the row's own title already states liveness), so
   it opts out of hit-testing entirely, and the toggle is positioned so it wins
   the stack whichever of the two is currently transparent. Fading with
   `display` would also fix it, but reserving the column is why this uses
   opacity. */
/* Start, on a stopped chat. It holds its cell at rest like .row-action does,
   so appearing under the pointer never re-truncates the name beside it. */
.entry-action { display: inline-flex; flex: none; align-items: center; justify-content: center; width: 18px; height: 18px; border-radius: 5px; color: var(--color-text-4); cursor: pointer; opacity: 0; }
.entry-action:hover { background: var(--color-raised); color: var(--color-text); }
.entry-action:disabled { cursor: default; }
.entry-action:disabled:hover { background: none; color: var(--color-text-4); }
.sidebar-entry:hover .entry-action, .entry-action:focus-visible, .sidebar-entry.menu-open .entry-action { opacity: 1; }

.entry-slot { display: grid; flex: none; width: 18px; height: 18px; }
.entry-status, .entry-menu { grid-area: 1 / 1; width: 100%; height: 100%; }
.entry-status { display: inline-flex; align-items: center; justify-content: center; pointer-events: none; }
.entry-menu { position: relative; z-index: 1; display: inline-flex; align-items: center; justify-content: center; border-radius: 5px; color: var(--color-text-4); cursor: pointer; opacity: 0; }
.entry-menu:hover, .entry-menu[aria-expanded="true"] { background: var(--color-raised); color: var(--color-text); }
.sidebar-entry:hover .entry-status, .sidebar-entry.menu-open .entry-status { opacity: 0; }
.sidebar-entry:hover .entry-menu, .entry-menu:focus-visible, .sidebar-entry.menu-open .entry-menu { opacity: 1; }


/* TerminalMode.vue's tree-rail, verbatim. Square ends, and motion fast enough
   to read as the same mark relocating rather than a second one appearing. The
   z-index is load-bearing: the rows and the wells are painted boxes too, so
   without it the rail goes under them. */
.tree-rail {
  position: absolute; left: 0; top: 0; z-index: 1; width: 3px;
  background: var(--color-accent);
  opacity: 0;
  pointer-events: none;
  transition: transform .2s cubic-bezier(.2, 0, 0, 1), height .2s cubic-bezier(.2, 0, 0, 1), opacity .12s ease;
}
.tree-rail-shown { opacity: 1; }
@media (prefers-reduced-motion: reduce) {
  .tree-rail { transition: none; }
}

/* Lined up with a chat name: the row's 12px inset, its 18px glyph cell, and the
   8px between them. */
.chat-empty { padding: 7px 12px 7px 38px; font-size: 11.5px; font-style: italic; color: var(--color-text-4); }
</style>
