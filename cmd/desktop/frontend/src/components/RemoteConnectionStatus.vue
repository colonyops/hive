<script setup lang="ts">
import IconUnplug from '~icons/lucide/unplug'
import IconRefresh from '~icons/lucide/refresh-cw'
import { useRemoteConnection } from '../stores/useRemoteConnection'
import BaseButton from './ui/BaseButton.vue'
import { useCommands } from '../composables/useCommands'

const { activeProfile, status, error, busy, reconnect } = useRemoteConnection()
useCommands(() =>
  activeProfile.value
    ? [
        {
          id: 'remote:reconnect',
          title: 'Reconnect remote server',
          group: `Remote · ${activeProfile.value.name}`,
          scope: 'actions',
          run: () => void reconnect(),
        },
      ]
    : [],
)
</script>

<template>
  <div
    v-if="activeProfile && status !== 'connected'"
    class="flex shrink-0 items-center gap-2 border-b border-border bg-sunken px-3 py-2 text-caption"
    data-testid="remote-connection-status"
    role="status"
  >
    <IconUnplug class="size-3.5 shrink-0 text-severity-warning" aria-hidden="true" />
    <span class="shrink-0 text-text-1"
      >{{ activeProfile.name }} · {{ status === 'connecting' ? 'Reconnecting…' : 'Disconnected' }}</span
    >
    <span class="min-w-0 flex-1 truncate text-text-3" :title="error || ''">{{ error }}</span>
    <BaseButton size="xs" variant="secondary" :busy="busy" data-testid="remote-server-reconnect" @click="reconnect">
      <IconRefresh class="size-3.5" aria-hidden="true" />Reconnect server
    </BaseButton>
  </div>
</template>
