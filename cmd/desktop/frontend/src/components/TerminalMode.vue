<script setup lang="ts">
import InlineError from './ui/InlineError.vue'
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  shallowReactive,
  shallowRef,
  watch,
  watchEffect,
  type Component,
} from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useResizeObserver, useStorage } from '@vueuse/core'
import IconArrowDown from '~icons/lucide/arrow-down'
import IconMessagesSquare from '~icons/lucide/messages-square'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronUp from '~icons/lucide/chevron-up'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconChevronsDownUp from '~icons/lucide/chevrons-down-up'
import IconChevronsUpDown from '~icons/lucide/chevrons-up-down'
import IconCircleAlert from '~icons/lucide/circle-alert'
import IconCircleCheck from '~icons/lucide/circle-check'
import IconCircleOff from '~icons/lucide/circle-off'
import IconEllipsisVertical from '~icons/lucide/ellipsis-vertical'
import IconInfo from '~icons/lucide/info'
import IconListFilter from '~icons/lucide/list-filter'
import IconListTodo from '~icons/lucide/list-todo'
import IconLoaderCircle from '~icons/lucide/loader-circle'
import IconPencil from '~icons/lucide/pencil'
import IconPinOff from '~icons/lucide/pin-off'
import IconPlay from '~icons/lucide/play'
import IconPlus from '~icons/lucide/plus'
import IconRecycle from '~icons/lucide/recycle'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconRotateCw from '~icons/lucide/rotate-cw'
import IconSearch from '~icons/lucide/search'
import IconSquare from '~icons/lucide/square'
import IconTerminal from '~icons/lucide/terminal'
import IconTrash from '~icons/lucide/trash-2'
import IconX from '~icons/lucide/x'
import ActionInputsDialog from './ActionInputsDialog.vue'
import AppMenu from './ui/AppMenu.vue'
import AppTooltip from './ui/AppTooltip.vue'
import BaseButton from './ui/BaseButton.vue'
import ConfirmationDialog from './ui/ConfirmationDialog.vue'
import NewWindowMenu from './NewWindowMenu.vue'
import PaneStatusBar from './PaneStatusBar.vue'
import PanelResizeHandle from './ui/PanelResizeHandle.vue'
import SearchField from './ui/SearchField.vue'
import SessionDetailDialog from './SessionDetailDialog.vue'
import SessionRenameDialog from './SessionRenameDialog.vue'
import SessionRowMenu from './SessionRowMenu.vue'
import SessionStatusChips from './SessionStatusChips.vue'
import TerminalTab from './TerminalTab.vue'
import { formatCombo, useKeybindings } from '../composables/useKeybindings'
import { useCommands, useShellEscape, type Command } from '../composables/useCommands'
import { useTerminalActions } from '../composables/useTerminalActions'
import { useTerminalAvailability } from '../stores/useTerminalAvailability'
import {
  terminalSessionGroups,
  useTerminalSessions,
  type TerminalSessionGroup,
  type TerminalSessionRow,
} from '../stores/useTerminalSessions'
import { useTerminalPinnedChats } from '../stores/useTerminalPinnedChats'
import { useAgentSessionsAll } from '../stores/useAgentSessionsAll'
import { setAttachedTerminalWindows } from '../composables/useAttachedTerminalWindows'
import { useAgentWorkspaces } from '../stores/useAgentWorkspaces'
import { useTerminalPoolSize } from '../stores/useTerminalPoolSize'
import { useTerminalShowWindows } from '../stores/useTerminalShowWindows'
import { useTerminalWindowListings } from '../composables/useTerminalWindowListings'
import { useTerminalWindows, type TerminalWindowTab, type UseTerminalWindows } from '../composables/useTerminalWindows'
import { activePaneOf } from '../lib/terminalLayout'
import { useNewSession } from '../composables/useNewSession'
import { useResizablePanel } from '../composables/useResizablePanel'
import { useEditorSettings } from '../composables/useEditorSettings'
import { useSessionActions } from '../composables/useSessionActions'
import { useSessionStatus } from '../composables/useSessionStatus'
import { useSessionStatuses } from '../composables/useSessionStatuses'
import { useTerminalStatusBar } from '../stores/useTerminalStatusBar'
import { useWailsEvent } from '../composables/useWailsEvent'
import type { WindowForeground, WindowState } from '../lib/terminalClient'
import { appErrorMessage, errorText } from '../lib/appError'
import { isEditableTarget } from '../lib/isEditableTarget'
import { paneMayAutoFocus, setTerminalTreeHandles, terminalTreeFocused } from '../lib/terminalTree'
import {
  OpenSessionInEditor,
  RevealSession,
  SessionLaunchOptions,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type {
  SessionStatus,
  SessionWindowStatus,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import type { MenuEntry } from '../types/menu'
import Kbd from './ui/Kbd.vue'
import EmptyState from './ui/EmptyState.vue'
import '@xterm/xterm/css/xterm.css'

// `active` is whether this mode is the surface on screen. The component is
// mounted once and hidden on a trip to the hub (App.vue), so it is the signal
// that replaces mount/unmount for anything that must not run off-screen.
const props = withDefaults(
  defineProps<{
    sidebarCollapsed?: boolean
    active?: boolean
  }>(),
  { active: true },
)

const emit = defineEmits<{ 'open-tasks': []; 'session-repo-key': [repoKey: string] }>()

const { checking, available, reason, client, probe: probeAvailability } = useTerminalAvailability()

// Switching sessions must not blank the pane, so a switch no longer detaches:
// the last few attaches stay live in this pool — control client, stream and
// terminals intact, panes hidden — and snapping back to one is a v-show flip.
// Detach happens on eviction, explicit close, list removal, and unmount.
// The limit is Settings ▸ Terminal's warm-session count.
const { poolSize } = useTerminalPoolSize()
const pool = shallowReactive(new Map<string, UseTerminalWindows>())
const lastUsed: string[] = []
const activeSlug = ref('')
// What attachedRow (below) and current track is on-screen state: activeSlug,
// which the route drives and clears on a trip to the hub. The window-row
// projection (useAttachedTerminalWindows) names the attached session, not the
// visible one, so it is keyed off this instead — set on every real attach,
// left alone by the route-driven detach, and cleared only when dropSession
// lets that session's pool entry go.
const lastAttachedSlug = ref('')
const current = computed(() => (activeSlug.value ? (pool.get(activeSlug.value) ?? null) : null))
// What the main area shows. It lags the selection during a cold attach: the
// outgoing session holds the pane until the incoming one has painted — or
// ended, or the hold cap fired — so a switch never shows a blank grid.
const displayed = shallowRef<UseTerminalWindows | null>(null)
const displayedSlug = ref('')
const visible = computed(() => displayed.value ?? current.value)
// Which session the strip's windows belong to, which is not always the selected
// one: a reorder started during a hold must reach the session on screen.
const visibleSlug = computed(() => (displayed.value ? displayedSlug.value : activeSlug.value))

const renamingId = ref('')
const renameDraft = ref('')

// How long the outgoing session may stand in for one that has not painted:
// long enough to cover a normal attach, short enough that a session with an
// empty screen — which sends no first paint at all — does not read as a dead
// click. Reached only on cold attaches; a pooled session reveals instantly.
const HOLD_MS = 300
const revealed = new WeakSet<UseTerminalWindows>()
let holdTimer: ReturnType<typeof setTimeout> | undefined

watch(
  [current, () => current.value?.painted.value, () => current.value?.status.value],
  () => {
    clearTimeout(holdTimer)
    const incoming = current.value
    if (!incoming || incoming === displayed.value) return
    if (!displayed.value || revealed.has(incoming) || incoming.painted.value || incoming.status.value === 'ended') {
      reveal(incoming)
      return
    }
    holdTimer = setTimeout(() => {
      if (current.value === incoming) reveal(incoming)
    }, HOLD_MS)
  },
  { immediate: true },
)

function reveal(incoming: UseTerminalWindows): void {
  revealed.add(incoming)
  displayed.value = incoming
  // Only ever the selected session reaches here, so activeSlug is its key.
  displayedSlug.value = activeSlug.value
  void nextTick(() => {
    if (displayed.value === incoming && paneMayAutoFocus.value) incoming.focusActive()
  })
}

function touchPool(slug: string): void {
  const at = lastUsed.indexOf(slug)
  if (at !== -1) lastUsed.splice(at, 1)
  lastUsed.push(slug)
  evictOverLimit()
}

// An evicted session's subtree falls back to the cached listing, which predates
// every window opened or closed while it was attached, so eviction re-sweeps.
function evictOverLimit(): void {
  let evicted = false
  for (const victim of [...lastUsed]) {
    if (pool.size <= poolSize.value) break
    if (victim !== activeSlug.value && pool.get(victim) !== displayed.value) {
      dropSession(victim)
      evicted = true
    }
  }
  if (evicted) sweepListings()
}

// Shrinking the setting takes effect without a re-entry; growing it simply
// leaves room for the next attaches.
watch(poolSize, evictOverLimit)

// The only way out of the pool, so every exit funnels through here: eviction,
// explicit close, and sessions the listing no longer carries.
function dropSession(slug: string): void {
  const entry = pool.get(slug)
  if (!entry) return
  if (displayed.value === entry) displayed.value = null
  entry.dispose()
  pool.delete(slug)
  const at = lastUsed.indexOf(slug)
  if (at !== -1) lastUsed.splice(at, 1)
  // This is the pool's one real exit, so it is where the projection's own
  // attach reference gives up on a session too — never on the route-driven
  // detach, which leaves the pool (and this) untouched.
  if (lastAttachedSlug.value === slug) lastAttachedSlug.value = ''
}

const route = useRoute()
const router = useRouter()

// The URL is the attach state: /terminal/:slug is the attached session,
// ?window its active window. Sidebar clicks push (history traverses session
// switches); window changes replace (tab flips must not pile up entries).
const routeSlug = computed(() =>
  route.name === 'terminal' && typeof route.params.slug === 'string' ? route.params.slug : '',
)
const routeWindow = computed(() => (typeof route.query.window === 'string' ? route.query.window : ''))

// Shared by the attach path (openSession, on a fresh or already-pooled slug)
// and the same-slug case (the watch below, for a push that only changes
// ?window): a wanted window is worth a select exactly when it names a tab
// this session actually has and is not already showing.
function selectIfWanted(session: UseTerminalWindows, windowId: string): void {
  if (!windowId || windowId === session.activeWindowId.value) return
  if (session.tabs.value.some((tab) => tab.windowId === windowId)) void session.select(windowId)
}

// The resume snapshot: entering bare /terminal re-attaches this instead of
// landing on the picker. Cleared when the session is closed on purpose or no
// longer exists.
const restore = useStorage('hive.terminal.restore', { slug: '', window: '' })

const {
  sessions: sessionRows,
  scratch: scratchRow,
  loading: sessionsLoading,
  loaded: sessionsLoaded,
  error: sessionsError,
  reload: reloadSessions,
  sessionRepository,
} = useTerminalSessions()
const { openBlank: openNewSession, prefetch: prefetchNewSession } = useNewSession()
// The tree is the attach surface, so only an active session belongs in it — a
// recycled or corrupted one has no checkout left to open a terminal in, and
// attaching cannot start one. They still arrive in the listing, which is what
// the header's prune entry counts and acts on.
const activeSessions = computed(() => sessionRows.value.filter((row) => row.state === 'active'))
// A pinned agent chat is a tmux session addressed exactly like a hive one and
// the Code view attaches it through the same pool (ADR agent-workspace-sessions-are-tmux-sessions). What it is not is a
// hive session: nothing in hive's listing or status projection knows about it,
// which is the axis `isHiveSession` below splits on.
const { rows: chatRows, slugs: chatSlugs, unpinSlug } = useTerminalPinnedChats()
const { recents, reload: reloadRecents } = useAgentSessionsAll()
const { resumeSession: resumeChat } = useAgentWorkspaces()

// The scratch terminal and the pinned chats are attachable like any session and
// deliberately part of this set rather than beside it: the pool, the window sweep
// and the watcher that lets go of a session the listing stopped carrying all read
// it, and a row missing from here would have its attach dropped on the next
// reload. Unpinning a chat is exactly that removal, which is what detaches it.
const attachable = computed(() => [
  ...chatRows.value,
  ...(scratchRow.value ? [scratchRow.value] : []),
  ...activeSessions.value,
])
const sessionGroups = computed(() => terminalSessionGroups(activeSessions.value, scratchRow.value, chatRows.value))

// Rows whose liveness hive can answer for. The scratch terminal and a pinned
// chat are tmux sessions hive holds no record of, so they read theirs off the
// window sweep and their own attach instead of the status projection.
function isHiveSession(row: TerminalSessionRow): boolean {
  return !isScratch(row) && !chatSlugs.value.has(row.slug)
}

function isChat(row: TerminalSessionRow): boolean {
  return chatSlugs.value.has(row.slug)
}

// The rows the sweep asks about whatever "always show windows" says, because
// the tree has no other way to learn whether tmux is holding one.
const alwaysSwept = computed(() => [...chatRows.value, ...(scratchRow.value ? [scratchRow.value] : [])])
// Counted off the listing rather than as the remainder of the tree: the scratch
// terminal is in the tree and in no listing, and prune only ever means hive's
// recycled and corrupted sessions.
const prunableCount = computed(() => sessionRows.value.filter((row) => row.state !== 'active').length)

function isScratch(row: TerminalSessionRow): boolean {
  return !!scratchRow.value && row.slug === scratchRow.value.slug
}

// What the sidebar says under a tree it has already drawn. The tree is never
// replaced by a note now that the pinned sections are in it, so the two states
// that used to stand in for it are named here instead of read off the same
// v-if chain.
const treeNote = computed<'' | 'empty' | 'no-matches'>(() => {
  if (sessionsError.value || !treeReady.value) return ''
  if (!activeSessions.value.length) return 'empty'
  // The pinned sections are exempt from the running filter, so neither can stand
  // in as something that filter found: leaving no repository behind is leaving
  // the tree with nothing it was asked about. A query does reach them, so one
  // that matched has found what it went looking for.
  const found = sessionFilter.value.trim()
    ? filteredGroups.value.length > 0
    : filteredGroups.value.some((group) => group.kind === 'repo')
  return found ? '' : 'no-matches'
})

// The sidebar filter. It narrows what the tree draws and nothing else:
// `attachable` keeps every session, because the watcher that follows a rename
// or a deletion reads it, and a session filtered off the screen must not read
// as one that went away. The attached session is not exempt either — filtering
// is a way to look at the list, not a way to detach.
//
// Transient, deliberately not stored: a filter restored at launch hides
// sessions the user has no reason to suspect are there.
const sessionFilter = ref('')
const filterInput = ref<{ select: () => void } | null>(null)

// The other axis the tree narrows on: the query picks a session by name, this
// picks by whether tmux is holding one, and it cuts repositories as well as
// sessions — a dormant repository's header is most of what a full tree costs
// to read.
//
// Stored, unlike the query, because it is how the user keeps the tree rather
// than a search to be undone. The banner below the header is the price of
// that: a narrowed tree must never read as a short one.
const runningOnly = useStorage('hive.terminal.sidebar.running-only', false)

// Taken before the query, so a repo-name match carries what is left rather
// than what the tree started with. Inert until the first status poll lands:
// a session nothing has answered for yet is not a session doing nothing.
const runningGroups = computed<TerminalSessionGroup[]>(() => {
  if (!runningOnly.value || !statusesLoaded.value) return sessionGroups.value
  const groups: TerminalSessionGroup[] = []
  for (const group of sessionGroups.value) {
    // The pinned sections stay: the scratch section is the tree's one
    // always-there way to open a shell, and hiding it leaves nothing to start one
    // from, while a chat was pinned precisely to be kept in view. A query still
    // reaches both — that is a search for a name, not a view of the list.
    if (group.kind !== 'repo') {
      groups.push(group)
      continue
    }
    const sessions = group.sessions.filter(rowRunning)
    if (sessions.length) groups.push({ ...group, sessions })
  }
  return groups
})

function countSessions(groups: TerminalSessionGroup[]): number {
  return groups.reduce((total, group) => total + group.sessions.length, 0)
}

const runningNote = computed(() => {
  if (!runningOnly.value) return ''
  const hidden = countSessions(sessionGroups.value) - countSessions(runningGroups.value)
  return hidden ? `Running sessions only · ${hidden} hidden` : 'Running sessions only'
})

// Why the tree came back empty, in the terms of whichever narrowing emptied it.
const noMatchesNote = computed(() => {
  const query = sessionFilter.value.trim()
  return query ? `No sessions match “${query}”.` : 'No sessions are running.'
})

// A repo match carries its whole group, which is what makes typing a repo name
// a way to narrow to it. Sessions match on the name and on the slug, since the
// slug is the tmux target and what a deep link or a script names.
const filteredGroups = computed<TerminalSessionGroup[]>(() => {
  const query = sessionFilter.value.trim().toLowerCase()
  if (!query) return runningGroups.value
  const groups: TerminalSessionGroup[] = []
  for (const group of runningGroups.value) {
    if (group.name.toLowerCase().includes(query)) {
      groups.push(group)
      continue
    }
    const sessions = group.sessions.filter(
      (row) => row.name.toLowerCase().includes(query) || row.slug.toLowerCase().includes(query),
    )
    if (sessions.length) groups.push({ ...group, sessions })
  }
  return groups
})

const {
  statuses: sessionStatuses,
  loaded: statusesLoaded,
  startPolling: startStatusPolling,
  stopPolling: stopStatusPolling,
} = useSessionStatuses()
interface StatusIndicator {
  icon: Component
  color: string
  label: string
  animated?: boolean
}
// Idle is carried by the row's own name rather than a glyph — undefined until
// the first status poll lands, so a row is not greyed before its state is known.
const sessionIdle = computed<Record<string, boolean>>(() =>
  Object.fromEntries(Object.entries(sessionStatuses.value).map(([id, status]) => [id, !status.running])),
)

function windowActivityIndicator(status: SessionWindowStatus): StatusIndicator {
  const tool = status.tool || 'Agent'
  switch (status.status) {
    case 'active':
      return { icon: IconLoaderCircle, color: 'text-severity-success', label: `${tool} is working`, animated: true }
    case 'approval':
      return { icon: IconCircleAlert, color: 'text-severity-warning', label: `${tool} needs approval` }
    case 'ready':
      return { icon: IconCircleCheck, color: 'text-text-2', label: `${tool} is ready` }
    default:
      return { icon: IconCircleOff, color: 'text-text-4', label: `${tool} status unavailable` }
  }
}

function windowIndicator(sessionID: string, windowID: string): StatusIndicator | null {
  const status = sessionStatuses.value[sessionID]?.windows?.find((window) => window.windowId === windowID)
  return status ? windowActivityIndicator(status) : null
}

// The configured actions from actions.yml that declare a terminal target. They
// land in the session row's menu under its own operations, and are the whole
// contents of a window row's menu — so a window row grows one only when there
// is something to put in it.
const {
  load: loadTerminalActions,
  sessionEntries: sessionActionEntries,
  windowEntries: windowActionEntries,
  hasWindowActions,
  select: selectTerminalAction,
  pendingInputs: actionInputs,
  inputsBusy: actionInputsBusy,
  inputsError: actionInputsError,
  cancelInputs: cancelActionInputs,
  submitInputs: submitActionInputs,
} = useTerminalActions()
useWailsEvent('actions:updated', () => {
  void loadTerminalActions()
})

const openRowMenu = ref('')
const openNewWindowMenu = ref('')
const newWindowToggle = ref<HTMLElement | null>(null)
const newWindowMenuFlip = ref(false)
const agentWindowBusy = ref(false)
const agentProfiles = ref<string[]>([])
const defaultAgentProfile = ref('')
const agentProfilesLoading = ref(false)
const agentProfilesFailed = ref(false)
let agentProfilesRequest: Promise<void> | null = null

function loadAgentProfiles(): Promise<void> {
  if (agentProfilesRequest) return agentProfilesRequest
  agentProfilesLoading.value = true
  agentProfilesFailed.value = false
  agentProfilesRequest = SessionLaunchOptions()
    .then((options) => {
      agentProfiles.value = options.agents ?? []
      defaultAgentProfile.value = options.defaultAgent
    })
    .catch(() => {
      agentProfilesFailed.value = true
    })
    .finally(() => {
      agentProfilesLoading.value = false
      agentProfilesRequest = null
    })
  return agentProfilesRequest
}

watch(
  () => props.active,
  (active) => {
    if (active) void loadAgentProfiles()
  },
  { immediate: true },
)
const rowMenuFlip = ref(false)
const rowMenuToggles = new Map<string, HTMLElement>()
const openWindowMenu = ref('')
const windowMenuFlip = ref(false)
const windowMenuToggles = new Map<string, HTMLElement>()
const sidebarMenuOpen = ref(false)
const sidebarMenuToggle = ref<HTMLElement | null>(null)
const sidebarMenuEntries = computed<MenuEntry[]>(() => [
  {
    kind: 'action',
    id: 'running-only',
    label: 'Only running sessions',
    checked: runningOnly.value,
    testid: 'terminal-sessions-running-only',
  },
  { kind: 'separator' },
  {
    kind: 'action',
    id: 'collapse-all',
    label: 'Collapse all',
    icon: IconChevronsDownUp,
    testid: 'terminal-sessions-collapse-all',
  },
  {
    kind: 'action',
    id: 'expand-all',
    label: 'Expand all',
    icon: IconChevronsUpDown,
    testid: 'terminal-sessions-expand-all',
  },
  { kind: 'separator' },
  {
    kind: 'action',
    id: 'prune',
    label: prunableCount.value ? `Prune ${prunableCount.value} recycled…` : 'Nothing to prune',
    icon: IconTrash,
    testid: 'terminal-sessions-prune',
  },
])

const {
  confirmation,
  detail: sessionDetail,
  openDetail: openSessionDetail,
  closeDetail: closeSessionDetail,
  renaming,
  renameBusy,
  renameError,
  requestRename,
  cancelRename,
  submitRename,
  requestDelete,
  requestRecycle,
  requestPrune,
} = useSessionActions({
  onChanged: () => {
    void reloadSessions()
  },
})
const {
  open: confirmOpen,
  options: confirmOptions,
  busy: confirmBusy,
  error: confirmError,
  cancel: cancelConfirm,
  confirm: runConfirm,
} = confirmation

function setRowMenuToggle(id: string, el: unknown): void {
  if (el instanceof HTMLElement) rowMenuToggles.set(id, el)
  else rowMenuToggles.delete(id)
}

function setWindowMenuToggle(id: string, el: unknown): void {
  if (el instanceof HTMLElement) windowMenuToggles.set(id, el)
  else windowMenuToggles.delete(id)
}

// The sidebar is a scroll container, so an overflowing menu is clipped rather
// than allowed to hang outside it: open upward near the bottom of the window.
function menuFlipsUp(toggle: HTMLElement | undefined): boolean {
  const rect = toggle?.getBoundingClientRect()
  return rect != null && window.innerHeight - rect.bottom < 200 && rect.top > 200
}

function toggleRowMenu(row: TerminalSessionRow, event?: MouseEvent): void {
  openNewWindowMenu.value = ''
  if (openRowMenu.value === row.id && !event) {
    openRowMenu.value = ''
    return
  }
  openWindowMenu.value = ''
  rowMenuFlip.value = menuFlipsUp(
    event?.currentTarget instanceof HTMLElement ? event.currentTarget : rowMenuToggles.get(row.id),
  )
  openRowMenu.value = row.id
}

// The separator is escaped, never typed: a literal NUL in the source makes the
// whole file read as binary, and every grep over it comes back empty.
function windowMenuKey(row: TerminalSessionRow, windowId: string): string {
  return `${row.slug}\u0000${windowId}`
}

// A configured action renders over the session it was invoked on, and the
// scratch terminal has none — no checkout, no remote, no record to template
// from — so its tabs offer no actions rather than failing one.
function rowHasWindowActions(row: TerminalSessionRow): boolean {
  return hasWindowActions.value && !isScratch(row)
}

function toggleWindowMenu(row: TerminalSessionRow, windowId: string, event?: MouseEvent): void {
  if (!rowHasWindowActions(row)) return
  const key = windowMenuKey(row, windowId)
  if (openWindowMenu.value === key && !event) {
    openWindowMenu.value = ''
    return
  }
  openRowMenu.value = ''
  windowMenuFlip.value = menuFlipsUp(
    event?.currentTarget instanceof HTMLElement ? event.currentTarget : windowMenuToggles.get(key),
  )
  openWindowMenu.value = key
}

function runSessionAction(row: TerminalSessionRow, entryID: string): void {
  void selectTerminalAction('session', entryID, { slug: row.slug, windowId: '' })
}

function runWindowAction(row: TerminalSessionRow, windowId: string, entryID: string): void {
  openWindowMenu.value = ''
  void selectTerminalAction('window', entryID, { slug: row.slug, windowId })
}

function onSidebarMenuSelect(id: string): void {
  sidebarMenuOpen.value = false
  if (id === 'running-only') runningOnly.value = !runningOnly.value
  else if (id === 'collapse-all' || id === 'expand-all') setAllGroups(id === 'expand-all')
  else if (id === 'prune' && prunableCount.value) requestPrune(prunableCount.value)
}

// A chat's own lifecycle — rename, stop, delete — stays in the Chats area, which
// owns its record; what this menu offers is the two things only the pin created:
// the way back to that area, and the way to undo it.
const chatMenuEntries: MenuEntry[] = [
  {
    kind: 'action',
    id: 'open-in-agents',
    label: 'Open in Chats',
    icon: IconMessagesSquare,
    testid: 'terminal-chat-open-in-agents',
  },
  { kind: 'separator' },
  { kind: 'action', id: 'unpin', label: 'Unpin from Code', icon: IconPinOff, testid: 'terminal-chat-unpin' },
]

// Unpinning drops the row from `attachable`, which is what detaches it — the
// pooled attach is released by the same watcher a deleted session goes through.
// The chat itself is untouched: the agent keeps running, which is the whole point
// of a tmux session outliving its clients (ADR agent-workspace-sessions-are-tmux-sessions).
function onChatMenuSelect(row: TerminalSessionRow, id: string): void {
  openRowMenu.value = ''
  if (id === 'unpin') unpinSlug(row.slug)
  else if (id === 'open-in-agents') openChatInAgents(row)
}

function openChatInAgents(row: TerminalSessionRow): void {
  const session = recents.value.find((candidate) => candidate.slug === row.slug)
  if (!session) return
  void router.push({ name: 'agents', params: { workspace: session.workspace }, query: { chat: String(session.id) } })
}

function groupAttached(group: TerminalSessionGroup): boolean {
  return group.sessions.some((row) => row.slug === activeSlug.value)
}

function groupRunning(group: TerminalSessionGroup): boolean {
  return group.sessions.some(rowRunning)
}

// Expand/collapse is transient view state, not configuration — localStorage,
// same as the hub sidebar's folder collapse. A repo the user has never toggled
// has no entry and takes the default, which is open only where something is
// live: a machine's worth of dormant repos would otherwise bury the one being
// worked in. The attached repo counts as live on its own, because statuses
// arrive a poll after the tree does and the repo on screen must not wait.
const groupExpansion = useStorage<Record<string, boolean>>('hive.terminal.sidebar.groups', {})
function groupExpanded(group: TerminalSessionGroup): boolean {
  // A filter overrides the stored state: a group is only in the list because
  // something in it matched, and a collapsed one would hide the match.
  if (sessionFilter.value.trim()) return true
  // Both pinned sections default open — free space nobody can see is not free
  // space. For the scratch section the row is its heading, so this is what its
  // chevron folds: the tabs under it, not the row itself.
  if (group.kind !== 'repo') return groupExpansion.value[group.key] ?? true
  return groupExpansion.value[group.key] ?? (groupAttached(group) || groupRunning(group))
}
function toggleGroup(group: TerminalSessionGroup): void {
  groupExpansion.value[group.key] = !groupExpanded(group)
}

// Every group, not the ones the tree happens to be drawing: a bulk fold that
// the narrowed-away groups escaped would spring back open as a surprise the
// moment the scope or the query came off.
function setAllGroups(expanded: boolean): void {
  for (const group of sessionGroups.value) groupExpansion.value[group.key] = expanded
}

// Windows are only known live through an attach, so every other active
// session's come from a one-shot listing per session — fetched only while the
// Settings ▸ Terminal option is on, and refreshed whenever the
// session list or the attached slug changes.
const { showWindows: showAllWindows, ready: showAllWindowsReady } = useTerminalShowWindows()
const { listings: sessionWindows, settled: listingsSettled, refresh: refreshListings } = useTerminalWindowListings()
// Not keyed on the attached slug: attaching to one session cannot change
// another's window list, and the attached one's own tabs come from its live
// client. Sweeping every session on every switch was pure cost on the path
// the switch itself was waiting on.
// Gated on `active` as well: an unattached session answers a listing by
// spawning tmux twice, and none of it is on screen while the hub is.
// Held until the session list has landed as well: sweeping the rows we happen
// to hold when the client appears asks about a set we already know is stale,
// and the answer would report the listings as settled before the real ones are
// even in flight.
// The scratch terminal and the pinned chats are swept whatever the setting says:
// the tree has no other way to know whether tmux is holding one, and a sweep is
// one tmux call for every slug in it, so asking about a few more costs nothing.
function sweepListings(): void {
  const transport = client.value
  if (!props.active || !transport || !sessionsLoaded.value) return
  if (showAllWindows.value) void refreshListings(transport, attachable.value)
  else if (alwaysSwept.value.length) void refreshListings(transport, alwaysSwept.value)
}

watch([showAllWindows, attachable, client, sessionsLoaded, () => props.active], sweepListings)

// The chat rows come from the Agents area's own listing, which nothing else in
// this mode reads: without this a pinned chat's name and liveness would be
// whatever they were when the Agents area was last on screen.
watch(
  () => props.active,
  (active) => {
    if (active) void reloadRecents()
  },
  { immediate: true },
)

// Only a hive session gets a status bar: the scratch terminal and the pinned
// chats are tmux sessions with no checkout behind them.
const { showStatusBar } = useTerminalStatusBar()
const { title: editorTitle, refresh: reloadEditor } = useEditorSettings()
watch(
  () => props.active,
  (active) => {
    if (active) void reloadEditor()
  },
  { immediate: true },
)

const statusBarRow = computed(() => {
  if (!showStatusBar.value || !visibleSlug.value) return null
  return activeSessions.value.find((row) => row.slug === visibleSlug.value) ?? null
})
const statusBarSessionId = computed(() => statusBarRow.value?.id ?? '')
const {
  git: sessionGit,
  pullRequest: sessionPullRequest,
  pullRequestError: sessionPullRequestError,
  refresh: refreshSessionStatus,
} = useSessionStatus(statusBarSessionId)
// Separate from actionError, which belongs to the terminal actions menu.
const statusBarError = ref('')

async function runStatusBarAction(action: (id: string) => Promise<void>): Promise<void> {
  const id = statusBarSessionId.value
  if (!id) return
  statusBarError.value = ''
  try {
    await action(id)
  } catch (error) {
    statusBarError.value = error instanceof Error ? error.message : String(error)
  }
}

// owner/repo is exactly the hc repoKey format; a git read that has not
// resolved, or resolved onto a remote that names no host, has neither. Reported
// continuously rather than only on click, so App.vue can scope Tasks to the
// attached session's repo from any entry point (titlebar, keybinding,
// palette) and not only a click on this bar's own button.
const sessionRepoKey = computed(() => {
  const git = sessionGit.value
  return git?.resolved && git.owner && git.repo ? `${git.owner}/${git.repo}` : ''
})
watch(sessionRepoKey, (key) => emit('session-repo-key', key), { immediate: true })

// A session dying moves nothing the sweep above watches — not the session set,
// not the pool, not the setting — while its last window closing empties the
// live tab set that was standing in front of the listing. The subtree would
// fall back to a cached listing of windows tmux no longer holds, so an end is
// a trigger of its own.
const endedSlugs = computed(() =>
  [...pool.entries()]
    .filter(([, session]) => session.status.value === 'ended')
    .map(([slug]) => slug)
    .join(' '),
)
watch(endedSlugs, sweepListings)

// The tree used to paint the moment the session list landed, then paint again
// for the window listings, the setting that decides whether they show at all,
// and the attach — four passes over every row, each one animating and
// relayouting the whole panel. The placeholder holds until a row's final shape
// is known, so the tree arrives once instead.
//
// One-shot: this is the cost of the first fill, not a state to return to. A
// later reload revalidates the tree already on screen, and toggling the window
// listing off and on again must not blank it.
const treeReady = ref(false)
watchEffect(() => {
  if (treeReady.value || !sessionsLoaded.value || !showAllWindowsReady.value) return
  // The filter decides which rows exist at all, so a tree painted before the
  // first status poll would be the wrong tree, not an early one.
  if (runningOnly.value && !statusesLoaded.value) return
  // Whatever the sweep is answering — every session's windows, or only whether
  // the scratch terminal is running — the tree waits for it, because both change
  // a row's final shape.
  const sweeping = attachable.value.length > 0 && (showAllWindows.value || alwaysSwept.value.length > 0)
  if (sweeping && !listingsSettled.value) return
  treeReady.value = true
})

// Motion stays off until the tree has been through a frame: on the first paint
// every row is an enter, so the panel would animate in as one block, and the
// rails measure per frame and would force layout through all of it. The
// transitions exist to make a *change* legible, and the first fill is not one.
const treeSettled = ref(false)
watch(
  treeReady,
  (ready) => {
    if (!ready) return
    void nextTick(() =>
      requestAnimationFrame(() =>
        requestAnimationFrame(() => {
          treeSettled.value = true
          settleRails()
        }),
      ),
    )
  },
  { immediate: true },
)

// The scratch section is exempt from "always show windows": that setting is about
// how much of every session to list, and the scratch terminal's tabs are the
// section itself — hiding them leaves a heading that says nothing.
function listedWindows(row: TerminalSessionRow): WindowState[] {
  if (!showAllWindows.value && !isScratch(row)) return []
  return sessionWindows.value[row.slug] ?? []
}

// Whether tmux is holding a session for a row. Hive's status projection is keyed
// by session id and knows nothing about the scratch terminal or a pinned chat, so
// those rows read their liveness off the window listing — swept for them whatever
// the "always show windows" setting says — and off their own live attach.
function rowRunning(row: TerminalSessionRow): boolean {
  if (isHiveSession(row)) return !!sessionStatuses.value[row.id]?.running
  if (sessionWindows.value[row.slug]?.length) return true
  const live = pool.get(row.slug)
  return !!live && live.status.value !== 'ended'
}

// Greyed only once its state is known: the status poll for a session, the first
// window sweep for the scratch terminal and the pinned chats.
function rowIdle(row: TerminalSessionRow): boolean {
  if (isHiveSession(row)) return !!sessionIdle.value[row.id]
  return listingsSettled.value && !rowRunning(row)
}

// One keyed list feeds a session's subtree whichever source is fresher, so the
// listed → live swap on attach patches rows in place — window ids are stable
// across the swap — instead of unmounting one branch and mounting the other.
interface TreeWindowRow {
  windowId: string
  name: string
  active: boolean
  live: boolean
  indicator: StatusIndicator | null
}

// Keyed by session id, derived once per invalidation. The template reads a
// session's rows three times per render and treeRowKeys reads every session's,
// so deriving them per call meant the whole tree re-ran — and re-allocated —
// on every status poll, and the row count made the innermost read quadratic.
const windowRows = computed<Record<string, TreeWindowRow[]>>(() => {
  const rows: Record<string, TreeWindowRow[]> = {}
  for (const row of attachable.value) rows[row.id] = buildWindowRows(row)
  return rows
})

function windowRowsFor(row: TerminalSessionRow): TreeWindowRow[] {
  return windowRows.value[row.id] ?? []
}

// A pooled session's live tab set is fresher than its listing — but while its
// attach is still in flight, the cached listing stands in so selecting a
// session does not collapse its subtree.
// A chat is one conversation, so its tmux window is a fact about how that is
// carried rather than something to navigate between: the row is a leaf.
function buildWindowRows(row: TerminalSessionRow): TreeWindowRow[] {
  if (isChat(row)) return []
  const live = pool.get(row.slug)
  if (live?.tabs.value.length && (row.slug === activeSlug.value || showAllWindows.value || isScratch(row))) {
    return live.tabs.value.map((tab) => ({
      windowId: tab.windowId,
      name: tab.name || tab.windowId,
      active: row.slug === activeSlug.value && tab.windowId === live.activeWindowId.value,
      live: true,
      indicator: windowIndicator(row.id, tab.windowId),
    }))
  }
  return listedWindows(row).map((win) => ({
    windowId: win.windowId,
    name: win.name || win.windowId,
    active: false,
    live: false,
    indicator: windowIndicator(row.id, win.windowId),
  }))
}

// The mouse's path, shared with Enter: both mean "go to work in this", so the
// pane is allowed to take focus as it comes up.
function openTreeWindow(row: TerminalSessionRow, win: TreeWindowRow): void {
  cursorRequest.value = `w:${row.id}:${win.windowId}`
  paneMayAutoFocus.value = true
  if (win.live && row.slug === activeSlug.value) void current.value?.select(win.windowId)
  else openWindow(row, win.windowId)
}

function selectSessionRow(row: TerminalSessionRow): void {
  cursorRequest.value = `s:${row.id}`
  paneMayAutoFocus.value = true
  selectSession(row.slug)
}

// Committing on a row the walk only stops at because there is no terminal behind
// it: starting one is the move on offer, so Enter makes it without the trip
// through the empty pane. A click still just opens the session — the pane's own
// Start button is right there, and the mouse has not asked for anything else.
// startSession does not respawn a live session, so a status poll that has not
// landed yet costs nothing.
function enterSessionRow(row: TerminalSessionRow): void {
  cursorRequest.value = `s:${row.id}`
  paneMayAutoFocus.value = true
  if (rowRunning(row)) selectSession(row.slug)
  else void startSession(row.slug)
}

// ── Tree keyboard navigation ─────────────────────────────────────────────────
// An arrow is a click on the next row, not a cursor that moves ahead of the
// selection: the keyboard and the mouse pick a session the same way, so there
// is one selected row rather than a selection and a pending cursor to reconcile.
//
// Only rows a click does something to are walked. A repo header collapses on
// click, which is not something to do to every group on the way past one, so it
// stays mouse- and Tab-reachable.
//
// A running session is its windows, so its own row drops out of the walk rather
// than standing in front of them as a stop that changes nothing. What is left is
// the rule the tree reads by: a session row is a stop exactly when Enter has a
// session to start. The `windows.length` guard is the exception that keeps
// everything reachable — with window listing off, an unattached session has no
// windows to stand in for it.
//
// Rows are keyed rather than indexed: the tree re-sorts under a poll, and an
// index would silently point at a different session afterwards.
const treeRowKeys = computed<string[]>(() => {
  const keys: string[] = []
  for (const group of filteredGroups.value) {
    if (!groupExpanded(group)) continue
    for (const row of group.sessions) {
      const windows = windowRowsFor(row)
      if (!rowRunning(row) || !windows.length) keys.push(`s:${row.id}`)
      for (const win of windows) keys.push(`w:${row.id}:${win.windowId}`)
    }
  }
  return keys
})

const attachedRow = computed(() => attachable.value.find((row) => row.slug === activeSlug.value) ?? null)

// Where the keyboard is, read off what is attached — a session's active window
// if it has one, the session itself otherwise.
const selectedRowKey = computed(() => {
  const row = attachedRow.value
  if (!row) return ''
  const active = windowRowsFor(row).find((win) => win.active)
  return active ? `w:${row.id}:${active.windowId}` : `s:${row.id}`
})

// The row last landed on, by either input. It cannot be derived from the
// selection: attaching a session also selects its active window, so a tree
// position read back off the selection would sit one row below the session row
// the arrow just picked, and the next arrow would skip that window.
// Both the arrows and the row handlers set it, which is what keeps a click and a
// keypress agreeing on where the next arrow starts.
const cursorRequest = ref('')

const cursorKey = computed(() => {
  const keys = treeRowKeys.value
  if (keys.includes(cursorRequest.value)) return cursorRequest.value
  if (keys.includes(selectedRowKey.value)) return selectedRowKey.value
  return ''
})

// Nothing is selected on the way in, so the first row carries the tab stop and
// the first arrow moves from there.
const tabStopKey = computed(() => cursorKey.value || treeRowKeys.value[0] || '')

const sidebarEl = ref<HTMLElement | null>(null)

// The chord worth showing is the one that leaves where focus already is —
// "⌘← tree" is noise to someone standing in the tree.
const { combosFor } = useKeybindings()
const focusHint = computed(() => {
  const inTree = terminalTreeFocused.value
  const combo = combosFor(inTree ? 'terminal.focus-pane' : 'terminal.focus-sidebar')[0]
  if (!combo) return null
  return { keys: formatCombo(combo), label: inTree ? 'terminal' : 'tree' }
})

function rowElement(key: string): HTMLElement | null {
  if (!key) return null
  const rows = treeContent.value?.querySelectorAll<HTMLElement>('[data-tree-key]')
  if (!rows) return null
  for (const row of Array.from(rows)) if (row.dataset.treeKey === key) return row
  return null
}

// The click handlers, reached by key. Re-picking the attached session is the one
// place the two part company: a click there means "put me back in the pane", and
// an arrow that did the same would hand the next keystroke to tmux.
async function activateRow(key: string): Promise<void> {
  const [kind, rowID, windowId] = key.split(':')
  const row = attachable.value.find((candidate) => candidate.id === rowID)
  if (!row) return
  if (kind === 's') {
    if (row.slug !== activeSlug.value) selectSession(row.slug)
    return
  }
  const win = windowRowsFor(row).find((candidate) => candidate.windowId === windowId)
  if (!win) return
  if (win.live && row.slug === activeSlug.value) await current.value?.select(win.windowId)
  else openWindow(row, win.windowId)
}

async function moveSelection(delta: number): Promise<void> {
  const keys = treeRowKeys.value
  if (!keys.length) return
  const at = keys.indexOf(cursorKey.value)
  // Clamped, not wrapped: running off the end of a session list and landing
  // back at the top reads as a jump, not as navigation.
  const next = keys[Math.min(keys.length - 1, Math.max(0, at + delta))]
  if (!next || next === cursorKey.value) return
  cursorRequest.value = next
  // Walking past a session is not an intent to type in it. The latch is what
  // holds that across the attach; the row still takes DOM focus so the ring and
  // the tab stop follow the walk.
  paneMayAutoFocus.value = false
  rowElement(next)?.focus()
  await activateRow(next)
}

function onTreeKeydown(event: KeyboardEvent): void {
  // Modified arrows belong to the keymap — Cmd+← is how focus got here.
  if (event.metaKey || event.ctrlKey || event.altKey) return
  if (renamingId.value || isEditableTarget(event.target)) return
  const delta = { ArrowDown: 1, j: 1, ArrowUp: -1, k: -1 }[event.key]
  if (delta === undefined) return
  event.preventDefault()
  void moveSelection(delta)
}

function onTreeFocusOut(event: FocusEvent): void {
  const next = event.relatedTarget
  if (next instanceof Node && sidebarEl.value?.contains(next)) return
  terminalTreeFocused.value = false
}

// Nothing to land on while the list is still loading, so take the panel itself
// and let the first arrow seed the cursor.
function focusTreeCursor(): void {
  void nextTick(() => (rowElement(cursorKey.value) ?? sidebarEl.value)?.focus())
}

// A chord that names a window is an intent to type in it, so the cursor follows
// and the pane is allowed to take focus.
function goToWindow(tab: TerminalWindowTab | undefined): void {
  if (!tab) return
  if (attachedRow.value) cursorRequest.value = `w:${attachedRow.value.id}:${tab.windowId}`
  paneMayAutoFocus.value = true
  void current.value?.select(tab.windowId)
}

// Wrapping, where the tree's own walk clamps: a session's windows are a ring in
// every terminal emulator, and there is nowhere else for "next" to go from the
// last one.
function relativeWindow(delta: number): TerminalWindowTab | undefined {
  const session = current.value
  const tabs = session?.tabs.value ?? []
  const at = tabs.findIndex((tab) => tab.windowId === session?.activeWindowId.value)
  if (at < 0) return undefined
  return tabs[(at + delta + tabs.length) % tabs.length]
}

// The Code view's palette library: the attached session's own operations,
// under the session's own name. Window and attach rows are registered at App
// level instead (useAppPaletteRows), since a fresh launch has to be able to
// list a session or its windows before this async component has ever
// mounted — a row registered here cannot be global. What is left here acts on
// state that lives in this component, and is gated on `active` inside the
// getter: the mode is mounted once and only hidden, so scope disposal never
// fires on a trip to the hub.
useCommands(() => {
  if (!props.active) return []
  const cmds: Command[] = []

  const attached = attachedRow.value
  if (attached) {
    const group = attached.name
    const hive = isHiveSession(attached)
    if ((hive && attached.state === 'active') || isScratch(attached)) {
      if (!rowRunning(attached)) {
        cmds.push({
          id: 'terminal:session:start',
          title: isScratch(attached) ? 'Start terminal' : 'Start session',
          group,
          order: -3,
          scope: 'actions',
          keywords: ['session', 'run', 'launch'],
          icon: IconPlay,
          run: () => void startSession(attached.slug),
        })
      }
      cmds.push({
        id: 'terminal:session:kill',
        title: 'Kill terminal…',
        group,
        order: -3,
        scope: 'actions',
        keywords: ['session', 'stop'],
        icon: IconSquare,
        run: () => requestKill(attached),
      })
    }
    if (hive) {
      if (attached.state === 'active' && rowRunning(attached) && !agentWindowBusy.value) {
        for (const agent of agentProfiles.value) {
          cmds.push({
            id: `terminal:session:agent:${agent}`,
            title: `New ${agent} agent`,
            group,
            order: -3,
            scope: 'actions',
            keywords: ['peer', 'window', 'shared', 'checkout'],
            icon: IconPlus,
            run: () => void newAgentWindowIn(attached, agent),
          })
        }
      }
      cmds.push(
        {
          id: 'terminal:session:detail',
          title: 'Session details…',
          group,
          order: -3,
          scope: 'actions',
          keywords: ['session', 'info'],
          icon: IconInfo,
          run: () => void openSessionDetail(attached),
        },
        {
          id: 'terminal:session:rename',
          title: 'Rename session…',
          group,
          order: -3,
          scope: 'actions',
          keywords: ['session'],
          icon: IconPencil,
          run: () => requestRename(attached),
        },
      )
      if (attached.state === 'active') {
        cmds.push({
          id: 'terminal:session:recycle',
          title: 'Recycle session…',
          group,
          order: -3,
          scope: 'actions',
          keywords: ['session', 'reset'],
          icon: IconRecycle,
          run: () => void requestRecycle(attached),
        })
      }
      cmds.push({
        id: 'terminal:session:delete',
        title: 'Delete session…',
        group,
        order: -3,
        scope: 'actions',
        keywords: ['session', 'remove'],
        icon: IconTrash,
        run: () => void requestDelete(attached),
      })
      for (const entry of sessionActionEntries.value) {
        if (entry.kind !== 'action') continue
        cmds.push({
          id: `terminal:session:${entry.id}`,
          title: entry.label,
          group,
          order: -3,
          scope: 'actions',
          keywords: ['session', 'action'],
          iconName: entry.iconName,
          iconColor: entry.iconColor,
          run: () => runSessionAction(attached, entry.id),
        })
      }
      // A window action needs a window, and the palette's window is the one on
      // screen — the row menu is where the other windows' copies live. The
      // window's name is in the hint because an action's label says what it
      // does, not what it does it to.
      const activeWindow = windowRowsFor(attached).find((win) => win.active)
      if (activeWindow) {
        for (const entry of windowActionEntries.value) {
          if (entry.kind !== 'action') continue
          cmds.push({
            id: `terminal:window:action:${entry.id}`,
            title: entry.label,
            group,
            order: -3,
            scope: 'actions',
            keywords: ['window', 'action', activeWindow.name],
            iconName: entry.iconName,
            iconColor: entry.iconColor,
            hint: activeWindow.name,
            run: () => runWindowAction(attached, activeWindow.windowId, entry.id),
          })
        }
      }
    }
    if (isChat(attached)) {
      cmds.push(
        {
          id: 'terminal:chat:open-in-agents',
          title: 'Open in Chats',
          group,
          order: -3,
          scope: 'goto',
          keywords: ['chat', 'chats', 'agents'],
          icon: IconMessagesSquare,
          run: () => openChatInAgents(attached),
        },
        {
          id: 'terminal:chat:unpin',
          title: 'Unpin from Code',
          group,
          order: -3,
          scope: 'actions',
          keywords: ['chat', 'pin'],
          icon: IconPinOff,
          run: () => unpinSlug(attached.slug),
        },
      )
    }
  }

  return cmds
})

// The App-level palette's window rows (useAppPaletteRows) read this
// projection rather than component state, so they can list a session's tabs
// before this async component has ever mounted, and so a jump still works
// after a trip back to the hub — the reason these rows moved to App level in
// the first place. Keyed off lastAttachedSlug and the pool rather than
// attachedRow/windowRowsFor, which follow activeSlug and only show a
// session's live tabs while it is the on-screen one: both go stale the
// moment the route leaves /terminal, which this must not.
watch(
  () => {
    const live = lastAttachedSlug.value ? pool.get(lastAttachedSlug.value) : undefined
    const row = live ? attachable.value.find((candidate) => candidate.slug === lastAttachedSlug.value) : undefined
    if (!live || !row) return null
    return {
      slug: row.slug,
      name: row.name,
      windows: live.tabs.value.map((tab) => ({
        windowId: tab.windowId,
        name: tab.name || tab.windowId,
        active: tab.windowId === live.activeWindowId.value,
      })),
    }
  },
  (next) => setAttachedTerminalWindows(next),
  { immediate: true },
)
onBeforeUnmount(() => setAttachedTerminalWindows(null))

// `!` in the palette opens a window on the attached session running the rest of
// the line — a shell in that checkout, in the strip beside the others, which
// outlives the command the way a window does. It needs a session to open in, so
// the picker offers nothing; the hint names the one it found.
useShellEscape(
  (line) => {
    if (!props.active) return []
    const attached = attachedRow.value
    if (!attached) return []
    return [
      {
        id: 'shell:run',
        title: `Run: ${line}`,
        keywords: ['shell', 'terminal', 'window', 'run'],
        icon: IconTerminal,
        hint: `new window in ${attached.name}`,
        run: () => void current.value?.newWindow(line),
      },
    ]
  },
  () => props.active,
)

onMounted(() =>
  setTerminalTreeHandles({
    focusTree: focusTreeCursor,
    // Selected, not just focused: `/` on a field that already has a query means
    // a new search far more often than an edit of the old one.
    focusFilter: (): void => {
      void nextTick(() => filterInput.value?.select())
    },
    focusPane: (): void => current.value?.focusActive(),
    // Counted along the strip rather than clamped to it: ⌘3 means the third
    // window, so a session with two ignores it instead of standing the last one
    // in for a window the user did not ask for.
    selectWindow: (position: number): void => goToWindow(current.value?.tabs.value[position - 1]),
    stepWindow: (delta: number): void => goToWindow(relativeWindow(delta)),
    newWindow: (): void => {
      const row = attachedRow.value
      if (!row) return
      paneMayAutoFocus.value = true
      // tmux names the window, so there is no key to point the cursor at yet.
      // Dropping the request lets it fall back to the attached session's active
      // window, which is the new one as soon as the event lands.
      cursorRequest.value = ''
      void newWindowIn(row)
    },
    closeWindow: (): void => {
      const session = current.value
      const windowId = session?.activeWindowId.value
      if (!session || !windowId) return
      const name = session.tabs.value.find((tab) => tab.windowId === windowId)?.name ?? ''
      void requestCloseWindow(activeSlug.value, windowId, name)
    },
    // Split, zoom, and directional focus keep typing focus when tmux reports the
    // active pane.
    splitPane: (direction): void => {
      paneMayAutoFocus.value = true
      void current.value?.splitPane(direction)
    },
    closePane: (): void => {
      const session = current.value
      const tab = session?.tabs.value.find((candidate) => candidate.windowId === session.activeWindowId.value)
      const pane = tab && activePaneOf(tab)
      if (!session || !tab || !pane) return
      void requestClosePane(activeSlug.value, pane.paneId, tab.name)
    },
    zoomPane: (): void => {
      paneMayAutoFocus.value = true
      void current.value?.zoomPane()
    },
    focusPaneDirection: (direction): void => {
      paneMayAutoFocus.value = true
      void current.value?.focusPane(direction)
    },
  }),
)
onBeforeUnmount(() => setTerminalTreeHandles(null))

// The selection markers are two elements that travel, not a border each row
// draws for itself: one tracks the attached session, one the active window, and
// moving them is what makes a selection read as the same mark relocating.
//
// Measured off the rows rather than computed. Row heights differ by kind, a
// group's own height animates, and a collapsed group contributes nothing — so
// there is no arithmetic over the list that stays true.
const treeContent = ref<HTMLElement | null>(null)
interface SelectionRail {
  y: number
  height: number
  shown: boolean
}
const sessionRail = ref<SelectionRail>({ y: 0, height: 0, shown: false })
const windowRail = ref<SelectionRail>({ y: 0, height: 0, shown: false })

// A rail with no row to sit on fades out where it stands rather than resetting,
// so it does not travel from a stale origin the next time one appears.
function measureRail(rail: SelectionRail, selector: string): SelectionRail {
  const content = treeContent.value
  const row = content?.querySelector<HTMLElement>(selector)
  if (!content || !row) return { ...rail, shown: false }
  return {
    y: row.getBoundingClientRect().top - content.getBoundingClientRect().top,
    height: row.offsetHeight,
    shown: true,
  }
}

function measureRails(): void {
  sessionRail.value = measureRail(sessionRail.value, '[data-testid="terminal-session-row"][data-attached="true"]')
  windowRail.value = measureRail(windowRail.value, '[data-testid="terminal-window-row"][data-active="true"]')
}

// Rows move under FLIP and inside a panel whose height is animating, so one
// measurement lands mid-flight and sticks. Re-measure per frame until the tree
// has stopped moving; a later trigger extends the window rather than stacking
// a second loop on it.
let railSettleUntil = 0
let railSettleFrame = 0
function settleRails(): void {
  measureRails() // land on the same frame as the click; the loop only corrects
  // The loop exists to correct a measurement taken while rows are still moving.
  // Nothing moves during the first fill — motion is suppressed for it — so
  // there is nothing to chase, and running it would force layout every frame
  // for as long as the tree took to fill in.
  if (!treeSettled.value) return
  railSettleUntil = performance.now() + 260
  if (railSettleFrame) return
  const step = (): void => {
    measureRails()
    railSettleFrame = performance.now() < railSettleUntil ? requestAnimationFrame(step) : 0
  }
  railSettleFrame = requestAnimationFrame(step)
}

// A resize of the tree's content is every layout change that can move a row —
// expand/collapse, a session arriving, a window subtree filling in — and it
// fires per frame while a height animates. Selection can also change without
// moving anything, which is what the watch below covers.
useResizeObserver(treeContent, () => settleRails())

watch(
  () => [activeSlug.value, current.value?.activeWindowId.value, props.sidebarCollapsed] as const,
  () => void nextTick(settleRails),
)

onBeforeUnmount(() => {
  if (railSettleFrame) cancelAnimationFrame(railSettleFrame)
  railSettleFrame = 0
})

// Expand/collapse animates the measured height — the hooks only pin the start
// and end values, and the .tree-expand-* classes carry the (fast) transition.
//
// The animated element holds nothing but the panel: its border and padding sit
// on the block inside it, because a box cannot shrink below its own padding, so
// a padded panel would animate down to 9px and then snap the rest of the way.
function expandEnter(el: Element): void {
  const panel = el as HTMLElement
  panel.style.height = '0'
  panel.getBoundingClientRect() // commit the collapsed height before the target lands
  panel.style.height = `${panel.scrollHeight}px`
}

// Clears the pinned height so an open panel resizes with its content — on the
// way in, and on an enter cut short by a collapse before it finished.
function expandSettle(el: Element): void {
  ;(el as HTMLElement).style.height = ''
}

function expandLeave(el: Element): void {
  const panel = el as HTMLElement
  panel.style.height = `${panel.scrollHeight}px`
  panel.getBoundingClientRect()
  panel.style.height = '0'
}

function openWindow(row: TerminalSessionRow, windowId: string): void {
  void router.push({ name: 'terminal', params: { slug: row.slug }, query: { window: windowId } })
}

const {
  size: sidebarWidth,
  startResize,
  step,
} = useResizablePanel({
  storageKey: 'hive.panel.terminal.sidebar',
  defaultSize: 250,
  min: 180,
  max: 400,
  edge: 'right',
})

// A launched session lands in the sidebar without a manual refresh: session
// creation runs as a job, and jobs:updated is the wake-up that fires when one
// finishes. Extra reloads are harmless — the list is small.
useWailsEvent('jobs:updated', () => {
  void reloadSessions()
})
// jobs:updated only covers this app's own session work; the CLI writes
// hive.db from another process.
useWailsEvent('sessions:updated', () => {
  void reloadSessions()
})

const tabs = computed<TerminalWindowTab[]>(() => visible.value?.tabs.value ?? [])
const activeWindowId = computed(() => visible.value?.activeWindowId.value ?? '')
const activeScrolledUp = computed(() =>
  tabs.value.some((tab) => tab.windowId === activeWindowId.value && activePaneOf(tab)?.scrolledUp),
)
const status = computed(() => visible.value?.status.value ?? 'connecting')
const endReason = computed(() => visible.value?.endReason.value ?? null)
// Not a failure: tmux is running nothing under this slug, and starting it runs
// the session's agent command — so it is offered, never done on selection.
const notStarted = computed(() => endReason.value === 'not-started')
const scratchAttached = computed(() => !!attachedRow.value && isScratch(attachedRow.value))
const chatAttached = computed(() => !!attachedRow.value && isChat(attachedRow.value))
const starting = ref('')
const startError = ref('')
const sessionError = computed(() => visible.value?.error.value ?? '')
// A tree control can act on a session that is not the attached one, so its
// failure has no session to report through; it falls back to the same strip.
const treeError = ref('')
const actionError = computed(() => visible.value?.actionError.value || treeError.value)
const sizeConstraint = computed(() => visible.value?.sizeConstraint.value ?? null)
const outputDropped = computed(() => visible.value?.outputDropped.value ?? false)

const search = computed(() => visible.value?.search.value ?? { open: false, query: '', matches: 0, index: 0 })
const searchInput = ref<HTMLInputElement | null>(null)

// The find bar opens from inside the pane (the terminal owns its keys), so the
// focus move is driven by the state rather than by the handler that set it.
watch(
  () => search.value.open,
  async (open) => {
    if (!open) return
    await nextTick()
    searchInput.value?.select()
  },
)

function searchLabel(): string {
  if (!search.value.query) return ''
  if (search.value.matches < 0) return 'many'
  if (!search.value.matches) return 'no results'
  return `${search.value.index}/${search.value.matches}`
}

// Every pooled session's panes stay mounted: a Terminal binds to one element
// for its lifetime, and an incoming session's first paint has to land while
// its panes are still hidden behind the held one.
const paneSessions = computed(() =>
  [...pool.entries()].map(([slug, entry]) => ({
    slug,
    entry,
    tabs: entry.tabs.value,
    activeWindowId: entry.activeWindowId.value,
  })),
)

// The toggle into this mode is always live, so the gate is a panel here
// rather than a disabled button in the title bar. The gate only shows on the
// first visit: with a cached client the last-known tree renders immediately,
// the resume and the listing revalidation start at once, and the probe below
// only re-checks availability.
async function probe(): Promise<void> {
  const hadClient = client.value !== null
  if (hadClient) {
    if (attachable.value.length) restoreLastSession()
    void reloadSessions().then(restoreLastSession)
  }
  // The session list is a SQLite read that knows nothing about tmux, so it goes
  // out with the availability probe rather than behind it. Awaiting the two in
  // series put three round trips in front of the first row.
  const sessions = hadClient ? null : reloadSessions()
  await probeAvailability()
  if (!hadClient && client.value) {
    await sessions
    restoreLastSession()
  }
}

// A remembered session that no longer exists — or that has since been recycled,
// which leaves no tmux session to attach to — is forgotten rather than attached
// blind; the picker shows, same as a first visit.
function restoreLastSession(): void {
  if (routeSlug.value || !restore.value.slug) return
  if (!attachable.value.some((row) => row.slug === restore.value.slug)) {
    restore.value = { slug: '', window: '' }
    return
  }
  void router.replace({
    name: 'terminal',
    params: { slug: restore.value.slug },
    query: restore.value.window ? { window: restore.value.window } : {},
  })
}

// The route is what attaches: rows and restores only navigate, and this
// watcher is the single path into openSession, so back/forward re-attach
// exactly like a click. Immediate because the cached client can already be
// live at mount, in which case a deep-linked slug fires no change at all.
watch(
  [client, routeSlug],
  ([ready, slug]) => {
    if (!ready) return
    if (slug) openSession(slug)
    else detachSession()
  },
  { immediate: true },
)

// `?window` is a route input in its own right, not only something read at
// attach time: a same-slug push that changes only the window (the App-level
// palette's window rows) moves neither of the pair above, so it needs its own
// watcher. The mirror watch just below then rewrites the URL to the same
// value once the resulting active-changed event lands — same value, so it
// does not loop back into this one.
watch(routeWindow, (wanted) => {
  if (current.value) selectIfWanted(current.value, wanted)
})

// Mirror the attached window into the URL and the resume snapshot. Guarded to
// the live route so a navigation away cannot claw the history entry back.
watch([activeSlug, () => current.value?.activeWindowId.value ?? ''], ([slug, windowId]) => {
  if (!slug || route.name !== 'terminal' || route.params.slug !== slug) return
  restore.value = { slug, window: windowId }
  if (windowId && routeWindow.value !== windowId) {
    void router.replace({ name: 'terminal', params: { slug }, query: { window: windowId } })
  }
})

// The attached slug can stop being attachable two ways, and a list reload is how
// we find out about either: the session was deleted or recycled (both run as
// jobs, and a recycled session leaves the attachable set), or it was renamed —
// hive re-slugs on rename, so the same session reappears under a new slug.
// Following the id is what tells the two apart, and it works whether the change
// came from this window or from the hive CLI.
const attachedId = ref('')
watch([attachable, activeSlug], ([rows, slug]) => {
  if (sessionsError.value) return
  // A pooled session the listing stopped carrying was deleted or recycled out
  // from under its attach. The selected slug is handled below instead, because
  // telling its deletion apart from a rename needs the id.
  for (const pooledSlug of [...pool.keys()]) {
    if (pooledSlug !== slug && !rows.some((row) => row.slug === pooledSlug)) dropSession(pooledSlug)
  }
  if (!slug) return
  const attached = rows.find((row) => row.slug === slug)
  if (attached) {
    attachedId.value = attached.id
    return
  }
  // Attached before the list ever loaded, so which session this is was never
  // learned; the attach reports its own failure rather than being guessed at.
  if (!attachedId.value) return
  const renamed = rows.find((row) => row.id === attachedId.value)
  if (!renamed) {
    closeSession()
    return
  }
  // The slug is the tmux target, so the attach follows the rename.
  restore.value = { slug: renamed.slug, window: '' }
  void router.replace({ name: 'terminal', params: { slug: renamed.slug } })
})

function selectSession(slug: string): void {
  if (slug === activeSlug.value) {
    // Same URL, so the route watcher stays silent — but after the session
    // ended the row is as valid a way back in as the overlay's Reconnect,
    // and reselecting a live one is an intent to type into it.
    if (current.value?.status.value === 'ended') openSession(slug)
    else current.value?.focusActive()
    return
  }
  void router.push({ name: 'terminal', params: { slug } })
}

// Starting is the user's move, never a side effect of selecting a row: it runs
// the session's agent command. A session already running is not respawned, so
// this doubles as "open it" from the row menu.
// Starting a pinned chat is the Agents area's resume rather than hive's spawn:
// there is no hive session behind an agentws-* slug for a spawn configuration to
// be read from, and relaunching a stopped chat is the agent's own resume flag to
// apply (ADR agent-workspace-sessions-are-tmux-sessions). Either way it stays an offered action and never
// something an attach does on its own (ADR terminal-start-is-an-offered-action).
async function startSession(slug: string): Promise<void> {
  if (!client.value || starting.value) return
  starting.value = slug
  startError.value = ''
  try {
    if (chatSlugs.value.has(slug)) await resumePinnedChat(slug)
    else await client.value.start(slug)
  } catch (e) {
    startError.value = e instanceof Error && e.message ? e.message : 'Could not start this session.'
    return
  } finally {
    starting.value = ''
  }
  // The resume created the tmux session behind a row the sweep last saw stopped,
  // and nothing the sweep watches moved — so it is asked again explicitly rather
  // than leaving the row's liveness resting on the attach that follows.
  if (chatSlugs.value.has(slug)) sweepListings()
  if (slug !== activeSlug.value) {
    void router.push({ name: 'terminal', params: { slug } })
    return
  }
  const pooled = pool.get(slug)
  if (!pooled) openSession(slug)
  else if (pooled.status.value === 'ended') void pooled.reconnect()
  else pooled.focusActive()
}

// The chat's record is the Agents area's, so the id comes from its listing rather
// than from the slug: deriving one from the other would put the core's tmux
// naming scheme in two places.
async function resumePinnedChat(slug: string): Promise<void> {
  const session = recents.value.find((candidate) => candidate.slug === slug)
  if (!session) throw new Error('This chat is no longer listed.')
  await resumeChat({ id: session.id })
  await reloadRecents()
}

// Killing ends the terminal and nothing else — the checkout, the record and the
// work stay — but it stops whatever is running inside, so it is confirmed like
// the session's own destructive operations. The scratch terminal *is* its tmux
// session, so there the confirmation names the tabs rather than a checkout.
function requestKill(row: TerminalSessionRow): void {
  confirmation.request({
    title: 'Kill this terminal?',
    description: isScratch(row)
      ? 'Every tab in the scratch terminal is closed and whatever is running in them stops. Starting it again opens an empty one.'
      : `The tmux session behind ${row.name} is killed, stopping the agent and anything else running in it. Its checkout and its work are untouched, and you can start it again from here.`,
    confirmLabel: 'Kill',
    onConfirm: () => killSession(row.slug),
  })
}

// Closing a tab kills its tmux window and everything running in it, so a window
// with a foreground process — an agent, an editor, a script — is confirmed
// first, and a shell at its prompt closes on the click. The state is read at the
// moment of the close rather than taken from anything the sidebar already has:
// what a pane is running changes without tmux announcing it, so a cached answer
// would be a stale one.
async function requestCloseWindow(slug: string, windowId: string, name: string): Promise<void> {
  const pooled = pool.get(slug)
  if (!pooled) return
  const close = (): Promise<void> => pooled.closeWindow(windowId)
  const foreground = await windowForeground(slug, windowId)
  if (!foreground.running) {
    await close()
    return
  }
  confirmation.request({
    title: 'Close this tab?',
    description: `${foreground.command || 'Something'} is still running ${name ? `in ${name}` : 'in this tab'}. Closing the tab stops it.`,
    confirmLabel: 'Close tab',
    onConfirm: close,
  })
}

// tmux closes the window when its last pane closes, so use the tab-close
// confirmation policy.
async function requestClosePane(slug: string, paneId: string, name: string): Promise<void> {
  const pooled = pool.get(slug)
  if (!pooled) return
  const close = (): Promise<void> => pooled.closePane(paneId)
  const foreground = await paneForeground(slug, paneId)
  if (!foreground.running) {
    await close()
    return
  }
  confirmation.request({
    title: 'Close this pane?',
    description: `${foreground.command || 'Something'} is still running ${name ? `in ${name}` : 'in this pane'}. Closing the pane stops it.`,
    confirmLabel: 'Close pane',
    onConfirm: close,
  })
}

// A read that failed is not evidence the tab is idle, and killing a process to
// find out is the one outcome the confirmation exists to prevent.
async function windowForeground(slug: string, windowId: string): Promise<WindowForeground> {
  if (!client.value) return { running: true, command: '' }
  try {
    return await client.value.windowForeground(slug, windowId)
  } catch {
    return { running: true, command: '' }
  }
}

async function paneForeground(slug: string, paneId: string): Promise<WindowForeground> {
  if (!client.value) return { running: true, command: '' }
  try {
    return await client.value.paneForeground(slug, paneId)
  } catch {
    return { running: true, command: '' }
  }
}

async function killSession(slug: string): Promise<void> {
  if (!client.value) return
  await client.value.kill(slug)
  // Re-attaching the killed session is what lands it on the start panel; a
  // pooled one that is not on screen is simply let go.
  if (slug === activeSlug.value) void pool.get(slug)?.reconnect()
  else dropSession(slug)
  if (showAllWindows.value) void refreshListings(client.value, attachable.value)
}

function openSession(slug: string): void {
  if (!client.value) return
  renamingId.value = ''
  startError.value = ''
  activeSlug.value = slug
  const pooled = pool.get(slug)
  if (pooled && pooled.status.value !== 'ended') {
    touchPool(slug)
    lastAttachedSlug.value = slug
    selectIfWanted(pooled, routeWindow.value)
    return
  }
  if (pooled) dropSession(slug)
  const opened = useTerminalWindows(slug, client.value)
  pool.set(slug, opened)
  touchPool(slug)
  // Set after the possible dropSession above, which would otherwise clear it
  // straight back to '' for this same slug.
  lastAttachedSlug.value = slug
  // Captured before attach: the mirror watcher rewrites ?window to tmux's
  // active the moment windows land, and the wanted one must survive that.
  const wanted = routeWindow.value
  void opened.start().then(() => {
    if (pool.get(slug) !== opened) return
    selectIfWanted(opened, wanted)
  })
}

// Leaving for the picker keeps the pool warm; only closeSession and the
// listing pruning actually let an attach go.
function detachSession(): void {
  activeSlug.value = ''
  renamingId.value = ''
  startError.value = ''
  displayed.value = null
}

function closeSession(): void {
  dropSession(activeSlug.value)
  detachSession()
  restore.value = { slug: '', window: '' }
  void reloadSessions()
  // Replace, not push: the closed session's entry points at a session that is
  // gone, so Back must not walk into it.
  if (routeSlug.value) void router.replace({ name: 'terminal' })
}

// Adding a window is a property of the session, not of what is on screen, so
// the row offers it whether or not that session is the attached one. A row
// with nothing running has no window to add to, so + starts it instead
// (mirrors enterSessionRow). Every branch selects the row first: a start can
// fail, and only the row's own pane, with its Start button, shows the error.
// A pooled session goes through its own client, which is what makes the new
// window the active one; an unattached but running session takes the plain
// slug-keyed call, and the attach that selecting the row starts lists its
// windows fresh.
async function newWindowIn(row: TerminalSessionRow): Promise<void> {
  const pooled = pool.get(row.slug)
  if (pooled && pooled.status.value !== 'ended') {
    selectSession(row.slug)
    await pooled.newWindow()
    return
  }
  if (!rowRunning(row)) {
    selectSession(row.slug)
    await startSession(row.slug)
    return
  }
  const transport = client.value
  if (!transport) return
  selectSession(row.slug)
  try {
    treeError.value = ''
    await transport.newWindow(row.slug)
  } catch (e) {
    treeError.value = appErrorMessage(e) || 'Could not create a window.'
  }
}

function toggleNewWindowMenu(row: TerminalSessionRow, event: MouseEvent): void {
  if (openNewWindowMenu.value === row.id) {
    openNewWindowMenu.value = ''
    return
  }
  openRowMenu.value = ''
  openWindowMenu.value = ''
  newWindowToggle.value = event.currentTarget as HTMLElement
  newWindowMenuFlip.value = menuFlipsUp(newWindowToggle.value)
  openNewWindowMenu.value = row.id
}

async function newAgentWindowIn(row: TerminalSessionRow, agent: string): Promise<void> {
  const transport = client.value
  if (!transport || agentWindowBusy.value) return
  agentWindowBusy.value = true
  treeError.value = ''
  try {
    const pooled = pool.get(row.slug)
    if (pooled && pooled.status.value !== 'ended') {
      selectSession(row.slug)
      await pooled.newAgentWindow(agent)
      return
    }
    const { windowId } = await transport.newAgentWindow(row.slug, agent)
    await router.push({ name: 'terminal', params: { slug: row.slug }, query: { window: windowId } })
    sweepListings()
  } catch (e) {
    treeError.value = errorText(e, 'Could not start the agent.')
  } finally {
    agentWindowBusy.value = false
  }
}

// The rename field sits inside the row, so a double-click meant for its text
// arrives here as well; restarting the rename would discard what was typed.
function startRename(win: TreeWindowRow): void {
  if (!win.live || renamingId.value === win.windowId) return
  renamingId.value = win.windowId
  renameDraft.value = win.name
}

function commitRename(): void {
  const windowId = renamingId.value
  if (!windowId) return
  renamingId.value = ''
  const name = renameDraft.value.trim()
  if (name) void visible.value?.rename(windowId, name)
}

// ── window reordering ───────────────────────────────────────────────────────
// A window carries the slug it came from and can only land back in that
// session. Native HTML5 DnD, the same shape the hub sidebar uses; the dragged
// window is tracked here because dataTransfer cannot be read during dragover,
// and the hovered edge drives the marker.
const WINDOW_DRAG_MIME = 'application/x-hive-terminal-window'
const draggingWindow = ref<{ slug: string; windowId: string } | null>(null)
const dropTarget = ref<{ slug: string; windowId: string; after: boolean } | null>(null)

function onWindowDragStart(event: DragEvent, slug: string, windowId: string): void {
  draggingWindow.value = { slug, windowId }
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData(WINDOW_DRAG_MIME, windowId)
  }
}

function onWindowDragOver(event: DragEvent, slug: string, windowId: string): void {
  const dragged = draggingWindow.value
  if (!dragged) return
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  // Order is a property of one session, and both edges of the dragged window
  // name the gap it already fills — neither is a move, so neither marks one.
  // Without the second half the drop reads as an insertion past every other
  // window and sends it to the end.
  if (dragged.slug !== slug || dragged.windowId === windowId) {
    dropTarget.value = null
    return
  }
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  dropTarget.value = { slug, windowId, after: event.clientY > rect.top + rect.height / 2 }
}

function onWindowDrop(): void {
  const target = dropTarget.value
  const dragged = draggingWindow.value
  onWindowDragEnd()
  if (!target || !dragged) return
  const session = pool.get(dragged.slug)
  if (!session) return
  void session.moveWindow(dragged.windowId, dropPosition(windowOrder(dragged.slug), dragged.windowId, target))
}

function onWindowDragEnd(): void {
  draggingWindow.value = null
  dropTarget.value = null
}

// The order a drag reorders against. A window is only movable while its session
// holds a control client, which is exactly when the tree renders its live tabs
// rather than a cached listing — so a pooled session is the whole precondition.
function windowOrder(slug: string): string[] {
  return pool.get(slug)?.tabs.value.map((tab) => tab.windowId) ?? []
}

// The drop edge names a gap between windows; the API takes the index the moved
// window ends up at, which is that gap once the window is out of the list.
function dropPosition(order: string[], windowId: string, target: { windowId: string; after: boolean }): number {
  const rest = order.filter((id) => id !== windowId)
  const anchor = rest.indexOf(target.windowId)
  if (anchor < 0) return rest.length
  return target.after ? anchor + 1 : anchor
}

// Dimmed where the drag started, marked where the pointer is.
function draggingWindowRow(slug: string, windowId: string): boolean {
  const dragged = draggingWindow.value
  return !!dragged && dragged.slug === slug && dragged.windowId === windowId
}

function showDropBefore(slug: string, windowId: string): boolean {
  const target = dropTarget.value
  return !!target && !target.after && target.slug === slug && target.windowId === windowId
}

function showDropAfter(slug: string, windowId: string): boolean {
  const target = dropTarget.value
  return !!target && target.after && target.slug === slug && target.windowId === windowId
}

// Entering the mode is an activation, not a mount: the pool, its tmux control
// clients and their screens all survive a trip to the hub, so re-entry is a
// display flip plus a revalidation — the hub is where a session gets created,
// renamed or deleted, and the tree has to catch up. The attach itself is what
// used to be paid here, and no longer is.
watch(
  () => props.active,
  (active) => {
    if (!active) {
      stopStatusPolling()
      return
    }
    void probe()
    startStatusPolling()
  },
  { immediate: true },
)

onMounted(() => {
  prefetchNewSession()
  void loadTerminalActions()
})
onBeforeUnmount(() => {
  clearTimeout(holdTimer)
  stopStatusPolling()
  for (const slug of [...pool.keys()]) dropSession(slug)
})
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col bg-app" data-testid="terminal-mode">
    <div v-if="checking" class="flex flex-1 items-center justify-center font-mono text-xs text-text-4">
      Checking tmux…
    </div>

    <div
      v-else-if="!available"
      class="flex flex-1 flex-col items-center justify-center gap-3 px-10 text-center"
      data-testid="terminal-unavailable"
    >
      <IconTerminal class="size-6 text-text-4" />
      <div class="text-[13.5px] font-semibold">Terminal unavailable</div>
      <p class="max-w-[420px] text-xs leading-relaxed text-text-3" data-testid="terminal-unavailable-reason">
        {{ reason || 'The terminal is not available in this build.' }}
      </p>
      <button
        type="button"
        class="mt-1 cursor-pointer rounded border border-strong px-3 py-1.5 text-xs text-text-2 hover:text-text"
        data-testid="terminal-retry"
        @click="probe"
      >
        Try again
      </button>
    </div>

    <div v-else class="flex min-h-0 min-w-0 flex-1">
      <!-- The sidebar is persistent, like the TUI's session tree: repos as
           group headers, sessions under them, and tmux windows nested beneath.
           A pooled session's windows are its live tab set; the rest render
           only with "Always show windows" on, from a one-shot listing. -->
      <aside
        v-if="!sidebarCollapsed"
        ref="sidebarEl"
        class="relative flex shrink-0 flex-col border-r border-border bg-sidebar"
        :style="{ width: sidebarWidth + 'px' }"
        data-testid="terminal-session-sidebar"
        tabindex="-1"
        @keydown="onTreeKeydown"
        @focusin="terminalTreeFocused = true"
        @focusout="onTreeFocusOut"
      >
        <div class="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3">
          <!-- Flush in the bar rather than a boxed field: the sidebar resizes
               down to 180px, and a bordered input beside three controls leaves
               the bar looking like nothing but chrome. -->
          <SearchField
            ref="filterInput"
            v-model="sessionFilter"
            variant="bar"
            aria-label="Filter sessions"
            testid="terminal-sessions-filter"
            @escape="focusTreeCursor"
            @keydown.down.prevent="focusTreeCursor"
            @keydown.enter.prevent="focusTreeCursor"
          />
          <!-- The refresh control doubles as the staleness indicator: the
               cached tree renders instantly, and the spin is what says a
               revalidation is still in flight. -->
          <button
            type="button"
            class="flex size-6 cursor-pointer items-center justify-center rounded-[7px] text-text-3 hover:bg-chip hover:text-text disabled:cursor-default"
            data-testid="terminal-sessions-refresh"
            aria-label="Reload sessions"
            :aria-busy="sessionsLoading"
            :disabled="sessionsLoading"
            @click="reloadSessions"
          >
            <IconRotateCw class="size-3.5" :class="{ 'animate-spin': sessionsLoading }" />
          </button>
          <button
            type="button"
            class="flex size-6 cursor-pointer items-center justify-center rounded-[7px] text-text-3 hover:bg-chip hover:text-text"
            data-testid="terminal-new-session"
            aria-label="New session"
            title="New session"
            @click="openNewSession(sessionRepository(activeSlug))"
          >
            <IconPlus class="size-3.5" />
          </button>
          <!-- List-wide operations; a session's own live on its row. -->
          <div class="relative flex">
            <button
              ref="sidebarMenuToggle"
              type="button"
              class="flex size-6 cursor-pointer items-center justify-center rounded-[7px] text-text-3 hover:bg-chip hover:text-text"
              data-testid="terminal-sessions-menu-toggle"
              aria-label="Session list actions"
              aria-haspopup="menu"
              :aria-expanded="sidebarMenuOpen"
              @click="sidebarMenuOpen = !sidebarMenuOpen"
            >
              <IconEllipsisVertical class="size-3.5" />
            </button>
            <AppMenu
              v-if="sidebarMenuOpen"
              :entries="sidebarMenuEntries"
              :ignore="[sidebarMenuToggle]"
              testid="terminal-sessions-menu"
              @close="sidebarMenuOpen = false"
              @select="onSidebarMenuSelect"
            />
          </div>
        </div>
        <div
          v-if="runningNote"
          class="flex h-7 shrink-0 items-center gap-2 border-b border-border bg-chip px-3"
          data-testid="terminal-sessions-running-note"
        >
          <IconListFilter class="size-3 shrink-0 text-accent" />
          <span class="min-w-0 flex-1 truncate text-[11.5px] text-text-2">{{ runningNote }}</span>
          <button
            type="button"
            class="shrink-0 cursor-pointer text-[11.5px] text-text-3 hover:text-text"
            data-testid="terminal-sessions-running-clear"
            @click="runningOnly = false"
          >
            Show all
          </button>
        </div>
        <div class="hive-scroll min-h-0 flex-1 overflow-y-auto pb-4">
          <InlineError
            v-if="sessionsError"
            testid="terminal-sessions-error"
            variant="line"
            class="px-3 py-2"
            :message="sessionsError"
          />
          <p
            v-else-if="!treeReady"
            class="px-3 py-2 font-mono text-xs text-text-4"
            data-testid="terminal-sessions-loading"
          >
            Loading…
          </p>
          <!-- One repository reads as one block: the header keeps the sidebar's
               own surface and its sessions sit in a recessed panel under it, so
               a long run of sessions cannot bleed into the next repo's. The
               wrapper is the rails' positioning context and the box whose
               resize tells them a row has moved. -->
          <div ref="treeContent" class="relative" :class="{ 'tree-settling': !treeSettled }">
            <div v-for="group in filteredGroups" :key="group.key" class="border-t border-border first:border-t-0">
              <!-- Chats take a repository's header rather than the scratch
                   section's: it heads several rows, so its name cannot be one of
                   them the way the scratch terminal's is. -->
              <div
                v-if="group.kind !== 'scratch'"
                class="repo-group"
                role="button"
                tabindex="0"
                :data-testid="group.kind === 'chats' ? 'terminal-chats-group' : 'terminal-repo-group'"
                :data-repo="group.key"
                :aria-expanded="groupExpanded(group)"
                @click="toggleGroup(group)"
                @keydown.enter.self.prevent="toggleGroup(group)"
                @keydown.space.self.prevent="toggleGroup(group)"
              >
                <span class="min-w-0 truncate text-[13.5px] text-text">{{ group.name }}</span>
                <!-- The count and the add button share a cell: starting a
                     session in the repository you are pointing at is worth more
                     than the count is while you are pointing at it. -->
                <div class="group-trailing" @click.stop>
                  <span
                    class="group-count font-mono text-[11.5px]"
                    :class="groupAttached(group) ? 'text-accent' : 'text-text-4'"
                    >{{ group.sessions.length }}</span
                  >
                  <button
                    v-if="group.kind === 'repo' && group.key"
                    type="button"
                    class="row-action group-add"
                    :title="`New session in ${group.name}`"
                    :aria-label="`New session in ${group.name}`"
                    data-testid="terminal-repo-new-session"
                    @click="openNewSession(group.key)"
                  >
                    <IconPlus class="size-3" />
                  </button>
                </div>
                <component
                  :is="groupExpanded(group) ? IconChevronDown : IconChevronRight"
                  class="size-3 shrink-0 text-text-4"
                />
              </div>
              <!-- The scratch section is one repository's worth of chrome for a
                   session that is not one: the same header, and its tabs where a
                   repository lists its sessions. Its own controls live in the
                   header because there is no row under it to put them on, which
                   is also why this is a div — the header a repository gets is a
                   button, and a button cannot hold one. -->
              <template v-else>
                <div
                  v-for="row in group.sessions"
                  :key="row.id"
                  class="group-row"
                  :class="{ 'menu-open': openRowMenu === row.id }"
                  role="button"
                  tabindex="-1"
                  data-testid="terminal-scratch-heading"
                  :data-slug="row.slug"
                  :aria-expanded="groupExpanded(group)"
                  :title="row.slug"
                  @click="toggleGroup(group)"
                  @keydown.enter.self.prevent="toggleGroup(group)"
                  @keydown.space.self.prevent="toggleGroup(group)"
                  @contextmenu.prevent="toggleRowMenu(row, $event)"
                >
                  <span class="min-w-0 truncate text-[13.5px] text-text">{{ group.name }}</span>
                  <component
                    :is="groupExpanded(group) ? IconChevronDown : IconChevronRight"
                    class="ml-auto size-3 shrink-0 text-text-4"
                  />
                  <div class="row-trailing" data-testid="terminal-session-trailing" @click.stop>
                    <button
                      type="button"
                      class="row-action row-lead"
                      title="New tab"
                      aria-label="New tab"
                      data-testid="terminal-new-window"
                      @click="newWindowIn(row)"
                    >
                      <IconPlus class="size-3" />
                    </button>
                    <span
                      v-if="rowRunning(row)"
                      class="row-status text-severity-success"
                      title="Terminal running"
                      data-testid="terminal-session-liveness"
                    >
                      <span class="size-2.5 rounded-full bg-current" aria-hidden="true" />
                      <span class="sr-only">Terminal running</span>
                    </span>
                    <button
                      :ref="(el) => setRowMenuToggle(row.id, el)"
                      type="button"
                      class="row-action row-swap"
                      title="Terminal actions"
                      aria-label="Terminal actions"
                      aria-haspopup="menu"
                      :aria-expanded="openRowMenu === row.id"
                      data-testid="terminal-session-menu-toggle"
                      @click="toggleRowMenu(row)"
                    >
                      <IconEllipsisVertical class="size-3" />
                    </button>
                    <SessionRowMenu
                      v-if="openRowMenu === row.id"
                      :session="row"
                      scratch
                      :flip="rowMenuFlip"
                      :ignore="[rowMenuToggles.get(row.id) ?? null]"
                      @close="openRowMenu = ''"
                      @start="startSession(row.slug)"
                      @kill="requestKill(row)"
                    />
                  </div>
                </div>
              </template>
              <Transition
                name="tree-expand"
                @enter="expandEnter"
                @after-enter="expandSettle"
                @enter-cancelled="expandSettle"
                @leave="expandLeave"
              >
                <div v-if="groupExpanded(group)">
                  <TransitionGroup
                    name="tree"
                    tag="div"
                    class="relative flex flex-col border-t border-border bg-app py-1"
                  >
                    <div v-for="row in group.sessions" :key="row.id">
                      <!-- Not a <button>: the row's menu toggle is a real button, and
                           nesting one inside another is invalid. The scratch
                           section has no row here at all — its heading above is
                           the session, and what this panel lists are its tabs. -->
                      <div
                        v-if="group.kind !== 'scratch'"
                        class="session-row"
                        :class="{
                          'session-row-attached': row.slug === activeSlug,
                          'menu-open': openRowMenu === row.id || openNewWindowMenu === row.id,
                        }"
                        role="button"
                        :tabindex="tabStopKey === `s:${row.id}` ? 0 : -1"
                        :data-testid="group.kind === 'chats' ? 'terminal-chat-row' : 'terminal-session-row'"
                        :data-slug="row.slug"
                        :data-tree-key="`s:${row.id}`"
                        :data-attached="row.slug === activeSlug"
                        :title="row.slug"
                        @click="selectSessionRow(row)"
                        @keydown.enter.self.prevent="enterSessionRow(row)"
                        @keydown.space.self.prevent="enterSessionRow(row)"
                        @contextmenu.prevent="toggleRowMenu(row, $event)"
                      >
                        <span class="min-w-0 flex-1 truncate text-[13.5px]" :class="{ 'text-text-3': rowIdle(row) }">{{
                          row.name
                        }}</span>
                        <!-- AppMenu must anchor to the positioned row so its panel
                             spans the row. Grid overlap avoids making this slot a
                             positioning ancestor while keeping its width fixed.
                             A chat has no tabs to add, so its slot carries the
                             liveness dot and its own two-entry menu; everything a
                             chat's own lifecycle needs lives in the Agents area. -->
                        <div class="row-trailing" data-testid="terminal-session-trailing" @click.stop>
                          <button
                            v-if="group.kind !== 'chats'"
                            type="button"
                            class="row-action row-lead"
                            title="New window"
                            aria-label="New window"
                            aria-haspopup="menu"
                            :aria-expanded="openNewWindowMenu === row.id"
                            :disabled="agentWindowBusy"
                            data-testid="terminal-new-window"
                            @click="toggleNewWindowMenu(row, $event)"
                          >
                            <IconPlus class="size-3" />
                          </button>
                          <NewWindowMenu
                            v-if="openNewWindowMenu === row.id"
                            :running="rowRunning(row)"
                            :flip="newWindowMenuFlip"
                            :ignore="[newWindowToggle]"
                            :agents="agentProfiles"
                            :default-agent="defaultAgentProfile"
                            :loading="agentProfilesLoading"
                            :failed="agentProfilesFailed"
                            @close="openNewWindowMenu = ''"
                            @terminal="newWindowIn(row)"
                            @agent="newAgentWindowIn(row, $event)"
                            @retry="loadAgentProfiles"
                          />
                          <span
                            v-if="rowRunning(row)"
                            class="row-status text-severity-success"
                            :title="group.kind === 'chats' ? 'Agent running' : 'Terminal running'"
                            data-testid="terminal-session-liveness"
                          >
                            <span class="size-2.5 rounded-full bg-current" aria-hidden="true" />
                            <span class="sr-only">{{
                              group.kind === 'chats' ? 'Agent running' : 'Terminal running'
                            }}</span>
                          </span>
                          <button
                            :ref="(el) => setRowMenuToggle(row.id, el)"
                            type="button"
                            class="row-action row-swap"
                            :title="group.kind === 'chats' ? 'Chat actions' : 'Session actions'"
                            :aria-label="group.kind === 'chats' ? 'Chat actions' : 'Session actions'"
                            aria-haspopup="menu"
                            :aria-expanded="openRowMenu === row.id"
                            :data-testid="
                              group.kind === 'chats' ? 'terminal-chat-menu-toggle' : 'terminal-session-menu-toggle'
                            "
                            @click="toggleRowMenu(row)"
                          >
                            <IconEllipsisVertical class="size-3" />
                          </button>
                          <AppMenu
                            v-if="openRowMenu === row.id && group.kind === 'chats'"
                            :entries="chatMenuEntries"
                            :flip="rowMenuFlip"
                            width="min(230px, 100%)"
                            :ignore="[rowMenuToggles.get(row.id) ?? null]"
                            testid="terminal-chat-menu"
                            @select="onChatMenuSelect(row, $event)"
                            @close="openRowMenu = ''"
                          />
                          <SessionRowMenu
                            v-else-if="openRowMenu === row.id"
                            :session="row"
                            :extra="sessionActionEntries"
                            :flip="rowMenuFlip"
                            :ignore="[rowMenuToggles.get(row.id) ?? null]"
                            @extra="runSessionAction(row, $event)"
                            @close="openRowMenu = ''"
                            @start="startSession(row.slug)"
                            @kill="requestKill(row)"
                            @detail="openSessionDetail(row)"
                            @rename="requestRename(row)"
                            @recycle="requestRecycle(row)"
                            @delete="requestDelete(row)"
                          />
                        </div>
                      </div>
                      <Transition
                        name="tree-expand"
                        @enter="expandEnter"
                        @after-enter="expandSettle"
                        @enter-cancelled="expandSettle"
                        @leave="expandLeave"
                      >
                        <div v-if="windowRowsFor(row).length && groupExpanded(group)">
                          <TransitionGroup name="tree" tag="div" class="relative flex flex-col pb-1">
                            <!-- The slot carries the drag and its insertion
                                 marker: the row's own ::before and ::after draw
                                 the tree connector. Draggable only while the
                                 session is attached, which is the whole
                                 precondition for moving one of its windows. -->
                            <div
                              v-for="(win, index) in windowRowsFor(row)"
                              :key="win.windowId"
                              class="window-slot"
                              :class="{
                                'opacity-40': draggingWindowRow(row.slug, win.windowId),
                                'drop-before': showDropBefore(row.slug, win.windowId),
                                'drop-after': showDropAfter(row.slug, win.windowId),
                              }"
                              :draggable="win.live && renamingId !== win.windowId"
                              data-testid="terminal-window-slot"
                              @dragstart="onWindowDragStart($event, row.slug, win.windowId)"
                              @dragover.prevent="onWindowDragOver($event, row.slug, win.windowId)"
                              @drop.prevent="onWindowDrop"
                              @dragend="onWindowDragEnd"
                            >
                              <!-- Not a <button>, for the same reason the session
                                   row above is not: its own menu toggle is one. -->
                              <div
                                class="window-row"
                                :class="{
                                  'window-row-flush': group.kind === 'scratch',
                                  'window-row-last': index === windowRowsFor(row).length - 1,
                                  'window-row-active': win.active,
                                  'has-swap': win.live,
                                  'menu-open': openWindowMenu === windowMenuKey(row, win.windowId),
                                }"
                                role="button"
                                :tabindex="tabStopKey === `w:${row.id}:${win.windowId}` ? 0 : -1"
                                :data-testid="win.live ? 'terminal-window-row' : 'terminal-listed-window-row'"
                                :data-window-id="win.windowId"
                                :data-tree-key="`w:${row.id}:${win.windowId}`"
                                :data-active="win.live ? win.active : undefined"
                                @click="openTreeWindow(row, win)"
                                @keydown.enter.self.prevent="openTreeWindow(row, win)"
                                @keydown.space.self.prevent="openTreeWindow(row, win)"
                                @dblclick="startRename(win)"
                                @contextmenu.prevent="toggleWindowMenu(row, win.windowId, $event)"
                              >
                                <input
                                  v-if="renamingId === win.windowId"
                                  v-model="renameDraft"
                                  class="min-w-0 flex-1 bg-transparent font-mono text-[12.5px] text-text outline-none"
                                  data-testid="terminal-rename-input"
                                  autocapitalize="off"
                                  autocorrect="off"
                                  spellcheck="false"
                                  autofocus
                                  @click.stop
                                  @dblclick.stop
                                  @keydown.enter="commitRename"
                                  @keydown.esc="renamingId = ''"
                                  @blur="commitRename"
                                />
                                <span v-else class="min-w-0 flex-1 truncate font-mono text-[12.5px]">{{
                                  win.name
                                }}</span>
                                <div class="window-trailing" data-testid="terminal-window-trailing" @click.stop>
                                  <button
                                    v-if="win.live"
                                    type="button"
                                    class="row-action row-swap"
                                    :title="`Close ${win.name}`"
                                    :aria-label="`Close ${win.name}`"
                                    data-testid="terminal-close-window"
                                    @click="requestCloseWindow(row.slug, win.windowId, win.name)"
                                  >
                                    <IconX class="size-3" />
                                  </button>
                                  <span
                                    v-if="win.indicator"
                                    class="window-status"
                                    :class="win.indicator.color"
                                    :title="win.indicator.label"
                                    data-testid="terminal-window-status"
                                    :data-status="
                                      sessionStatuses[row.id]?.windows?.find(
                                        (status) => status.windowId === win.windowId,
                                      )?.status
                                    "
                                  >
                                    <component
                                      :is="win.indicator.icon"
                                      class="size-3"
                                      :class="{ 'animate-spin': win.indicator.animated }"
                                      aria-hidden="true"
                                    />
                                    <span class="sr-only">{{ win.indicator.label }}</span>
                                  </span>
                                  <!-- A window row has no operations of its own,
                                       so the toggle exists only once a configured
                                       action targets one. -->
                                  <button
                                    v-if="rowHasWindowActions(row)"
                                    :ref="(el) => setWindowMenuToggle(windowMenuKey(row, win.windowId), el)"
                                    type="button"
                                    class="row-action row-lead"
                                    title="Window actions"
                                    aria-label="Window actions"
                                    aria-haspopup="menu"
                                    :aria-expanded="openWindowMenu === windowMenuKey(row, win.windowId)"
                                    data-testid="terminal-window-menu-toggle"
                                    @click="toggleWindowMenu(row, win.windowId)"
                                  >
                                    <IconEllipsisVertical class="size-3" />
                                  </button>
                                  <AppMenu
                                    v-if="openWindowMenu === windowMenuKey(row, win.windowId)"
                                    :entries="windowActionEntries"
                                    :flip="windowMenuFlip"
                                    width="min(230px, 100%)"
                                    :ignore="[windowMenuToggles.get(windowMenuKey(row, win.windowId)) ?? null]"
                                    testid="terminal-window-menu"
                                    @select="runWindowAction(row, win.windowId, $event)"
                                    @close="openWindowMenu = ''"
                                  />
                                </div>
                              </div>
                            </div>
                          </TransitionGroup>
                        </div>
                        <!-- The scratch section with nothing in it. A bare heading
                             would leave the keyboard nothing to land on and the
                             mouse nothing but the + to guess at. -->
                        <div v-else-if="group.kind === 'scratch' && groupExpanded(group)" class="pb-1">
                          <div
                            class="window-row window-row-flush window-row-last text-text-4"
                            role="button"
                            :tabindex="tabStopKey === `s:${row.id}` ? 0 : -1"
                            data-testid="terminal-start-scratch"
                            :data-tree-key="`s:${row.id}`"
                            @click="enterSessionRow(row)"
                            @keydown.enter.self.prevent="enterSessionRow(row)"
                            @keydown.space.self.prevent="enterSessionRow(row)"
                          >
                            <span class="min-w-0 flex-1 truncate font-mono text-[12.5px]">Start a terminal</span>
                          </div>
                        </div>
                      </Transition>
                    </div>
                  </TransitionGroup>
                </div>
              </Transition>
            </div>
            <!-- Last, so they paint over the rows: every row and panel is
                 positioned too, and among positioned boxes DOM order decides. -->
            <div
              v-for="rail in [
                { key: 'session', state: sessionRail, testid: 'terminal-session-rail' },
                { key: 'window', state: windowRail, testid: 'terminal-window-rail' },
              ]"
              :key="rail.key"
              class="tree-rail"
              :class="{ 'tree-rail-shown': rail.state.shown }"
              :style="{ transform: `translateY(${rail.state.y}px)`, height: `${rail.state.height}px` }"
              :data-testid="rail.testid"
              :data-shown="rail.state.shown"
              aria-hidden="true"
            />
          </div>
          <!-- Under the tree rather than instead of it: the pinned section is
               drawn whether or not hive has a session to list. -->
          <EmptyState
            v-if="treeNote === 'empty'"
            variant="inline"
            class="px-3 py-2"
            message="No active sessions. Start one from the hub and it will appear here."
            data-testid="terminal-sessions-empty"
          />
          <EmptyState
            v-else-if="treeNote === 'no-matches'"
            variant="inline"
            class="px-3 py-2"
            :message="noMatchesNote"
            data-testid="terminal-sessions-no-matches"
          />
        </div>
        <!-- The tree's keys are not otherwise announced anywhere, so the panel
             carries its own legend. The focus chord is read off the live keymap
             because it is rebindable; the arrows are the tree's own handler and
             cannot move. -->
        <div v-if="attachable.length" class="tree-hints" data-testid="terminal-tree-hints">
          <span><Kbd class="text-text-3">↑↓</Kbd> switch</span>
          <span><Kbd class="text-text-3">↵</Kbd> enter</span>
          <!-- Whichever half of the focus pair leaves where focus is. The pane
               has nowhere to advertise its own way out, so the tree carries it. -->
          <span v-if="focusHint"
            ><Kbd class="text-text-3">{{ focusHint.keys }}</Kbd> {{ focusHint.label }}</span
          >
        </div>
        <PanelResizeHandle edge="right" name="terminal-sidebar" :start="startResize" :step="step" />
      </aside>

      <div class="flex min-h-0 min-w-0 flex-1 flex-col">
        <div
          v-if="!visible"
          class="flex flex-1 flex-col items-center justify-center gap-2 px-10 text-center"
          data-testid="terminal-no-session"
        >
          <IconTerminal class="size-6 text-text-4" />
          <p class="text-xs text-text-3">Select a session to attach.</p>
        </div>

        <!-- A session with no tmux session behind it has nothing to attach to
             yet, so the chrome stays out of the way and the panel below does
             the talking. The sidebar tree is the only window list — there is
             no tab strip to keep in step with it (ADR the-sidebar-tree-is-the-only-window-list). -->
        <!-- Outside the started/not-started split: a session whose tmux is not
             running still has a checkout to open. No name or folder passed —
             the sidebar already says which session this is. -->
        <PaneStatusBar
          v-if="statusBarRow"
          testid="terminal-pane-statusbar"
          :error="statusBarError"
          :editor-title="editorTitle"
          @open-editor="runStatusBarAction(OpenSessionInEditor)"
          @reveal="runStatusBarAction(RevealSession)"
        >
          <SessionStatusChips
            :git="sessionGit"
            :pull-request="sessionPullRequest"
            :pull-request-error="sessionPullRequestError"
            @refresh-pull-request="refreshSessionStatus({ refreshPullRequest: true })"
          />
          <template #actions>
            <AppTooltip text="Tasks">
              <button
                type="button"
                class="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-[7px] text-text-3 hover:bg-chip hover:text-text"
                aria-label="Tasks"
                data-testid="terminal-statusbar-tasks"
                @click="emit('open-tasks')"
              >
                <IconListTodo class="size-3.5" />
              </button>
            </AppTooltip>
          </template>
        </PaneStatusBar>

        <p
          v-if="actionError"
          class="shrink-0 border-b border-border px-3 py-1.5 text-[11.5px] text-severity-error"
          data-testid="terminal-action-error"
        >
          {{ actionError }}
        </p>

        <template v-if="visible && !notStarted">
          <!-- The gap has to be said out loud. Recovering the view without
               naming what it cost would be worse than the teardown this
               replaced, which at least told the truth loudly. -->
          <div
            v-if="outputDropped"
            class="flex shrink-0 items-start gap-2 border-b border-border bg-raised px-3 py-2"
            data-testid="terminal-output-dropped"
          >
            <IconInfo class="mt-px size-3.5 shrink-0 text-severity-warning" />
            <p class="min-w-0 flex-1 text-[11.5px] leading-relaxed text-text-3">
              Output arrived faster than this window could draw it, so some of it was dropped. These panes were
              repainted from tmux — what they show now is current, and their scrollback is tmux's, not what streamed
              here before the gap.
            </p>
            <button
              type="button"
              class="flex size-4 shrink-0 cursor-pointer items-center justify-center rounded text-text-4 hover:bg-chip hover:text-text"
              data-testid="terminal-output-dropped-dismiss"
              aria-label="Dismiss"
              @click="visible?.dismissOutputDropped()"
            >
              <IconX class="size-3" />
            </button>
          </div>

          <!-- Names tmux's rule rather than reporting a fault: the grid is
               smaller (or larger) than the pane because another client attached
               to this session is the one deciding its size. -->
          <div
            v-if="sizeConstraint"
            class="flex shrink-0 items-start gap-2 border-b border-border bg-raised px-3 py-2"
            data-testid="terminal-size-constraint"
          >
            <IconInfo class="mt-px size-3.5 shrink-0 text-text-4" />
            <p class="min-w-0 flex-1 text-[11.5px] leading-relaxed text-text-3">
              tmux is drawing this window at
              <span class="font-mono text-text-2"
                >{{ sizeConstraint.granted.cols }}×{{ sizeConstraint.granted.rows }}</span
              >, not the
              <span class="font-mono text-text-2">{{ sizeConstraint.voted.cols }}×{{ sizeConstraint.voted.rows }}</span>
              this pane fits. Every client attached to a session shares one grid per window, so another attached client
              is deciding the size. Detach it, or change tmux's
              <span class="font-mono text-text-2">window-size</span> option, to use the whole pane.
            </p>
            <button
              type="button"
              class="flex size-4 shrink-0 cursor-pointer items-center justify-center rounded text-text-4 hover:bg-chip hover:text-text"
              data-testid="terminal-size-constraint-dismiss"
              aria-label="Dismiss"
              @click="visible?.dismissSizeConstraint()"
            >
              <IconX class="size-3" />
            </button>
          </div>
        </template>

        <!-- Rendered outside the v-if and merely hidden without a session: a
             pooled session's terminals must keep their elements, and unmounting
             the hosts would cost every one of them its screen. -->
        <div class="relative min-h-0 min-w-0 flex-1 flex-col" :class="visible ? 'flex' : 'hidden'">
          <template v-for="pane in paneSessions" :key="pane.slug">
            <TerminalTab
              v-for="tab in pane.tabs"
              :key="tab.uid"
              :tab="tab"
              :session="pane.entry"
              :active="pane.entry === visible && tab.windowId === pane.activeWindowId"
            />
          </template>
          <template v-if="visible">
            <div
              v-if="!tabs.length && status !== 'ended'"
              class="flex flex-1 items-center justify-center font-mono text-xs text-text-4"
            >
              Attaching…
            </div>

            <!-- Floated over the pane rather than placed above it: a bar in the
                 flex column would shrink the pane's box, and the box is what
                 this client votes tmux's window size from — opening a find bar
                 would reflow the session for every client attached to it. -->
            <div
              v-if="search.open && status !== 'ended'"
              class="absolute right-5 top-3 z-10 flex items-center gap-1 rounded-md border border-strong bg-raised/95 py-1 pl-2 pr-1 shadow-lg"
              data-testid="terminal-search"
            >
              <IconSearch class="size-3 shrink-0 text-text-4" />
              <input
                ref="searchInput"
                :value="search.query"
                type="text"
                placeholder="Find"
                spellcheck="false"
                class="w-44 bg-transparent text-[11.5px] text-text outline-none placeholder:text-text-4"
                data-testid="terminal-search-input"
                @input="visible?.setSearchQuery(($event.target as HTMLInputElement).value)"
                @keydown.enter.exact.prevent="visible?.findNext()"
                @keydown.enter.shift.prevent="visible?.findPrevious()"
                @keydown.esc.prevent="visible?.closeSearch()"
              />
              <span
                class="min-w-[54px] shrink-0 text-right font-mono text-[10.5px] text-text-4"
                data-testid="terminal-search-count"
                >{{ searchLabel() }}</span
              >
              <button
                type="button"
                class="flex size-5 shrink-0 cursor-pointer items-center justify-center rounded text-text-4 hover:bg-chip hover:text-text"
                aria-label="Previous match"
                data-testid="terminal-search-prev"
                @click="visible?.findPrevious()"
              >
                <IconChevronUp class="size-3" />
              </button>
              <button
                type="button"
                class="flex size-5 shrink-0 cursor-pointer items-center justify-center rounded text-text-4 hover:bg-chip hover:text-text"
                aria-label="Next match"
                data-testid="terminal-search-next"
                @click="visible?.findNext()"
              >
                <IconChevronDown class="size-3" />
              </button>
              <button
                type="button"
                class="flex size-5 shrink-0 cursor-pointer items-center justify-center rounded text-text-4 hover:bg-chip hover:text-text"
                aria-label="Close find"
                data-testid="terminal-search-close"
                @click="visible?.closeSearch()"
              >
                <IconX class="size-3" />
              </button>
            </div>

            <!-- New output keeps landing below the fold while the viewport is
                 scrolled up; this is the way back to the live tail. -->
            <Transition name="tail-pill">
              <button
                v-if="activeScrolledUp && status !== 'ended'"
                type="button"
                class="absolute bottom-3 right-5 z-10 flex cursor-pointer items-center gap-1.5 rounded-full border border-strong bg-raised/95 px-3 py-1.5 text-[11.5px] text-text-2 shadow-lg hover:text-text"
                data-testid="terminal-scroll-to-bottom"
                @click="visible?.scrollToBottom()"
              >
                <IconArrowDown class="size-3" />Scroll to bottom
              </button>
            </Transition>

            <!-- Selecting a session never starts it: starting runs the
                 session's own agent command, so it is offered here and taken
                 on a click. -->
            <div
              v-if="notStarted"
              class="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-app/95 px-10 text-center"
              data-testid="terminal-session-not-started"
            >
              <IconTerminal class="size-6 text-text-4" />
              <div class="text-[13.5px] font-semibold">
                {{
                  chatAttached ? 'Chat not running' : scratchAttached ? 'Terminal not started' : 'Session not started'
                }}
              </div>
              <p v-if="chatAttached" class="max-w-[420px] text-xs leading-relaxed text-text-3">
                This chat is stopped. Resuming it launches the agent again in its workspace, picking the conversation
                back up where the agent itself can.
              </p>
              <p v-else-if="scratchAttached" class="max-w-[420px] text-xs leading-relaxed text-text-3">
                The scratch terminal is not running. Starting it opens a shell in your home directory, and every tab you
                add opens there too.
              </p>
              <p v-else class="max-w-[420px] text-xs leading-relaxed text-text-3">
                No terminal is running for <span class="font-mono text-text-2">{{ activeSlug }}</span> yet. Starting it
                opens this session's configured windows and runs its agent command.
              </p>
              <InlineError
                v-if="startError"
                testid="terminal-start-error"
                variant="line"
                class="max-w-[420px]"
                :message="startError"
              />
              <div class="mt-1 flex items-center gap-2">
                <BaseButton
                  size="sm"
                  :busy="starting === activeSlug"
                  data-testid="terminal-start-session"
                  @click="startSession(activeSlug)"
                >
                  <template #icon><IconPlay class="size-3.5" /></template>
                  {{
                    starting === activeSlug
                      ? chatAttached
                        ? 'Resuming…'
                        : 'Starting…'
                      : chatAttached
                        ? 'Resume chat'
                        : scratchAttached
                          ? 'Start terminal'
                          : 'Start session'
                  }}
                </BaseButton>
                <BaseButton variant="secondary" size="sm" data-testid="terminal-close-session" @click="closeSession"
                  >Close</BaseButton
                >
              </div>
            </div>

            <div
              v-else-if="status === 'ended'"
              class="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-app/95 px-10 text-center"
              data-testid="terminal-session-ended"
            >
              <div class="text-[13.5px] font-semibold">
                {{ endReason === 'disconnected' ? 'Terminal stream lost' : 'Session ended' }}
              </div>
              <p class="max-w-[420px] text-xs leading-relaxed text-text-3" data-testid="terminal-session-ended-reason">
                {{ sessionError }}
              </p>
              <div class="mt-1 flex items-center gap-2">
                <button
                  type="button"
                  class="flex cursor-pointer items-center gap-1.5 rounded border border-strong px-3 py-1.5 text-xs text-text-2 hover:text-text"
                  data-testid="terminal-reconnect"
                  @click="visible?.reconnect()"
                >
                  <IconRefreshCw class="size-3" />Reconnect
                </button>
                <button
                  type="button"
                  class="cursor-pointer rounded border border-strong px-3 py-1.5 text-xs text-text-2 hover:text-text"
                  data-testid="terminal-close-session"
                  @click="closeSession"
                >
                  Close session
                </button>
              </div>
            </div>
          </template>
        </div>
      </div>
    </div>

    <ActionInputsDialog
      v-if="actionInputs"
      :action-label="actionInputs.action.label"
      :inputs="actionInputs.action.inputs ?? []"
      :busy="actionInputsBusy"
      :error="actionInputsError"
      @close="cancelActionInputs"
      @submit="submitActionInputs"
    />
    <SessionDetailDialog v-if="sessionDetail" :detail="sessionDetail" @close="closeSessionDetail" />
    <SessionRenameDialog
      v-if="renaming"
      :name="renaming.name"
      :busy="renameBusy"
      :error="renameError"
      @close="cancelRename"
      @save="submitRename"
    />
    <ConfirmationDialog
      v-if="confirmOpen && confirmOptions"
      :title="confirmOptions.title"
      :description="confirmOptions.description"
      :details="confirmOptions.details"
      :confirm-label="confirmOptions.confirmLabel"
      :busy="confirmBusy"
      :error="confirmError"
      testid="session-confirmation"
      @confirm="runConfirm"
      @cancel="cancelConfirm"
    />
  </div>
</template>

<style scoped>
/* Pinned under the tree rather than scrolling with it: a legend that scrolls
   away is one you cannot consult at the moment you need it. */
.tree-hints {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
  padding: 8px 12px;
  border-top: 1px solid var(--color-border);
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--color-text-4);
  user-select: none;
}

.session-row {
  position: relative;
  display: flex;
  height: 30px;
  width: 100%;
  align-items: center;
  gap: 8px;
  padding-left: 20px;
  padding-right: 12px;
  text-align: left;
  color: var(--color-text);
  cursor: pointer;
}
/* A repository's heading. Like the pinned section's it holds a control, which a
   button cannot contain, so it is a div wearing a button's role. */
.repo-group {
  display: flex;
  height: 36px;
  width: 100%;
  align-items: center;
  gap: 8px;
  padding-left: 12px;
  padding-right: 12px;
  text-align: left;
  cursor: pointer;
}
.repo-group:hover {
  background: var(--color-chip);
}
.repo-group:focus-visible {
  outline: none;
}
/* One cell, two occupants: the count is what the row says at rest, the add
   button what it offers under the pointer. */
.group-trailing {
  display: grid;
  margin-left: auto;
  min-width: 18px;
  height: 18px;
  flex: none;
  align-self: center;
}
.group-count,
.group-add {
  grid-area: 1 / 1;
}
.group-count {
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: none;
}
.repo-group:hover .group-count,
.group-trailing:focus-within .group-count {
  opacity: 0;
}
.repo-group:hover .group-add,
.group-add:focus-visible {
  opacity: 1;
}
/* The pinned section's heading. This one holds
   the terminal's own controls, which a button cannot contain, so it is a row
   wearing the same box. */
.group-row {
  position: relative;
  display: flex;
  height: 36px;
  width: 100%;
  align-items: center;
  gap: 8px;
  padding-left: 12px;
  padding-right: 12px;
  text-align: left;
  cursor: pointer;
}
.group-row:hover,
.group-row.menu-open {
  background: var(--color-chip);
}
.group-row:focus-visible {
  outline: none;
}
.session-row:hover,
.session-row.menu-open {
  background: var(--color-chip);
}
/* No focus styling of its own: the walk activates the row it lands on, so the
   rail and the accent are already the mark that follows the cursor. The outline
   has to be set to none rather than merely dropped — the UA draws its own. */
.session-row:focus-visible {
  outline: none;
}
/* No fill: the rail and the accent are enough to find the attached row, and
   leaving the surface alone also lets it keep its hover feedback. */
.session-row-attached {
  font-weight: 500;
  color: var(--color-accent);
}
/* The tree connector is drawn, not typed: a box-drawing glyph is only as tall as
   its font size, so stacked rows would show a gap where the TUI's cell grid
   shows an unbroken line. ::before is the vertical, stopped at the elbow on the
   last row; ::after is the tick into the name. */
.window-row {
  position: relative;
  display: flex;
  height: 28px;
  width: 100%;
  align-items: center;
  gap: 8px;
  padding-left: 40px;
  padding-right: 12px;
  text-align: left;
  color: var(--color-text-2);
  cursor: pointer;
}
.window-row:hover,
.window-row.menu-open {
  background: var(--color-chip);
}
.window-row:focus-visible {
  outline: none;
}
.window-row-active {
  font-weight: 500;
  color: var(--color-accent);
}

/* The travelling selection markers. Out of flow, so moving one costs no layout
   anywhere else, and its height is free to animate between the two row heights.
   The z-index is load-bearing: rows and panels are positioned boxes as well, so
   without it a rail is painted over by whichever of them comes after it. */
.tree-rail {
  /* Square ends, not rounded: the attached session's row and its active
     window's are usually adjacent, and two rounded bars stacked pinch the edge
     at the seam instead of reading as one continuous mark. */
  position: absolute;
  left: 0;
  top: 0;
  z-index: 1;
  width: 3px;
  background: var(--color-accent);
  opacity: 0;
  pointer-events: none;
  transition:
    transform 0.2s cubic-bezier(0.2, 0, 0, 1),
    height 0.2s cubic-bezier(0.2, 0, 0, 1),
    opacity 0.12s ease;
}
.tree-rail-shown {
  opacity: 1;
}
.window-row::before {
  content: '';
  position: absolute;
  left: 26px;
  top: 0;
  bottom: 0;
  border-left: 1px solid var(--color-strong);
}
.window-row::after {
  content: '';
  position: absolute;
  left: 26px;
  top: 50%;
  width: 9px;
  border-top: 1px solid var(--color-strong);
}
.window-row-last::before {
  bottom: 50%;
}
/* The pinned section lists its tabs where a repository lists its sessions, so
   they take that row's box and drop the connector — there is no row above them
   for one to hang from. */
.window-row-flush {
  height: 30px;
  padding-left: 20px;
}
.window-row-flush::before,
.window-row-flush::after {
  display: none;
}

/* The window well's insertion marker, on the slot because the row's own
   ::before and ::after are the tree connector. Inset like the hub sidebar's. */
.window-slot {
  position: relative;
}
.window-slot.drop-before {
  box-shadow: inset 0 2px 0 0 var(--color-accent);
}
.window-slot.drop-after {
  box-shadow: inset 0 -2px 0 0 var(--color-accent);
}

/* Tree motion, fast enough to read as instant: rows fade/slide over 150ms, a
   leaving row drops out of flow so its neighbors glide up through .tree-move
   (FLIP) instead of snapping, and expand/collapse animates the measured height
   set by the expand* hooks. */
.tree-enter-active,
.tree-leave-active,
.tree-move {
  transition:
    opacity 0.15s ease,
    transform 0.15s ease;
}
.tree-enter-from,
.tree-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
.tree-leave-active {
  position: absolute;
  left: 0;
  right: 0;
}
/* Decelerating rather than `ease`: a disclosure that leaves at full speed and
   settles reads as instant, where ease-in-out spends its first frames barely
   moving and reads as lag. */
.tree-expand-enter-active,
.tree-expand-leave-active {
  overflow: hidden;
  transition: height 0.18s cubic-bezier(0.2, 0, 0, 1);
}

/* The first fill is not a change to make legible: every row is entering, so the
   whole panel would animate in as one block and the rails would force layout
   per frame chasing it. Motion starts once the tree is on screen. */
.tree-settling
  :is(.tree-enter-active, .tree-leave-active, .tree-move, .tree-expand-enter-active, .tree-expand-leave-active) {
  transition: none;
}

@media (prefers-reduced-motion: reduce) {
  .tree-enter-active,
  .tree-leave-active,
  .tree-move,
  .tree-expand-enter-active,
  .tree-expand-leave-active,
  .tree-rail {
    transition: none;
  }
}

/* Two fixed columns, so session and window names stay aligned whatever a row
   is carrying. The second is where the status glyph sits, and the control that
   replaces it on hover shares that cell rather than crowding in beside it — a
   session's menu, a window's close. Anything else goes in the first.

   A row whose second cell has no other occupant keeps its status there, and its
   tooltip with it: that is a listed window, which has no live client to close.
   .has-swap is what says otherwise. */
.row-trailing,
.window-trailing {
  display: grid;
  grid-template-columns: 18px 18px;
  width: 36px;
  height: 18px;
  flex: none;
  align-self: center;
}
.row-lead {
  grid-area: 1 / 1;
}
.row-status,
.window-status,
.row-swap {
  grid-area: 1 / 2;
}
.row-status,
.window-status {
  display: flex;
  width: 18px;
  height: 18px;
  flex: none;
  align-items: center;
  justify-content: center;
}
.row-status,
.window-row.has-swap .window-status {
  pointer-events: none;
}
.row-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 5px;
  color: var(--color-text-4);
  cursor: pointer;
  opacity: 0;
}
.row-action:hover,
.row-action[aria-expanded='true'] {
  background: var(--color-app);
  color: var(--color-text);
}
/* The pinned section's heading carries a session's controls, so it reveals them
   on hover like a session row. It is a .group-row rather than a .session-row —
   its own row is the heading — and leaving it out of these left the scratch
   terminal's + and ⋮ at opacity 0 with no way to reach them by mouse. */
.session-row:hover .row-action,
.group-row:hover .row-action,
.window-row:hover .row-action,
.row-action:focus-visible,
.session-row.menu-open .row-action,
.group-row.menu-open .row-action,
.window-row.menu-open .row-action {
  opacity: 1;
}
.session-row:hover .row-status,
.group-row:hover .row-status,
.session-row.menu-open .row-status,
.group-row.menu-open .row-status,
.row-trailing:focus-within .row-status {
  opacity: 0;
}
.window-row.has-swap:hover .window-status,
.window-row.has-swap .window-trailing:focus-within .window-status {
  opacity: 0;
}
</style>
