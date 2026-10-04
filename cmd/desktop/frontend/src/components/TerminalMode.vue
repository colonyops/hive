<script setup lang="ts">
import { onMounted, toRef } from 'vue'
import { useRouter } from 'vue-router'
import IconTerminal from '~icons/lucide/terminal'
import ActionInputsDialog from './ActionInputsDialog.vue'
import AgentCanvasPane from './AgentCanvasPane.vue'
import SessionDetailDialog from './SessionDetailDialog.vue'
import ConfirmationHost from './ui/ConfirmationHost.vue'
import RenameDialog from './ui/RenameDialog.vue'
import TerminalPaneColumn from './terminal/TerminalPaneColumn.vue'
import TerminalSidebar from './terminal/TerminalSidebar.vue'
import { provideTerminalMode } from './terminal/terminalModeContext'
import { useSessionTreeView } from './terminal/useSessionTreeView'
import { useTerminalCanvas, type TerminalCanvasTarget } from './terminal/useTerminalCanvas'
import { useTerminalAttach } from './terminal/useTerminalAttach'
import { useTerminalCommands } from './terminal/useTerminalCommands'
import { useTerminalPool } from './terminal/useTerminalPool'
import { useTerminalSessionOps } from './terminal/useTerminalSessionOps'
import { useTerminalTree } from './terminal/useTerminalTree'
import { useTreeKeyboardNav } from './terminal/useTreeKeyboardNav'
import { useWindowRename } from './terminal/useWindowRename'
import { useNewSession } from '../composables/useNewSession'
import { useSessionActions } from '../composables/useSessionActions'
import { openLink } from '../composables/useXtermPane'
import type { CanvasScope } from '../lib/agentCanvas'
import { useTerminalAvailability } from '../stores/useTerminalAvailability'
import { useTerminalPoolSize } from '../stores/useTerminalPoolSize'
import '@xterm/xterm/css/xterm.css'

// `active` is whether this mode is the surface on screen. The component is
// mounted once and hidden on a trip to the hub (App.vue), so it is the signal
// that replaces mount/unmount for anything that must not run off-screen.
const props = withDefaults(
  defineProps<{
    sidebarCollapsed?: boolean
    active?: boolean
    /** Whose canvases the attached session reads; null for a session with none. */
    canvas?: TerminalCanvasTarget | null
  }>(),
  { active: true, canvas: null },
)
const emit = defineEmits<{
  'open-tasks': []
  'session-repo-key': [repoKey: string]
  'open-canvas-page': [scope: CanvasScope]
}>()
const active = (): boolean => props.active

const { checking, available, reason } = useTerminalAvailability()
const { poolSize } = useTerminalPoolSize()

const pool = useTerminalPool({ size: poolSize, onEvicted: () => tree.sweepListings() })
const tree = useTerminalTree({ pool, active })
const view = useSessionTreeView(tree)
const rename = useWindowRename(pool.visible)
const attach = useTerminalAttach({ pool, tree, active, onSwitch: () => (rename.windowId.value = '') })
const sessions = useSessionActions({ onChanged: () => void tree.reloadSessions() })
const ops = useTerminalSessionOps({ pool, tree, attach, confirmation: sessions.confirmation, active })
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
const context = { pool, tree, view, nav, attach, ops, rename, sessions }
provideTerminalMode(context)
useTerminalCommands(context, active)

const {
  pane: canvasPane,
  canvasName,
  client: canvasClient,
  syncCanvasQuery,
} = useTerminalCanvas(toRef(props, 'canvas'), active)

// Hive does not configure a Code session's agent, so the empty pane leads to
// the page that says how.
const router = useRouter()
function openMcpSettings(): void {
  void router.push({ name: 'application-settings', params: { section: 'mcp' } })
}

const { prefetch: prefetchNewSession } = useNewSession()
onMounted(prefetchNewSession)

const { pendingInputs: actionInputs, inputsBusy, inputsError, cancelInputs, submitInputs } = ops.actions
const { confirmation, detail, closeDetail, renaming, renameBusy, renameError, cancelRename, submitRename } = sessions
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
      <TerminalSidebar v-if="!sidebarCollapsed" />
      <TerminalPaneColumn
        :active="props.active"
        @open-tasks="emit('open-tasks')"
        @session-repo-key="emit('session-repo-key', $event)"
      />
      <!-- A sibling of the pane column, as in Chats: opening it must not
           re-key a terminal host, and the size vote absorbs the width change. -->
      <AgentCanvasPane
        v-if="canvasPane"
        :session="canvasPane.session"
        :workspace="canvasPane.workspace"
        :name="canvasName"
        :client="canvasClient"
        :agent-setup="typeof canvasPane.session === 'string'"
        @setup="openMcpSettings"
        @close="syncCanvasQuery(false)"
        @open-url="openLink"
        @pick="(name) => syncCanvasQuery(true, name)"
        @open-page="(name) => emit('open-canvas-page', { ...canvasPane!, name })"
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
    <RenameDialog
      v-if="renaming"
      title="Rename session"
      label="Session name"
      hint="Its terminal session is renamed too, so an open terminal reconnects."
      testid="session-rename"
      :name="renaming.name"
      :busy="renameBusy"
      :error="renameError"
      @close="cancelRename"
      @save="submitRename"
    />
    <ConfirmationHost :confirmation="confirmation" testid="session-confirmation" />
  </div>
</template>
