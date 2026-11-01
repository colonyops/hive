<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useTimeoutFn, whenever } from '@vueuse/core'
import { useRouter } from 'vue-router'
import IconTerminal from '~icons/lucide/terminal'
import ActionInputsDialog from './ActionInputsDialog.vue'
import ModeSkeleton from './ModeSkeleton.vue'
import SessionDetailDialog from './SessionDetailDialog.vue'
import SessionEditDialog from './SessionEditDialog.vue'
import ConfirmationHost from '@hive/ui/components/ui/ConfirmationHost.vue'
import TerminalPaneColumn from './terminal/TerminalPaneColumn.vue'
import TerminalSidebar from './terminal/TerminalSidebar.vue'
import { provideTerminalMode } from './terminal/terminalModeContext'
import { useCodePanels } from './terminal/useCodePanels'
import { useSessionTreeView } from './terminal/useSessionTreeView'
import { useSessionViewMenus } from './terminal/useSessionViewMenus'
import { useTerminalAttach } from './terminal/useTerminalAttach'
import { useTerminalCommands } from './terminal/useTerminalCommands'
import type { TerminalDriveTarget } from './terminal/useTerminalDrive'
import { useTerminalPool } from './terminal/useTerminalPool'
import { useTerminalSessionOps } from './terminal/useTerminalSessionOps'
import { useTerminalTree } from './terminal/useTerminalTree'
import { useTreeKeyboardNav } from './terminal/useTreeKeyboardNav'
import { useWindowRename } from './terminal/useWindowRename'
import { useNewSession } from '../composables/useNewSession'
import { usePaintedActive } from '../composables/usePaintedActive'
import { useSessionActions } from '../composables/useSessionActions'
import { revealWindow } from '../lib/windowTabs'
import { TERMINAL_SIDEBAR } from '../lib/modeSidebars'
import { useCodeViews } from '../stores/useCodeViews'
import { usePanelLayouts } from '../stores/usePanelLayouts'
import { useTerminalAvailability } from '../stores/useTerminalAvailability'
import { useTerminalPoolSize } from '../stores/useTerminalPoolSize'
import { useTerminalSessions } from '../stores/useTerminalSessions'
import '@xterm/xterm/css/xterm.css'

// `active` is whether this mode is the surface on screen. The component is
// mounted once and hidden on a trip to the hub (App.vue), so it is the signal
// that replaces mount/unmount for anything that must not run off-screen.
const props = withDefaults(
  defineProps<{
    sidebarCollapsed?: boolean
    active?: boolean
    /** The space the attached session reads; null for a session with none. */
    drive?: TerminalDriveTarget | null
    overlayOpen?: boolean
  }>(),
  { active: true, drive: null },
)
const emit = defineEmits<{
  'session-repo-key': [repoKey: string]
}>()
const PREPARE_LIMIT_MS = 3000
const active = (): boolean => props.active
const { checking, available, reason, client } = useTerminalAvailability()
const painted = usePaintedActive(active, { preloaded: client.value !== null })
const paintedActive = (): boolean => painted.value

const { poolSize } = useTerminalPoolSize()

const pool = useTerminalPool({ size: poolSize, onEvicted: () => tree.sweepListings() })
// Mounted hidden by the preload: the tree reads statuses and windows once, so the
// first entry reveals a finished tree as every later entry does. Only the tree:
// the attach's probe restores a session by route, which must wait for an entry.
// eslint-disable-next-line vue/no-setup-props-reactivity-loss -- how this mount began, read once
const preparing = ref(!props.active)
const tree = useTerminalTree({ pool, active: () => paintedActive() || preparing.value })
const codeViews = useCodeViews()
const view = useSessionTreeView(tree, {
  filter: () => codeViews.draft.value,
  filterReady: () => codeViews.hydrated.value,
})
const stopPreparing = (): void => {
  preparing.value = false
}
whenever(view.ready, stopPreparing, { immediate: true })
// A tree that cannot get ready, with tmux missing, must not keep polling off-screen.
useTimeoutFn(stopPreparing, PREPARE_LIMIT_MS)
onMounted(() => {
  if (preparing.value) tree.sweepListings()
})
// A pool reveal draws its terminal; coming back to the same session has to as well,
// or output taken while hidden waits two frames for the observer.
watch(
  () => props.active,
  (shown) => {
    if (shown) panels.renderOnScreen()
  },
  { flush: 'post' },
)
const viewMenus = useSessionViewMenus(tree)
const rename = useWindowRename(pool.visible)
const panelLayouts = usePanelLayouts()
const attach = useTerminalAttach({
  pool,
  tree,
  active,
  paintedActive,
  onSwitch: () => (rename.windowId.value = ''),
  onSessionClosed: (row) => panelLayouts.forget(row.id),
  onWindowChosen: (row, windowId) => panelLayouts.apply(row.id, (layout) => revealWindow(layout, windowId)),
})
const sessions = useSessionActions({ onChanged: () => void tree.reloadSessions() })
const ops = useTerminalSessionOps({ pool, tree, attach, confirmation: sessions.confirmation, paintedActive })
const panels = useCodePanels({ pool, tree, ops })
const nav = useTreeKeyboardNav({
  groups: view.groups,
  expanded: view.expansion.expanded,
  rows: tree.attachable,
  attachedRow: tree.attachedRow,
  activeSlug: pool.activeSlug,
  windowRowsFor: tree.windowRowsFor,
  rowRunning: tree.rowRunning,
  renaming: () => !!rename.windowId.value,
  selectSession: attach.selectSession,
  startSession: attach.startSession,
  goToWindow: attach.goToWindow,
})
const context = { pool, tree, view, viewMenus, nav, attach, ops, rename, sessions, panels }
provideTerminalMode(context)
useTerminalCommands(context, active)

// Hive does not configure a Code session's agent, so the empty pane leads to
// the page that says how.
const router = useRouter()
function openMcpSettings(): void {
  void router.push({ name: 'application-settings', params: { section: 'mcp' } })
}

const { prefetch: prefetchNewSession } = useNewSession()
onMounted(prefetchNewSession)

const { pendingInputs: actionInputs, inputsBusy, inputsError, cancelInputs, submitInputs } = ops.actions
const { confirmation, detail, closeDetail, editing, editFocus, editBusy, editError, cancelEdit, submitEdit } = sessions

// Every tag already in use is offered, so one spelling spreads instead of
// near-duplicates.
const { sessions: sessionRows } = useTerminalSessions()
const knownTags = computed(() => {
  const seen = new Map<string, string>()
  for (const row of sessionRows.value) {
    for (const tag of row.tags ?? []) if (!seen.has(tag.toLowerCase())) seen.set(tag.toLowerCase(), tag)
  }
  return [...seen.values()].sort((a, b) => a.localeCompare(b))
})
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col bg-app" data-testid="terminal-mode">
    <ModeSkeleton v-if="checking" :sidebar="TERMINAL_SIDEBAR" />

    <div
      v-else-if="!available"
      class="flex flex-1 flex-col items-center justify-center gap-3 px-10 text-center"
      data-testid="terminal-unavailable"
    >
      <IconTerminal class="size-6 text-text-4" />
      <div class="text-body font-semibold">Terminal unavailable</div>
      <p class="max-w-[420px] text-xs leading-relaxed text-text-3" data-testid="terminal-unavailable-reason">
        {{ reason || 'The terminal is not available in this build.' }}
      </p>
      <button
        type="button"
        class="mt-1 cursor-pointer rounded border border-strong px-3 py-1.5 text-xs text-text-2 hover:text-text"
        data-testid="terminal-retry"
        @click="attach.probe"
      >
        Try again
      </button>
    </div>

    <div v-else class="flex min-h-0 min-w-0 flex-1">
      <TerminalSidebar v-if="!sidebarCollapsed" :active="props.active" />
      <TerminalPaneColumn
        :painted-active="painted"
        :active="props.active"
        :drive="props.drive"
        :overlay-open="props.overlayOpen"
        @session-repo-key="emit('session-repo-key', $event)"
        @setup="openMcpSettings"
      />
    </div>

    <ActionInputsDialog
      v-if="actionInputs"
      :action-label="actionInputs.action.label"
      :inputs="actionInputs.action.inputs ?? []"
      :busy="inputsBusy"
      :error="inputsError"
      @close="cancelInputs"
      @submit="submitInputs"
    />
    <SessionDetailDialog v-if="detail" :detail="detail" @close="closeDetail" />
    <SessionEditDialog
      v-if="editing"
      :name="editing.name"
      :tags="editing.tags"
      :focus="editFocus"
      :suggestions="knownTags"
      :busy="editBusy"
      :error="editError"
      @close="cancelEdit"
      @save="submitEdit"
    />
    <ConfirmationHost :confirmation="confirmation" testid="session-confirmation" />
  </div>
</template>
