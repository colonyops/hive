<script setup lang="ts">
import { computed } from 'vue'
import NewWindowMenu from '../NewWindowMenu.vue'
import { useApplets } from '../../stores/useApplets'
import { useNativeTerminals } from '../../stores/useNativeTerminals'
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
const applets = useApplets()
const nativeTerminals = useNativeTerminals()
void nativeTerminals.probe()

// Agents open in a session's own checkout, which a server session, the scratch
// terminal, and a pinned chat do not have here.
const offersAgents = computed(() => tree.isHiveSession(props.row) && !props.row.server)

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
    :agents="offersAgents ? ops.agentProfiles : null"
    :default-agent="ops.defaultAgentProfile"
    :loading="offersAgents && ops.agentProfilesLoading"
    :failed="offersAgents && ops.agentProfilesFailed"
    :views="views"
    :applets="applets.sessionMenu.value"
    :native-terminal="panels.offersNativeTerminal(row)"
    @close="emit('close')"
    @terminal="panels.newWindow(row, panel)"
    @native-terminal="panels.newNativeTerminal(row, panel)"
    @agent="panels.newAgentWindow(row, $event, panel)"
    @view="panels.openViewIn(row, $event, panel)"
    @applet="panels.openAppletIn(row, $event, panel)"
    @retry="ops.loadAgentProfiles"
  />
</template>
