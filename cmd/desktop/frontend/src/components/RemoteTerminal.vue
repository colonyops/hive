<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useCommands, type Command } from '../composables/useCommands'
import { useTerminalWindows } from '../composables/useTerminalWindows'
import { createTerminalClient, type TerminalEndpoint } from '../lib/terminalClient'
import BaseButton from './ui/BaseButton.vue'
import InlineError from './ui/InlineError.vue'
import TerminalTab from './TerminalTab.vue'
import '@xterm/xterm/css/xterm.css'

const props = defineProps<{ slug: string; endpoint: TerminalEndpoint }>()
// The parent keys this component by connection and session; changing either remounts it.
// eslint-disable-next-line vue/no-setup-props-reactivity-loss
const session = useTerminalWindows(props.slug, createTerminalClient(props.endpoint), { localImages: false })
const { tabs, activeWindowId, status, error, actionError, outputDropped } = session
useCommands((): Command[] => [
  {
    id: 'remote:reconnect',
    title: 'Reconnect terminal',
    group: `Remote · ${props.slug}`,
    scope: 'actions',
    run: () => void session.reconnect(),
  },
  ...tabs.value.map((tab): Command => ({
    id: `remote:${props.endpoint.httpBaseURL}:${props.slug}:${tab.windowId}`,
    title: tab.name,
    group: `Remote · ${props.slug}`,
    scope: 'goto',
    run: () => void session.select(tab.windowId),
  })),
])
onMounted(() => void session.start())
onUnmounted(session.dispose)
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col" data-testid="remote-terminal">
    <div class="flex items-center gap-2 border-b border-border px-3 py-2">
      <span class="text-xs text-text-3" data-testid="remote-terminal-status">{{ status }}</span>
      <BaseButton
        size="xs"
        :disabled="status === 'connecting'"
        data-testid="remote-reconnect"
        @click="session.reconnect"
        >Reconnect</BaseButton
      >
      <BaseButton
        v-for="tab in tabs"
        :key="tab.windowId"
        size="xs"
        :variant="activeWindowId === tab.windowId ? 'primary' : 'ghost'"
        @click="session.select(tab.windowId)"
        >{{ tab.name }}</BaseButton
      >
    </div>
    <InlineError v-if="error || actionError" :message="error || actionError" testid="remote-terminal-error" />
    <p v-if="outputDropped" class="px-3 py-2 text-xs text-text-3">
      Output was interrupted. The panes have been repainted from remote tmux.
    </p>
    <TerminalTab
      v-for="tab in tabs"
      :key="tab.uid"
      :tab="tab"
      :session="session"
      :active="activeWindowId === tab.windowId"
    />
  </div>
</template>
