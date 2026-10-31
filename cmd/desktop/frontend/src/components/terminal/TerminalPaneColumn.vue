<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, ref, toRef, watch } from 'vue'
import IconArrowDown from '~icons/lucide/arrow-down'
import IconCheck from '~icons/lucide/check'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronUp from '~icons/lucide/chevron-up'
import IconCircleAlert from '~icons/lucide/circle-alert'
import IconCopy from '~icons/lucide/copy'
import IconCode from '~icons/lucide/code'
import IconColumns2 from '~icons/lucide/columns-2'
import IconFolderOpen from '~icons/lucide/folder-open'
import IconInfo from '~icons/lucide/info'
import IconRotateCw from '~icons/lucide/rotate-cw'
import IconRows2 from '~icons/lucide/rows-2'
import IconServer from '~icons/lucide/server'
import IconPlay from '~icons/lucide/play'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconSearch from '~icons/lucide/search'
import IconTerminal from '~icons/lucide/terminal'
import IconX from '~icons/lucide/x'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import ChangesLoading from '../git/ChangesLoading.vue'
import DrivePage from '../drive/DrivePage.vue'
import TasksView from '../tasks/TasksView.vue'
import SessionPanePicker from './SessionPanePicker.vue'
import SessionTabRow from './SessionTabRow.vue'
import SessionTagsStatus from '../SessionTagsStatus.vue'
import SessionStatusChips from '../SessionStatusChips.vue'
import TerminalTab from '../TerminalTab.vue'
import { useClipboard } from '@hive/ui/composables/useClipboard'
import { useEditorSettings } from '../../composables/useEditorSettings'
import { useSessionStatus } from '../../composables/useSessionStatus'
import { openLink } from '../../composables/useXtermPane'
import { activityIndicator } from '../../lib/agentActivity'
import type { AgentIconID } from '../../lib/agentIcon'
import type { DriveScope } from '../../lib/drive'
import { launcherIconComponent } from '../../lib/launcherIcons'
import {
  addViewQuietly,
  closePane,
  closeTab,
  focusPane,
  offscreenWindows,
  openView,
  paneBoxes,
  paneById,
  panesOf,
  placeView,
  placeWindow,
  resizeSplit,
  selectTab,
  splitBoxes,
  splitPane,
  terminalPane,
  turnSplit,
  type LayoutBox,
  type SessionPane,
  type SessionSplit,
  type SessionTab,
} from '../../lib/sessionLayout'
import { activePaneOf } from '../../lib/terminalLayout'
import { terminalTreeFocused } from '../../lib/terminalTree'
import { useDriveActivity } from '../../stores/useDriveActivity'
import { useTasks } from '../../stores/useTasks'
import { useTerminalStatusBar } from '../../stores/useTerminalStatusBar'
import {
  OpenSessionInEditor,
  RevealSession,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import { useTerminalModeContext } from './terminalModeContext'
import { useSessionPaneDrag } from './useSessionPaneDrag'
import { useTerminalDrive, type TerminalDriveTarget } from './useTerminalDrive'

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
  drive: TerminalDriveTarget | null
  /** An app overlay has the keyboard, so a view under it must not take its keys. */
  overlayOpen?: boolean
}>()
const emit = defineEmits<{
  'session-repo-key': [repoKey: string]
  setup: []
}>()

const { pool, tree, attach, ops, sessions, panes } = useTerminalModeContext()
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
const statusBarRow = computed(() => (showStatusBar.value ? actionRow.value : null))
const footerRow = computed(() => {
  if (!showStatusBar.value) return null
  return statusBarRow.value ?? (panes.row?.server ? panes.row : null)
})
const agentStatus = computed(() => {
  const row = footerRow.value
  const status = row ? tree.windowStatus(row, activeWindowId.value) : undefined
  return status ? activityIndicator(status.status, status.tool) : null
})
const statusBarSessionId = computed(() => statusBarRow.value?.id ?? '')
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
// only from this bar's own button.
const sessionRepoKey = computed(() => {
  const git = sessionGit.value
  return git?.resolved && git.owner && git.repo ? `${git.owner}/${git.repo}` : ''
})
watch(sessionRepoKey, (key) => emit('session-repo-key', key), { immediate: true })

const layout = computed(() => panes.layout)
const panesList = computed(() => panesOf(layout.value))
const boxes = computed(() => paneBoxes(layout.value))
const splits = computed(() => splitBoxes(layout.value))
const panesHost = ref<HTMLElement | null>(null)
const { tabDrop, startTabDrag, startResize, stepResize } = useSessionPaneDrag(panes, panesHost)
const TAB_ROW_PX = 30
const BORDER_PX = 1

const percent = (share: number): string => `${share * 100}%`

function paneStyle(paneId: string): Record<string, string> {
  const box = boxes.value.get(paneId)
  if (!box) return {}
  return { left: percent(box.x), top: percent(box.y), width: percent(box.width), height: percent(box.height) }
}

// Terminals stay in one parent, because a Terminal binds to one element for its
// lifetime, so a pane places them by position under its tab row.
function paneRegion(paneId: string): Record<string, string> {
  const box = boxes.value.get(paneId)
  if (!box) return { inset: `${TAB_ROW_PX}px 0 0 0` }
  const left = box.x > 0 ? BORDER_PX : 0
  const top = TAB_ROW_PX + (box.y > 0 ? BORDER_PX : 0)
  return {
    left: `calc(${percent(box.x)} + ${left}px)`,
    top: `calc(${percent(box.y)} + ${top}px)`,
    width: `calc(${percent(box.width)} - ${left}px)`,
    height: `calc(${percent(box.height)} - ${top}px)`,
  }
}

function dividerStyle(split: SessionSplit, box: LayoutBox): Record<string, string> {
  if (split.axis === 'row') {
    return {
      left: `calc(${percent(box.x + box.width * split.ratio)} - 3px)`,
      top: percent(box.y),
      height: percent(box.height),
      width: '6px',
    }
  }
  return {
    top: `calc(${percent(box.y + box.height * split.ratio)} - 3px)`,
    left: percent(box.x),
    width: percent(box.width),
    height: '6px',
  }
}

// A pane under a stacked boundary: its tab row is a grab area for that boundary.
function boundaryAbove(paneId: string): { split: SessionSplit; box: LayoutBox } | undefined {
  const pane = boxes.value.get(paneId)
  if (!pane || pane.y === 0) return undefined
  return splits.value.find(({ split, box }) => {
    if (split.axis !== 'column') return false
    const at = box.y + box.height * split.ratio
    return Math.abs(at - pane.y) < 1e-6 && pane.x >= box.x - 1e-6 && pane.x + pane.width <= box.x + box.width + 1e-6
  })
}

function startRowResize(event: PointerEvent, paneId: string): void {
  const boundary = boundaryAbove(paneId)
  if (boundary) startResize(event, boundary.split, boundary.box)
}

// The session's own actions sit once, at the top right of the whole column.
const actionsPane = computed(() => {
  for (const pane of panesList.value) {
    const box = boxes.value.get(pane.id)
    if (box && box.y === 0 && Math.abs(box.x + box.width - 1) < 1e-6) return pane.id
  }
  return ''
})

const activeWindowRegion = computed(() => paneRegion(panes.onScreen.get(activeWindowId.value) ?? ''))
const activeWindowOnScreen = computed(() => panes.onScreen.has(activeWindowId.value))
const statusPane = computed(() => terminalPane(layout.value))

function windowName(windowId: string): string {
  const tab = tabs.value.find((candidate) => candidate.windowId === windowId)
  return tab?.name || windowId
}

function tabLabel(tab: SessionTab): string {
  switch (tab.kind) {
    case 'window':
      return windowName(tab.windowId)
    case 'docs':
      return 'Docs'
    case 'diff':
      return 'Diff'
    case 'tasks':
      return 'Tasks'
  }
}

const driveActivity = useDriveActivity()

function needsApproval(windowId: string): boolean {
  const row = panes.row
  return !!row && tree.windowStatus(row, windowId)?.status === 'approval'
}

function tabAgent(tab: SessionTab): AgentIconID | undefined {
  const row = panes.row
  return row && tab.kind === 'window' ? tree.windowAgent(row, tab.windowId, windowName(tab.windowId)) : undefined
}

function tabAttention(tab: SessionTab): boolean {
  if (tab.kind === 'window') return needsApproval(tab.windowId)
  return tab.kind === 'docs' && !!shownDrive.value && driveActivity.isUnseen(shownDrive.value.session)
}

function viewTabs(tabList: readonly SessionTab[]): SessionTab[] {
  return tabList.filter((tab) => tab.kind !== 'window')
}

// A running window has a tab and a tree row, so closing its tab closes the window,
// through the same confirmation as the tree.
function closePaneTab(paneId: string, tabId: string): void {
  const tab = paneById(layout.value, paneId)?.tabs.find((candidate) => candidate.id === tabId)
  if (tab?.kind === 'window') void ops.requestCloseWindow(pool.visibleSlug, tab.windowId, windowName(tab.windowId))
  else panes.apply((current) => closeTab(current, paneId, tabId))
}

function placeWindowHere(windowId: string, paneId: string): void {
  panes.apply((current) => placeWindow(current, windowId, paneId))
  panes.syncWindowOrder(windowId)
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
  const row = panes.row
  return row && tree.isHiveSession(row) && !tree.isServerSession(row) ? row.id : ''
})
const availableViews = computed(() => (panes.row ? panes.viewsFor(panes.row) : []))
const canAddTerminal = computed(() => !!panes.row && !tree.isChat(panes.row))
const agents = computed(() => (changesSession.value && !ops.agentWindowBusy ? ops.agentProfiles : []))

const pickerWindows = computed(() =>
  offscreenWindows(
    layout.value,
    tabs.value.map((tab) => tab.windowId),
  ).map((windowId) => ({ windowId, name: windowName(windowId) })),
)

function openDiff(): void {
  panes.apply((current) => openView(current, 'diff'))
}

function viewShown(pane: SessionPane, tabId: string): boolean {
  return !pane.picking && pane.activeTab === tabId
}

function viewHasKeys(pane: SessionPane, tabId: string): boolean {
  return (
    props.active &&
    !props.overlayOpen &&
    !terminalTreeFocused.value &&
    layout.value.focusedPane === pane.id &&
    viewShown(pane, tabId)
  )
}

const { setRepoKey } = useTasks()
const tasksTabOpen = computed(() => panesList.value.some((pane) => pane.tabs.some((tab) => tab.kind === 'tasks')))
watch(
  [tasksTabOpen, sessionRepoKey],
  ([open, repoKey]) => {
    if (open && repoKey) setRepoKey(repoKey)
  },
  { immediate: true },
)

const driveScope = computed<DriveScope | null>(() => {
  const own = layout.value.docScope
  if (own) return own
  return shownDrive.value ? { ...shownDrive.value, path: null } : null
})

function scopeAt(path: string | null): DriveScope | null {
  const base = layout.value.docScope ?? (props.drive ? { ...props.drive, path: null } : null)
  return base && path ? { ...base, path } : base
}

useTerminalDrive(toRef(props, 'drive'), () => props.active, {
  // Tabs are keyed by the row's id, so a request has to wait for the row.
  ready: () => !!panes.row && pool.activeSlug === pool.visibleSlug,
  open: (path) => panes.apply((current) => ({ ...openView(current, 'docs'), docScope: scopeAt(path) })),
  arrive: (path) => panes.apply((current) => ({ ...addViewQuietly(current, 'docs'), docScope: scopeAt(path) })),
  close: () =>
    panes.apply((current) => {
      const pane = panesOf(current).find((candidate) => candidate.tabs.some((tab) => tab.kind === 'docs'))
      return pane ? closeTab(current, pane.id, 'docs') : current
    }),
})

// The full page knows nothing about authors. A Docs tab on screen is the user
// reading, and one behind another tab is not, so a write there still earns its dot.
const docsReader = computed(() => {
  const shownDocs = panesList.value.some((pane) => viewShown(pane, 'docs'))
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

    <!-- Hidden rather than unmounted without a session: a pooled session's terminals keep
         their elements, or each would lose its screen. -->
    <div
      ref="panesHost"
      class="relative min-h-0 min-w-0 flex-1"
      :class="visible ? 'block' : 'hidden'"
      data-testid="session-panes"
    >
      <section
        v-for="pane in panesList"
        :key="pane.id"
        class="absolute flex min-h-0 min-w-0 flex-col"
        :class="[
          (boxes.get(pane.id)?.x ?? 0) > 0 ? 'border-l border-strong' : '',
          (boxes.get(pane.id)?.y ?? 0) > 0 ? 'border-t border-strong' : '',
        ]"
        :style="paneStyle(pane.id)"
        :data-session-pane="pane.id"
        :aria-label="`Pane ${pane.id}`"
        :data-testid="`session-pane-${pane.id}`"
        @pointerdown.capture="panes.apply((current) => focusPane(current, pane.id))"
      >
        <SessionTabRow
          :pane="pane"
          :focused="layout.focusedPane === pane.id"
          :label="tabLabel"
          :attention="tabAttention"
          :agent="tabAgent"
          :row="panes.row"
          :drop-gap="tabDrop?.pane === pane.id ? tabDrop.gap : null"
          :resizable="!!boundaryAbove(pane.id)"
          @tab-drag="startTabDrag"
          @resize-start="(event) => startRowResize(event, pane.id)"
          @select="(tabId) => panes.apply((current) => selectTab(current, pane.id, tabId))"
          @close="(tabId) => closePaneTab(pane.id, tabId)"
        >
          <template #actions>
            <div class="ml-auto flex shrink-0 items-center gap-px pl-2">
              <template v-if="pane.id === actionsPane">
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
              <IconButton
                label="Split right"
                :icon="IconColumns2"
                :data-testid="`session-split-row-${pane.id}`"
                @click="panes.apply((current) => splitPane(current, pane.id, 'row'))"
              />
              <IconButton
                label="Split down"
                :icon="IconRows2"
                :data-testid="`session-split-column-${pane.id}`"
                @click="panes.apply((current) => splitPane(current, pane.id, 'column'))"
              />
              <IconButton
                v-if="panesList.length > 1"
                label="Close this pane, keeping its tabs"
                :icon="IconX"
                :data-testid="`session-pane-close-${pane.id}`"
                @click="panes.apply((current) => closePane(current, pane.id))"
              />
            </div>
          </template>
        </SessionTabRow>

        <div class="relative flex min-h-0 min-w-0 flex-1 flex-col">
          <SessionPanePicker
            v-if="pane.picking"
            :windows="pickerWindows"
            :agents="agents"
            :views="availableViews"
            :can-add-terminal="canAddTerminal"
            @window="(windowId) => placeWindowHere(windowId, pane.id)"
            @terminal="panes.newWindow(panes.row!, pane.id)"
            @agent="(agent) => panes.newAgentWindow(panes.row!, agent, pane.id)"
            @view="(view) => panes.apply((current) => placeView(current, view, pane.id))"
            @cancel="panes.apply((current) => closePane(current, pane.id))"
          />
          <!-- Mounted while open, so a tab switch keeps a diff's scroll and picks;
               a session switch re-keys them, so only the visible session pays. -->
          <div
            v-for="tab in viewTabs(pane.tabs)"
            v-show="viewShown(pane, tab.id)"
            :key="`${panes.key}:${tab.id}`"
            class="absolute inset-0 flex min-h-0 min-w-0 flex-col bg-app"
            :data-testid="`session-view-${tab.kind}`"
          >
            <DrivePage
              v-if="tab.kind === 'docs' && driveScope"
              embedded
              :scope="driveScope"
              @update:scope="(scope) => panes.apply((current) => ({ ...current, docScope: scope }))"
              @setup="emit('setup')"
              @open-url="openLink"
            />
            <ChangesView
              v-else-if="tab.kind === 'diff' && changesSession"
              embedded
              :keys-active="viewHasKeys(pane, tab.id)"
              :session-id="changesSession"
            />
            <TasksView v-else-if="tab.kind === 'tasks'" embedded :keys-active="viewHasKeys(pane, tab.id)" />
          </div>
        </div>
      </section>

      <template v-for="session in pool.paneSessions" :key="session.slug">
        <div
          v-for="tab in session.tabs"
          v-show="session.entry === visible && panes.onScreen.has(tab.windowId)"
          :key="tab.uid"
          class="absolute flex min-h-0 min-w-0 flex-col"
          :style="paneRegion(panes.onScreen.get(tab.windowId) ?? '')"
          :data-session-pane="panes.onScreen.get(tab.windowId) ?? ''"
          @pointerdown.capture="panes.apply((current) => focusPane(current, panes.onScreen.get(tab.windowId) ?? ''))"
        >
          <TerminalTab
            :tab="tab"
            :session="session.entry"
            :active="session.entry === visible && panes.onScreen.has(tab.windowId)"
          />
        </div>
      </template>
      <div
        v-for="{ split, box } in splits"
        :key="split.id"
        class="group/divider absolute z-20 touch-none"
        :class="split.axis === 'column' ? 'cursor-row-resize' : 'cursor-col-resize'"
        :style="dividerStyle(split, box)"
        role="separator"
        tabindex="0"
        :aria-orientation="split.axis === 'column' ? 'horizontal' : 'vertical'"
        :aria-valuenow="Math.round(split.ratio * 100)"
        aria-label="Resize the panes"
        :data-testid="`session-split-divider-${split.id}`"
        @pointerdown="startResize($event, split, box)"
        @dblclick="panes.apply((current) => resizeSplit(current, split.id, 0.5))"
        @keydown.left.prevent="stepResize(split, -1)"
        @keydown.up.prevent="stepResize(split, -1)"
        @keydown.right.prevent="stepResize(split, 1)"
        @keydown.down.prevent="stepResize(split, 1)"
      >
        <button
          type="button"
          class="absolute flex size-5 cursor-pointer items-center justify-center rounded-full border border-strong bg-raised text-text-3 opacity-0 hover:text-text focus-visible:opacity-100 group-hover/divider:opacity-100"
          :class="split.axis === 'column' ? 'left-3 top-1/2 -translate-y-1/2' : 'left-1/2 top-9 -translate-x-1/2'"
          aria-label="Turn this split"
          title="Turn this split"
          :data-testid="`session-split-turn-${split.id}`"
          @pointerdown.stop
          @click="panes.apply((current) => turnSplit(current, split.id))"
        >
          <IconRotateCw class="size-3" aria-hidden="true" />
        </button>
      </div>
      <div
        v-if="visible && activeWindowOnScreen"
        class="pointer-events-none absolute flex flex-col"
        :style="activeWindowRegion"
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
        v-if="visible && statusPane"
        class="pointer-events-none absolute flex flex-col"
        :style="paneRegion(statusPane)"
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
              {{ chatAttached ? 'Chat not running' : scratchAttached ? 'Terminal not started' : 'Session not started' }}
            </div>
            <p v-if="chatAttached" class="max-w-[420px] text-xs leading-relaxed text-text-3">
              This chat is stopped. Resuming it launches the agent again in its workspace, picking the conversation back
              up where the agent itself can.
            </p>
            <p v-else-if="scratchAttached" class="max-w-[420px] text-xs leading-relaxed text-text-3">
              The scratch terminal is not running. Starting it opens a shell in your home directory, and every tab you
              add opens there too.
            </p>
            <p v-else class="max-w-[420px] text-xs leading-relaxed text-text-3">
              No terminal is running for <span class="font-mono text-text-2">{{ pool.activeSlug }}</span> yet. Starting
              it opens this session's configured windows and runs its agent command.
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
            <BaseButton variant="secondary" size="sm" data-testid="terminal-close-session" @click="attach.closeSession"
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
            <BaseButton variant="secondary" size="sm" data-testid="terminal-close-session" @click="attach.closeSession">
              Close session
            </BaseButton>
          </div>
        </div>
      </div>
    </div>

    <footer
      v-if="footerRow"
      class="flex h-6 shrink-0 items-center gap-1 overflow-hidden border-t border-border bg-raised px-1.5"
      aria-label="Session status"
      data-testid="terminal-pane-statusbar"
    >
      <div class="flex min-w-0 flex-1 items-center overflow-hidden">
        <SessionStatusChips
          v-if="statusBarRow"
          :git="sessionGit"
          :pull-request="sessionPullRequest"
          :pull-request-error="sessionPullRequestError"
          @refresh-pull-request="refreshSessionStatus({ refreshPullRequest: true })"
          @open-changes="openDiff"
        />
        <SessionTagsStatus
          v-if="statusBarRow"
          :tags="statusBarRow.tags"
          @edit="sessions.requestEdit(statusBarRow!, 'tags')"
        />
      </div>
      <div class="flex shrink-0 items-center gap-1 pl-2 text-caption text-text-3">
        <span v-if="agentStatus" class="flex items-center gap-1" data-testid="session-status-agent">
          <component
            :is="agentStatus.icon"
            class="size-3"
            :class="[agentStatus.color, agentStatus.animated ? 'animate-spin' : '']"
            aria-hidden="true"
          />{{ agentStatus.label }}
        </span>
        <span v-if="agentStatus" class="mx-1 h-3 w-px bg-border" aria-hidden="true" />
        <span class="flex items-center gap-1" data-testid="session-status-host">
          <IconServer v-if="footerRow.server" class="size-3 text-text-4" aria-hidden="true" />
          {{ footerRow.server || 'Local' }}
        </span>
      </div>
    </footer>
  </div>
</template>
