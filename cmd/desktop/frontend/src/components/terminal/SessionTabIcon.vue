<script setup lang="ts">
import { computed } from 'vue'
import IconTerminal from '~icons/lucide/terminal'
import AgentMark from '../AgentMark.vue'
import type { AgentIconID } from '../../lib/agentIcon'
import { launcherIconComponent } from '../../lib/launcherIcons'
import { useApplets } from '../../stores/useApplets'
import { isSessionViewKind, sessionView } from './panelViews'

// One mark for a tab and for its tree row, so the two read as the same thing.
const props = defineProps<{ kind: string; agent?: AgentIconID; applet?: string }>()

const applets = useApplets()
const icon = computed(() => {
  if (props.applet) return launcherIconComponent(applets.byID(props.applet)?.icon ?? '')
  return isSessionViewKind(props.kind) ? sessionView(props.kind).icon : IconTerminal
})
</script>

<template>
  <AgentMark v-if="agent" :agent="agent" />
  <component :is="icon" v-else class="size-3.5 shrink-0" aria-hidden="true" />
</template>
