<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, ref, toRef, watch } from 'vue'
import IconArrowDown from '~icons/lucide/arrow-down'
import IconCheck from '~icons/lucide/check'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronUp from '~icons/lucide/chevron-up'
import IconCircleAlert from '~icons/lucide/circle-alert'
import IconCopy from '~icons/lucide/copy'
import IconCode from '~icons/lucide/code'
import IconFolderOpen from '~icons/lucide/folder-open'
import IconInfo from '~icons/lucide/info'
import IconPlay from '~icons/lucide/play'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconSearch from '~icons/lucide/search'
import IconServer from '~icons/lucide/server'
import IconTerminal from '~icons/lucide/terminal'
import IconX from '~icons/lucide/x'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import ChangesLoading from '../git/ChangesLoading.vue'
import DrivePage from '../drive/DrivePage.vue'
import TasksView from '../tasks/TasksView.vue'
import PanelLayoutHost from '../panels/PanelLayoutHost.vue'
import SessionPanelPicker from './SessionPanelPicker.vue'
import SessionTabAdd from './SessionTabAdd.vue'
import SessionTabIcon from './SessionTabIcon.vue'
import SessionTagsStatus from '../SessionTagsStatus.vue'
import SessionStatusChips from '../SessionStatusChips.vue'
import TerminalTab from '../TerminalTab.vue'
import { useClipboard } from '@hive/ui/composables/useClipboard'
import { useEditorSettings } from '../../composables/useEditorSettings'
import { useSessionStatus } from '../../composables/useSessionStatus'
import { openLink } from '../../composables/useXtermPane'
import type { AgentIconID } from '../../lib/agentIcon'
import type { DriveScope } from '../../lib/drive'
import { launcherIconComponent } from '../../lib/launcherIcons'
import {
  closePanel,
  closeTab,
  focusPanel,
  openTab,
  panelById,
  panelWithTab,
  panelsOf,
  placeTab,
  type Panel,
  type PanelTab,
} from '../../lib/panelLayout'
import { offscreenWindows, placeWindow, terminalPanel, windowIdOf } from '../../lib/windowTabs'
import { activePaneOf } from '../../lib/terminalLayout'
import { terminalTreeFocused } from '../../lib/terminalTree'
import { useDriveActivity } from '../../stores/useDriveActivity'
import { usePanelLayouts } from '../../stores/usePanelLayouts'
import { useTasks } from '../../stores/useTasks'
import { useTerminalStatusBar } from '../../stores/useTerminalStatusBar'
import { useViewKeys } from '../../stores/useViewKeys'
import {
  OpenSessionInEditor,
  RevealSession,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import { docsScopeOf, viewOfTab, viewTab, withDocsScope } from './panelViews'
import { useTerminalModeContext } from './terminalModeContext'
import { useSessionLayout, type SessionDriveTarget } from '../panels/useSessionLayout'
import {
  anchorPanelOf,
  closePanelRule,
  focusRule,
  moveRule,
  openViewRule,
  resizeRule,
  turnRule,
} from '../panels/sessionTabRefs'

// Its diff library and highlighter load only when a Diff tab opens.
// eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- the type-aware lint cannot read a .vue import
const ChangesView = defineAsyncComponent({
  loader: () => import('../git/ChangesView.vue'),
  // eslint-disable-next-line @typescript-eslint/no-unsafe-assignment -- the type-aware lint cannot read a .vue import
  loadingComponent: ChangesLoading,
  delay: 0,
})

const props = defineProps<{
  paintedActive: boolean
  active: boolean
  drive: SessionDriveTarget | null
  /** An app overlay has the keyboard, so a view under it must not take its keys. */
  overlayOpen?: boolean
}>()
const emit = defineEmits<{
  'session-repo-key': [repoKey: string]
  setup: []
}>()

const { pool, tree, attach, ops, sessions, panels } = useTerminalModeContext()
const visible = computed(() => pool.visible)

const tabs = computed(() => visible.value?.tabs.value ?? [])
const activeWindowId = computed(() => visible.value?.activeWindowId.value ?? '')
const activeScrolledUp = computed(() =>
  tabs.value.some((tab) => tab.windowId === activeWindowId.value && activePaneOf(tab)?.scrolledUp),
)
const status = computed(() => visible.value?.status.value ?? 'connecting')
const endReason = computed(() => visible.value?.endReason.value ?? null)
const notStarted = computed(() => endReason.value === 'not-started' || endReason.value === 'start-failed')
const canRestart = computed(() => ['terminated', 'completed', 'stopped', 'interrupted'].includes(endReason.value ?? ''))
const endedTitle = computed(() => {
  switch (endReason.value) {
    case 'terminated':
      return 'Tmux session terminated'
    case 'completed':
      return 'Session completed'
    case 'stopped':
      return 'Terminal stopped'
    case 'interrupted':
      return 'Session launch did not finish'
    case 'disconnected':
      return 'Terminal connection lost'
    case 'exited':
      return 'Terminal connection ended'
    default:
      return 'Terminal error'
  }
})
const scratchAttached = computed(() => !!tree.attachedRow && tree.isScratch(tree.attachedRow))
const chatAttached = computed(() => !!tree.attachedRow && tree.isChat(tree.attachedRow))
const sessionError = computed(() => visible.value?.error.value ?? '')
const actionError = computed(() => visible.value?.actionError.value || ops.treeError)
const sizeConstraint = computed(() => visible.value?.sizeConstraint.value ?? null)
const outputDropped = computed(() => visible.value?.outputDropped.value ?? false)
const { copy: copyStartError, status: startErrorCopyStatus } = useClipboard()
const startErrorCopyLabel = computed(() => {
  if (startErrorCopyStatus.value === 'success') return 'Copied'
  if (startErrorCopyStatus.value === 'error') return 'Copy failed'
  return 'Copy error'
})

const search = computed(() => visible.value?.search.value ?? { open: false, query: '', matches: 0, index: 0 })
const searchInput = ref<HTMLInputElement | null>(null)

// The find bar opens from inside the pane (the terminal owns its keys), so the
// focus move follows the state rather than the handler that set it.
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

// Only a hive session gets a status bar: the scratch terminal and the pinned
// chats are tmux sessions with no checkout behind them.
const { showStatusBar } = useTerminalStatusBar()
const { title: editorTitle, refresh: reloadEditor } = useEditorSettings()
watch(
  () => props.paintedActive,
  (shown) => {
    if (shown) void reloadEditor()
  },
  { immediate: true },
)

// A stopped session still has a checkout to open, so the actions do not ask
// whether it is running, or whether the status bar is on.
const actionRow = computed(() => tree.activeSessions.find((row) => row.slug === pool.visibleSlug) ?? null)
const statusBarRow = computed(() => {
  if (!showStatusBar.value || !pool.visibleSlug) return null
  const slug = pool.visibleSlug
  const onServer = tree.servers.flatMap((list) => list.sessions).filter((row) => row.state === 'active')
  return actionRow.value ?? onServer.find((row) => row.slug === slug) ?? null
})
const footerRow = computed(() => {
  if (!showStatusBar.value) return null
  return statusBarRow.value ?? (panels.row?.server ? panels.row : null)
})
// A server session's checkout is on the server: Changes and Tasks read this machine.
const statusBarLocal = computed(() => !!statusBarRow.value && !tree.isServerSession(statusBarRow.value))
// An offline server's row keeps its bar and the badge that names the server,
// and asks that server nothing.
const statusBarSessionId = computed(() => (statusBarRow.value?.offline ? '' : (statusBarRow.value?.id ?? '')))
// The server a session runs on is named here, so nobody types into a server's
// agent thinking it is this machine's.
const statusBarHost = computed(() => {
  const server = footerRow.value?.server
  if (!server) return null
  const ssh = tree.servers.find((list) => list.server.name === server)?.server.ssh ?? ''
  return { name: server, ssh, hue: tree.hue(server) }
})
const {
  git: sessionGit,
  pullRequest: sessionPullRequest,
  pullRequestError: sessionPullRequestError,
  refresh: refreshSessionStatus,
} = useSessionStatus(statusBarSessionId, () => props.paintedActive)
const statusBarError = ref('')

async function runStatusBarAction(action: (id: string) => Promise<void>): Promise<void> {
  const id = actionRow.value?.id
  if (!id) return
  statusBarError.value = ''
  try {
    await action(id)
  } catch (error) {
    statusBarError.value = error instanceof Error ? error.message : String(error)
  }
}

// owner/repo is exactly the hc repoKey format. Reported continuously, so App.vue
// can scope Tasks to the attached session's repo from any entry point and not
// only from this bar's own button. Tasks are this machine's, so a session on a
// server has no repo to scope them to.
const sessionRepoKey = computed(() => {
  if (!statusBarLocal.value) return ''
  const git = sessionGit.value
  return git?.resolved && git.owner && git.repo ? `${git.owner}/${git.repo}` : ''
})
watch(sessionRepoKey, (key) => emit('session-repo-key', key), { immediate: true })

const layout = computed(() => panels.layout)
const panelList = computed(() => panelsOf(layout.value))
const activeWindowOnScreen = computed(() => panels.onScreen.has(activeWindowId.value))
const statusPanel = computed(() => terminalPanel(layout.value))

function windowName(windowId: string): string {
  const tab = tabs.value.find((candidate) => candidate.windowId === windowId)
  return tab?.name || windowId
}

function tabLabel(tab: PanelTab): string {
  const windowId = windowIdOf(tab)
  return windowId ? windowName(windowId) : (viewOfTab(tab)?.label ?? tab.id)
}

const driveActivity = useDriveActivity()

function needsApproval(windowId: string): boolean {
  const row = panels.row
  return !!row && tree.windowStatus(row, windowId)?.status === 'approval'
}

function tabAgent(tab: PanelTab): AgentIconID | undefined {
  const row = panels.row
  const windowId = windowIdOf(tab)
  return row && windowId ? tree.windowAgent(row, windowId, windowName(windowId)) : undefined
}

function tabAttention(tab: PanelTab): boolean {
  const windowId = windowIdOf(tab)
  if (windowId) return needsApproval(windowId)
  return tab.kind === 'docs' && !!shownDrive.value && driveActivity.isUnseen(shownDrive.value.session)
}

function isWindowTab(tab: PanelTab): boolean {
  return !!windowIdOf(tab)
}

// A running window has a tab and a tree row, so closing its tab closes the window,
// through the same confirmation as the tree.
function closePanelTab(panelId: string, tabId: string): void {
  const windowId = windowIdOf(panelById(layout.value, panelId)?.tabs.find((candidate) => candidate.id === tabId))
  if (windowId) void ops.requestCloseWindow(pool.visibleSlug, windowId, windowName(windowId))
  else panels.apply((current) => closeTab(current, panelId, tabId))
}

function placeWindowHere(windowId: string, panelId: string): void {
  panels.apply((current) => placeWindow(current, windowId, panelId))
  panels.syncWindowOrder(windowId)
}

// The mode hides on a trip to the hub, and props.drive empties with it. A tab
// that follows would unmount and lose its place for nothing.
// eslint-disable-next-line vue/no-setup-props-reactivity-loss -- the first value; the watcher below follows it
const shownDrive = ref(props.drive)
watch(
  () => props.drive,
  (drive) => {
    if (props.active) shownDrive.value = drive
  },
)

const changesSession = computed(() => {
  const row = panels.row
  return row && tree.isHiveSession(row) && !tree.isServerSession(row) ? row.id : ''
})
const availableViews = computed(() => (panels.row ? panels.viewsFor(panels.row) : []))
const canAddTerminal = computed(() => !!panels.row && !tree.isChat(panels.row))
// An agent needs a running tmux session to open in, as the "+" menu says in its own words.
const agents = computed(() =>
  changesSession.value && panels.row && tree.rowRunning(panels.row) && !ops.agentWindowBusy ? ops.agentProfiles : [],
)

const pickerWindows = computed(() =>
  offscreenWindows(
    layout.value,
    tabs.value.map((tab) => tab.windowId),
  ).map((windowId) => ({ windowId, name: windowName(windowId) })),
)

function openDiff(): void {
  panels.apply((current) => openTab(current, viewTab('diff')))
}

function viewShown(panel: Panel, tabId: string): boolean {
  return !panel.picking && panel.activeTab === tabId
}

function viewHasKeys(focused: boolean, shown: boolean): boolean {
  return props.active && !props.overlayOpen && !terminalTreeFocused.value && focused && shown
}

// Told to the keymap, which otherwise answers `/` and the other bare keys itself.
const viewKeys = useViewKeys()
const frontViewHasKeys = computed(() => {
  const panel = panelList.value.find((candidate) => candidate.id === layout.value.focusedPanel)
  const front = panel && !panel.picking ? panel.tabs.find((tab) => tab.id === panel.activeTab) : undefined
  return !!front && !!viewOfTab(front)?.ownsKeys && viewHasKeys(true, true)
})
watch(frontViewHasKeys, (owns) => viewKeys.setOwned(owns), { immediate: true })
onBeforeUnmount(() => viewKeys.setOwned(false))

const { setRepoKey } = useTasks()
const tasksTabOpen = computed(() => panelList.value.some((panel) => panel.tabs.some((tab) => tab.kind === 'tasks')))
watch(
  [tasksTabOpen, sessionRepoKey],
  ([open, repoKey]) => {
    if (open && repoKey) setRepoKey(repoKey)
  },
  { immediate: true },
)

const driveScope = computed<DriveScope | null>(() => {
  const own = docsScopeOf(layout.value)
  if (own) return own
  return shownDrive.value ? { ...shownDrive.value, path: null } : null
})

function scopeAt(path: string | null): DriveScope | null {
  const base = docsScopeOf(layout.value) ?? (props.drive ? { ...props.drive, path: null } : null)
  return base && path ? { ...base, path } : base
}

// The route names the session an ask is checked against, and the pool the one
// its layout is written to. During a cold attach the pool still shows the
// outgoing session, so an ask waits until the two agree.
useSessionLayout('terminal', toRef(props, 'drive'), () => props.active && pool.activeSlug === pool.visibleSlug, {
  // Tabs are keyed by the row's id, so a request has to wait for the row.
  ready: () => !!panels.row && pool.activeSlug === pool.visibleSlug,
  reveal: (path) => panels.apply((current) => withDocsScope(openTab(current, viewTab('docs')), scopeAt(path))),
  open: (view, path, placement, anchor) => {
    if (!panels.row || !panels.viewsFor(panels.row).includes(view)) return
    panels.apply((current) => {
      const placed = openViewRule(current, viewTab(view), placement, anchor)
      return view === 'docs' ? withDocsScope(placed, scopeAt(path)) : placed
    })
  },
  close: (view) =>
    panels.apply((current) => {
      const tab = viewTab(view)
      const panel = panelWithTab(current, tab.id)
      return panel ? closeTab(current, panel.id, tab.id) : current
    }),
  openWindow: (windowId, placement, anchor) => {
    if (panels.row && !tree.isChat(panels.row)) {
      panels.placeArrivingWindow(windowId, placement, anchorPanelOf(layout.value, anchor))
    }
  },
  move: (tab, placement, anchor) => {
    panels.apply((current) => moveRule(current, tab, placement, anchor))
    if (tab.view === 'terminal' && tab.window) panels.syncWindowOrder(tab.window)
  },
  focus: (tab) => panels.apply((current) => focusRule(current, tab)),
  closePanel: (tab) => panels.apply((current) => closePanelRule(current, tab)),
  resize: (tab, share) => panels.apply((current) => resizeRule(current, tab, share)),
  turn: (tab) => panels.apply((current) => turnRule(current, tab)),
})

const panelLayouts = usePanelLayouts()
watch([() => props.active, () => panels.key], ([active, key]) => panelLayouts.markOnScreen(active ? key : ''), {
  immediate: true,
})
onBeforeUnmount(() => panelLayouts.markOnScreen(''))

// The full page knows nothing about authors. A Drive tab on screen is the user
// reading, and one behind another tab is not, so a write there still earns its dot.
const docsReader = computed(() => {
  const shownDocs = panelList.value.some((panel) => viewShown(panel, 'docs'))
  return shownDocs && props.active ? (shownDrive.value?.session ?? undefined) : undefined
})
watch(
  docsReader,
  (session, before) => {
    if (before !== undefined) driveActivity.unregisterVisible(before)
    if (session !== undefined) driveActivity.registerVisible(session)
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  if (docsReader.value !== undefined) driveActivity.unregisterVisible(docsReader.value)
})
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <div
      v-if="!visible"
      class="flex flex-1 flex-col items-center justify-center gap-2 px-10 text-center"
      data-testid="terminal-no-session"
    >
      <IconTerminal class="size-6 text-text-4" />
      <p class="text-xs text-text-3">Select a session to attach.</p>
    </div>

    <p
      v-if="actionError"
      class="shrink-0 border-b border-border px-3 py-1.5 text-caption text-severity-error"
      data-testid="terminal-action-error"
    >
      {{ actionError }}
    </p>

    <template v-if="visible && !notStarted">
      <!-- Recovering the view without naming what it cost would hide the gap. -->
      <div
        v-if="outputDropped"
        class="flex shrink-0 items-start gap-2 border-b border-border bg-raised px-3 py-2"
        data-testid="terminal-output-dropped"
      >
        <IconInfo class="mt-px size-3.5 shrink-0 text-severity-warning" />
        <p class="min-w-0 flex-1 text-caption leading-relaxed text-text-3">
          Output arrived faster than this window could draw it, so some of it was dropped. These panes were repainted
          from tmux — what they show now is current, and their scrollback is tmux's, not what streamed here before the
          gap.
        </p>
        <IconButton
          label="Dismiss"
          :icon="IconX"
          size="sm"
          data-testid="terminal-output-dropped-dismiss"
          @click="visible?.dismissOutputDropped()"
        />
      </div>

      <!-- tmux's rule, not a fault: another attached client decides the grid size. -->
      <div
        v-if="sizeConstraint"
        class="flex shrink-0 items-start gap-2 border-b border-border bg-raised px-3 py-2"
        data-testid="terminal-size-constraint"
      >
        <IconInfo class="mt-px size-3.5 shrink-0 text-text-4" />
        <p class="min-w-0 flex-1 text-caption leading-relaxed text-text-3">
          tmux is drawing this window at
          <span class="font-mono text-text-2">{{ sizeConstraint.granted.cols }}×{{ sizeConstraint.granted.rows }}</span
          >, not the
          <span class="font-mono text-text-2">{{ sizeConstraint.voted.cols }}×{{ sizeConstraint.voted.rows }}</span>
          this pane fits. Every client attached to a session shares one grid per window, so another attached client is
          deciding the size. Detach it, or change tmux's
          <span class="font-mono text-text-2">window-size</span> option, to use the whole pane.
        </p>
        <IconButton
          label="Dismiss"
          :icon="IconX"
          size="sm"
          data-testid="terminal-size-constraint-dismiss"
          @click="visible?.dismissSizeConstraint()"
        />
      </div>
    </template>

    <PanelLayoutHost
      :layout="layout"
      :apply="panels.apply"
      :move-tab="panels.moveTabTo"
      :tab-label="tabLabel"
      :tab-attention="tabAttention"
      :tab-mono="isWindowTab"
      :has-content="(tab) => !!viewOfTab(tab)"
      :content-key="panels.key"
      :hidden="!visible"
      @close-tab="closePanelTab"
    >
      <template #icon="{ tab }">
        <SessionTabIcon :kind="tab.kind" :agent="tabAgent(tab)" />
      </template>
      <template #add="{ panel }">
        <SessionTabAdd :row="panels.row" :panel="panel.id" />
      </template>
      <template #actions="{ lead }">
        <template v-if="lead">
          <span
            v-if="statusBarError"
            class="max-w-40 truncate px-1.5 text-caption text-severity-error"
            data-testid="terminal-pane-statusbar-error"
            >{{ statusBarError }}</span
          >
          <template v-if="actionRow">
            <IconButton
              v-if="editorTitle"
              :label="`Open in ${editorTitle}`"
              :icon="IconCode"
              data-testid="terminal-pane-statusbar-open-editor"
              @click="runStatusBarAction(OpenSessionInEditor)"
            />
            <IconButton
              label="Show in Finder"
              :icon="IconFolderOpen"
              data-testid="terminal-pane-statusbar-reveal"
              @click="runStatusBarAction(RevealSession)"
            />
            <IconButton
              v-for="action in ops.actions.statusBarActions.value"
              :key="action.id"
              :label="action.label"
              :icon="launcherIconComponent(action.icon)"
              :data-testid="`terminal-statusbar-action-${action.id}`"
              @click="ops.actions.invoke(action, { slug: actionRow!.slug, windowId: '' })"
            />
            <span class="mx-1 h-3.5 w-px bg-strong" aria-hidden="true" />
          </template>
        </template>
      </template>
      <template #picker="{ panel }">
        <SessionPanelPicker
          :windows="pickerWindows"
          :agents="agents"
          :views="availableViews"
          :can-add-terminal="canAddTerminal"
          @window="(windowId) => placeWindowHere(windowId, panel.id)"
          @terminal="panels.newWindow(panels.row!, panel.id)"
          @agent="(agent) => panels.newAgentWindow(panels.row!, agent, panel.id)"
          @view="(view) => panels.apply((current) => placeTab(current, viewTab(view), panel.id))"
          @cancel="panels.apply((current) => closePanel(current, panel.id))"
        />
      </template>
      <template #tab="{ tab, shown, focused }">
        <DrivePage
          v-if="tab.kind === 'docs' && driveScope"
          embedded
          :scope="driveScope"
          @update:scope="(scope) => panels.apply((current) => withDocsScope(current, scope))"
          @setup="emit('setup')"
          @open-url="openLink"
        />
        <ChangesView
          v-else-if="tab.kind === 'diff' && changesSession"
          :keys-active="viewHasKeys(focused, shown)"
          :session-id="changesSession"
        />
        <TasksView v-else-if="tab.kind === 'tasks'" embedded :keys-active="viewHasKeys(focused, shown)" />
      </template>
      <template #floating="{ region }">
        <template v-for="session in pool.paneSessions" :key="session.slug">
          <div
            v-for="tab in session.tabs"
            v-show="session.entry === visible && panels.onScreen.has(tab.windowId)"
            :key="tab.uid"
            class="absolute flex min-h-0 min-w-0 flex-col"
            :style="region(panels.onScreen.get(tab.windowId) ?? '')"
            :data-panel="panels.onScreen.get(tab.windowId) ?? ''"
            @pointerdown.capture="
              panels.apply((current) => focusPanel(current, panels.onScreen.get(tab.windowId) ?? ''))
            "
          >
            <TerminalTab
              :tab="tab"
              :session="session.entry"
              :active="session.entry === visible && panels.onScreen.has(tab.windowId)"
            />
          </div>
        </template>
        <div
          v-if="visible && activeWindowOnScreen"
          class="pointer-events-none absolute flex flex-col"
          :style="region(panels.onScreen.get(activeWindowId) ?? '')"
        >
          <!-- Floated rather than in the column: shrinking the pane's box would change the
             window size this client votes to tmux, for every attached client. -->
          <div
            v-if="search.open && status !== 'ended'"
            class="pointer-events-auto absolute right-5 top-3 z-10 flex items-center gap-1 rounded-md border border-strong bg-raised/95 py-1 pl-2 pr-1 shadow-lg"
            data-testid="terminal-search"
          >
            <IconSearch class="size-3 shrink-0 text-text-4" />
            <input
              ref="searchInput"
              :value="search.query"
              type="text"
              placeholder="Find"
              spellcheck="false"
              class="w-44 bg-transparent text-caption text-text outline-none placeholder:text-text-4"
              data-testid="terminal-search-input"
              @input="visible?.setSearchQuery(($event.target as HTMLInputElement).value)"
              @keydown.enter.exact.prevent="visible?.findNext()"
              @keydown.enter.shift.prevent="visible?.findPrevious()"
              @keydown.esc.prevent="visible?.closeSearch()"
            />
            <span
              class="min-w-[54px] shrink-0 text-right font-mono text-micro text-text-4"
              data-testid="terminal-search-count"
              >{{ searchLabel() }}</span
            >
            <IconButton
              label="Previous match"
              :icon="IconChevronUp"
              size="sm"
              data-testid="terminal-search-prev"
              @click="visible?.findPrevious()"
            />
            <IconButton
              label="Next match"
              :icon="IconChevronDown"
              size="sm"
              data-testid="terminal-search-next"
              @click="visible?.findNext()"
            />
            <IconButton
              label="Close find"
              :icon="IconX"
              size="sm"
              data-testid="terminal-search-close"
              @click="visible?.closeSearch()"
            />
          </div>

          <Transition name="tail-pill">
            <button
              v-if="activeScrolledUp && status !== 'ended'"
              type="button"
              class="pointer-events-auto absolute bottom-3 right-5 z-10 flex cursor-pointer items-center gap-1.5 rounded-full border border-strong bg-raised/95 px-3 py-1.5 text-caption text-text-2 shadow-lg hover:text-text"
              data-testid="terminal-scroll-to-bottom"
              @click="visible?.scrollToBottom()"
            >
              <IconArrowDown class="size-3" />Scroll to bottom
            </button>
          </Transition>
        </div>
        <div
          v-if="visible && statusPanel"
          class="pointer-events-none absolute flex flex-col"
          :style="region(statusPanel)"
        >
          <div
            v-if="!tabs.length && status !== 'ended'"
            class="flex flex-1 items-center justify-center font-mono text-xs text-text-4"
          >
            Attaching…
          </div>

          <!-- Starting runs the session's agent command, so it is offered, never done on selection. -->
          <div
            v-if="notStarted"
            class="pointer-events-auto absolute inset-0 flex flex-col items-center justify-center gap-3 bg-app/95 px-10 text-center"
            data-testid="terminal-session-not-started"
          >
            <template v-if="attach.startError">
              <div
                class="flex size-11 items-center justify-center rounded-xl border border-severity-error-border bg-severity-error-tint"
              >
                <IconCircleAlert class="size-5 text-severity-error" />
              </div>
              <div class="text-title font-semibold" data-testid="terminal-start-error-title">
                {{
                  chatAttached
                    ? 'Chat failed to resume'
                    : scratchAttached
                      ? 'Terminal failed to start'
                      : 'Session failed to start'
                }}
              </div>
              <p class="max-w-[560px] text-xs leading-relaxed text-text-3">
                Hive could not start the configured terminal for
                <span class="font-mono text-text-2">{{ pool.activeSlug }}</span
                >.
              </p>
              <div
                class="w-full max-w-[680px] overflow-hidden rounded-xl border border-severity-error-border bg-sunken text-left shadow-lg"
                data-testid="terminal-start-error"
              >
                <div class="flex h-9 items-center border-b border-severity-error-border bg-severity-error-tint px-3">
                  <div class="flex items-center gap-1.5" aria-hidden="true">
                    <span class="size-2 rounded-full bg-severity-error" />
                    <span class="size-2 rounded-full bg-severity-warning" />
                    <span class="size-2 rounded-full bg-severity-success" />
                  </div>
                  <div class="flex min-w-0 flex-1 items-center justify-center gap-1.5 font-mono text-micro text-text-3">
                    <IconTerminal class="size-3" />
                    <span>Startup error</span>
                  </div>
                  <IconButton
                    :label="startErrorCopyLabel"
                    :icon="startErrorCopyStatus === 'success' ? IconCheck : IconCopy"
                    size="sm"
                    tone="danger"
                    data-testid="terminal-start-error-copy"
                    @click="copyStartError(attach.startError)"
                  />
                </div>
                <pre
                  class="hive-scroll max-h-[280px] min-h-24 select-text overflow-auto whitespace-pre-wrap break-words p-4 font-mono text-small leading-6 text-text-2"
                  data-testid="terminal-start-error-output"
                  >{{ attach.startError }}</pre>
              </div>
            </template>
            <template v-else>
              <IconTerminal class="size-6 text-text-4" />
              <div class="text-body font-semibold">
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
                No terminal is running for <span class="font-mono text-text-2">{{ pool.activeSlug }}</span> yet.
                Starting it opens this session's configured windows and runs its agent command.
              </p>
            </template>
            <div class="mt-1 flex items-center gap-2">
              <BaseButton
                size="sm"
                :busy="attach.starting === pool.activeSlug"
                data-testid="terminal-start-session"
                @click="attach.startSession(pool.activeSlug)"
              >
                <template #icon>
                  <IconRefreshCw v-if="attach.startError" class="size-3.5" />
                  <IconPlay v-else class="size-3.5" />
                </template>
                {{
                  attach.starting === pool.activeSlug
                    ? chatAttached
                      ? 'Resuming…'
                      : 'Starting…'
                    : attach.startError
                      ? 'Try again'
                      : chatAttached
                        ? 'Resume chat'
                        : scratchAttached
                          ? 'Start terminal'
                          : 'Start session'
                }}
              </BaseButton>
              <BaseButton
                variant="secondary"
                size="sm"
                data-testid="terminal-close-session"
                @click="attach.closeSession"
                >Close</BaseButton
              >
            </div>
          </div>

          <div
            v-else-if="status === 'ended'"
            class="pointer-events-auto absolute inset-0 flex flex-col items-center justify-center gap-3 bg-app/95 px-10 text-center"
            data-testid="terminal-session-ended"
          >
            <div class="text-body font-semibold" data-testid="terminal-session-ended-title">{{ endedTitle }}</div>
            <div class="w-full max-w-[680px] overflow-hidden rounded-xl border border-strong bg-sunken text-left">
              <div class="flex items-center justify-between border-b border-strong px-3 py-2">
                <span class="font-mono text-micro text-text-3">Terminal report</span>
                <IconButton
                  :label="startErrorCopyLabel"
                  :icon="startErrorCopyStatus === 'success' ? IconCheck : IconCopy"
                  size="sm"
                  data-testid="terminal-ended-copy"
                  @click="copyStartError(sessionError)"
                />
              </div>
              <pre
                class="hive-scroll max-h-[280px] select-text overflow-auto whitespace-pre-wrap break-words p-4 font-mono text-small text-text-2"
                data-testid="terminal-session-ended-reason"
                >{{ sessionError }}</pre>
            </div>
            <div class="mt-1 flex items-center gap-2">
              <BaseButton
                v-if="canRestart"
                size="sm"
                :disabled="attach.starting === pool.activeSlug"
                data-testid="terminal-restart"
                @click="attach.startSession(pool.activeSlug)"
              >
                <template #icon><IconRefreshCw class="size-3" /></template>
                {{ attach.starting === pool.activeSlug ? 'Starting…' : 'Restart' }}
              </BaseButton>
              <BaseButton v-else size="sm" data-testid="terminal-reconnect" @click="visible?.reconnect()">
                <template #icon><IconRefreshCw class="size-3" /></template>Reconnect
              </BaseButton>
              <BaseButton
                variant="secondary"
                size="sm"
                data-testid="terminal-close-session"
                @click="attach.closeSession"
              >
                Close session
              </BaseButton>
            </div>
          </div>
        </div>
      </template>
    </PanelLayoutHost>

    <!-- pb-0.5: the fonts sit lowercase letters about 1.4px below the middle of their box. -->
    <footer
      v-if="footerRow"
      class="flex h-7 shrink-0 items-center gap-1 overflow-hidden border-t border-border bg-raised px-1.5 pb-0.5"
      aria-label="Session status"
      data-testid="terminal-pane-statusbar"
    >
      <div class="flex min-w-0 flex-1 items-center overflow-hidden">
        <BaseBadge
          v-if="statusBarHost"
          class="mx-1 h-[18px] shrink-0 px-1.5 font-mono text-micro font-medium"
          :class="statusBarHost.hue.badge"
          :title="statusBarHost.ssh ? `${statusBarHost.name} · ${statusBarHost.ssh}` : statusBarHost.name"
          data-testid="session-status-host"
          ><IconServer class="size-3" aria-hidden="true" />{{ statusBarHost.name }}</BaseBadge
        >
        <SessionStatusChips
          v-if="statusBarRow"
          :git="sessionGit"
          :pull-request="sessionPullRequest"
          :pull-request-error="sessionPullRequestError"
          :can-open-changes="statusBarLocal"
          @refresh-pull-request="refreshSessionStatus({ refreshPullRequest: true })"
          @open-changes="openDiff"
        />
        <SessionTagsStatus
          v-if="statusBarRow"
          :tags="statusBarRow.tags"
          @edit="sessions.requestEdit(statusBarRow!, 'tags')"
        />
      </div>
    </footer>
  </div>
</template>
