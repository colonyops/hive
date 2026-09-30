<script setup lang="ts">
import { computed, h, markRaw, type Component } from 'vue'
import IconTerminal from '~icons/lucide/terminal'
import AgentIcon, { agentIconID } from './AgentIcon.vue'
import AppMenu from './AppMenu.vue'
import type { MenuEntry } from '../types/menu'

const props = defineProps<{
  running: boolean
  flip: boolean
  ignore: (HTMLElement | null)[]
  agents: string[]
  defaultAgent: string
  loading: boolean
  failed: boolean
}>()
const emit = defineEmits<{ close: []; terminal: []; agent: [profile: string]; retry: [] }>()

const agentIcons = new Map<string, Component>()
function agentIcon(agent: string): Component | undefined {
  const id = agentIconID(agent)
  if (!id) return undefined
  let icon = agentIcons.get(id)
  if (!icon) {
    icon = markRaw(() => h(AgentIcon, { id }))
    agentIcons.set(id, icon)
  }
  return icon
}

const entries = computed<MenuEntry[]>(() => [
  { kind: 'action', id: 'terminal', label: props.running ? 'New terminal' : 'Start session', icon: IconTerminal, testid: 'new-window-terminal' },
  { kind: 'separator' },
  ...(!props.running ? [{ kind: 'label' as const, text: 'Start the session to add an agent' }] : []),
  ...(props.loading
    ? [{ kind: 'label' as const, text: 'Loading agents…' }]
    : props.failed
      ? [{ kind: 'action' as const, id: 'retry', label: 'Could not load agents. Retry', testid: 'new-window-retry' }]
      : props.agents.length
        ? props.agents.map((agent) => ({ kind: 'action' as const, id: `agent:${agent}`, label: agent === props.defaultAgent ? `${agent} (default)` : agent, icon: agentIcon(agent), disabled: !props.running, testid: `new-window-agent-${agent}` }))
        : [{ kind: 'label' as const, text: 'No agents configured' }]),
])

function select(id: string): void {
  if (id === 'retry') {
    emit('retry')
    return
  }
  emit('close')
  if (id === 'terminal') emit('terminal')
  else if (id.startsWith('agent:')) emit('agent', id.slice(6))
}
</script>

<template>
  <AppMenu :entries="entries" :flip="flip" :ignore="ignore" width="min(200px, 100%)" testid="new-window-menu" @select="select" @close="emit('close')" />
</template>
