<script setup lang="ts">
import { computed } from 'vue'
import NewWindowMenu from '../NewWindowMenu.vue'
import type { TerminalSessionRow } from '../../stores/useTerminalSessions'
import { sessionView } from './panelViews'
import { useTerminalModeContext } from './terminalModeContext'

// The one "+" menu, for a session row in the tree and for a tab row alike.
const props = withDefaults(
  defineProps<{
    row: TerminalSessionRow
    /** The panel a choice lands in; the session's focused panel without one. */
    panel?: string
    flip?: boolean
    ignore: (HTMLElement | null)[]
    anchor?: HTMLElement | null
  }>(),
  { panel: undefined, flip: false, anchor: undefined },
)
const emit = defineEmits<{ close: [] }>()

const { tree, ops, panels } = useTerminalModeContext()

const views = computed(() =>
  panels.viewsFor(props.row).map((kind) => {
    const { label, icon } = sessionView(kind)
    return { kind, label, icon }
  }),
)
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
    @terminal="panels.newWindow(row, panel)"
    @agent="panels.newAgentWindow(row, $event, panel)"
    @view="panels.openViewIn(row, $event, panel)"
    @retry="ops.loadAgentProfiles"
  />
</template>
