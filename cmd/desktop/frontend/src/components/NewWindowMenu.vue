<script setup lang="ts">
import { computed, type Component } from 'vue'
import IconTerminal from '~icons/lucide/terminal'
import IconSquareTerminal from '~icons/lucide/square-terminal'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import { agentBrandIcon } from './terminal/agentBrandIcon'
import type { SessionViewKind } from './terminal/panelViews'
import type { MenuEntry } from '@hive/ui/types/menu'
import { appletMenuChoice, appletMenuEntries } from './applets/appletMenu'
import type { Applet } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const props = defineProps<{
  running: boolean
  flip: boolean
  ignore: (HTMLElement | null)[]
  anchor?: HTMLElement | null
  /** Null where agents cannot run at all, which drops the agent part of the menu. */
  agents: string[] | null
  defaultAgent: string
  loading: boolean
  failed: boolean
  views?: { kind: SessionViewKind; label: string; icon: Component }[]
  applets?: readonly Applet[]
  /** Offer a native terminal: a shell in herdr-core, drawn by libghostty. */
  nativeTerminal?: boolean
}>()
const emit = defineEmits<{
  close: []
  terminal: []
  nativeTerminal: []
  agent: [profile: string]
  view: [kind: SessionViewKind]
  applet: [id: string]
  retry: []
}>()

function agentEntries(agents: string[]): MenuEntry[] {
  return [
    { kind: 'separator' },
    ...(!props.running ? [{ kind: 'label' as const, text: 'Start the session to add an agent' }] : []),
    ...(props.loading
      ? [{ kind: 'label' as const, text: 'Loading agents…' }]
      : props.failed
        ? [{ kind: 'action' as const, id: 'retry', label: 'Could not load agents. Retry', testid: 'new-window-retry' }]
        : agents.length
          ? agents.map((agent) => ({
              kind: 'action' as const,
              id: `agent:${agent}`,
              label: agent === props.defaultAgent ? `${agent} (default)` : agent,
              icon: agentBrandIcon(agent),
              disabled: !props.running,
              testid: `new-window-agent-${agent}`,
            }))
          : [{ kind: 'label' as const, text: 'No agents configured' }]),
  ]
}

const entries = computed<MenuEntry[]>(() => [
  {
    kind: 'action',
    id: 'terminal',
    label: props.running ? 'New terminal' : 'Start session',
    icon: IconTerminal,
    testid: 'new-window-terminal',
  },
  ...(props.nativeTerminal
    ? [
        {
          kind: 'action' as const,
          id: 'native-terminal',
          label: 'New native terminal',
          icon: IconSquareTerminal,
          testid: 'new-window-native-terminal',
        },
      ]
    : []),
  ...(props.agents ? agentEntries(props.agents) : []),
  ...(props.views?.length ? [{ kind: 'separator' as const }] : []),
  ...(props.views ?? []).map((view) => ({
    kind: 'action' as const,
    id: `view:${view.kind}`,
    label: view.label,
    icon: view.icon,
    testid: `new-window-view-${view.kind}`,
  })),
  ...(props.applets?.length ? [{ kind: 'separator' as const }] : []),
  ...appletMenuEntries(props.applets ?? []),
])

function select(id: string): void {
  if (id === 'retry') {
    emit('retry')
    return
  }
  emit('close')
  if (id === 'terminal') emit('terminal')
  else if (id === 'native-terminal') emit('nativeTerminal')
  else if (id.startsWith('agent:')) emit('agent', id.slice(6))
  else if (id.startsWith('view:')) emit('view', id.slice(5) as SessionViewKind)
  else {
    const choice = appletMenuChoice(id)
    if (choice) emit('applet', choice)
  }
}
</script>

<template>
  <AppMenu
    :entries="entries"
    :flip="flip"
    :ignore="ignore"
    :anchor="anchor ?? undefined"
    :align="anchor ? 'start' : undefined"
    width="min(200px, 100%)"
    testid="new-window-menu"
    @select="select"
    @close="emit('close')"
  />
</template>
