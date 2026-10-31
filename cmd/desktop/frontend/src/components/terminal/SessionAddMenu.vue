<script setup lang="ts">
import { computed } from 'vue'
import IconFileDiff from '~icons/lucide/file-diff'
import IconFileText from '~icons/lucide/file-text'
import IconListTodo from '~icons/lucide/list-todo'
import NewWindowMenu from '../NewWindowMenu.vue'
import type { TerminalSessionRow } from '../../stores/useTerminalSessions'
import type { SessionViewKind } from '../../lib/sessionLayout'
import { useTerminalModeContext } from './terminalModeContext'

// The one "+" menu, for a session row in the tree and for a tab row alike.
const props = withDefaults(
  defineProps<{
    row: TerminalSessionRow
    /** The pane a choice lands in; the session's focused pane without one. */
    pane?: number
    flip?: boolean
    ignore: (HTMLElement | null)[]
    anchor?: HTMLElement | null
  }>(),
  { pane: undefined, flip: false, anchor: undefined },
)
const emit = defineEmits<{ close: [] }>()

const { tree, ops, panes } = useTerminalModeContext()

const VIEWS = {
  docs: { label: 'Docs', icon: IconFileText },
  diff: { label: 'Diff', icon: IconFileDiff },
  tasks: { label: 'Tasks', icon: IconListTodo },
}

const views = computed(() => panes.viewsFor(props.row).map((kind: SessionViewKind) => ({ kind, ...VIEWS[kind] })))
</script>

<template>
  <NewWindowMenu
    :running="tree.rowRunning(row)"
    :flip="flip"
    :ignore="ignore"
    :anchor="anchor"
    :agents="row.server ? [] : ops.agentProfiles"
    :default-agent="ops.defaultAgentProfile"
    :loading="!row.server && ops.agentProfilesLoading"
    :failed="!row.server && ops.agentProfilesFailed"
    :views="views"
    @close="emit('close')"
    @terminal="panes.newWindow(row, pane)"
    @agent="panes.newAgentWindow(row, $event, pane)"
    @view="panes.openViewIn(row, $event, pane)"
    @retry="ops.loadAgentProfiles"
  />
</template>
