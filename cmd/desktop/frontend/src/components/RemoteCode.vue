<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useRemoteConnection } from '../stores/useRemoteConnection'
import { useIntervalFn } from '@vueuse/core'
import { useCommands, type Command } from '../composables/useCommands'
import BaseButton from './ui/BaseButton.vue'
import EmptyState from './ui/EmptyState.vue'
import InlineError from './ui/InlineError.vue'
import RemoteTerminal from './RemoteTerminal.vue'

const props = defineProps<{ active: boolean }>()
const router = useRouter()
const { endpoint, sessions, busy, error, color, activeProfile, reload, disconnect } = useRemoteConnection()
const selected = ref('')
const selectedSession = computed(() =>
  sessions.value.find((row) => row.id === selected.value && row.state === 'active'),
)

function openSettings(): void {
  void router.push({ name: 'application-settings', params: { section: 'remote' } })
}

watch(endpoint, () => {
  selected.value = ''
})
watch(sessions, (rows) => {
  if (!rows.some((row) => row.id === selected.value && row.state === 'active')) selected.value = ''
})

useCommands((): Command[] => {
  if (!props.active || !endpoint.value) return []
  const host = endpoint.value.httpBaseURL
  return [
    {
      id: 'remote:refresh',
      title: 'Refresh sessions',
      group: `Remote · ${host}`,
      scope: 'actions',
      run: () => void reload(),
    },
    { id: 'remote:disconnect', title: 'Disconnect', group: `Remote · ${host}`, scope: 'actions', run: disconnect },
    ...sessions.value
      .filter((row) => row.state === 'active')
      .map((row): Command => ({
        id: `remote:${host}:session:${row.id}`,
        title: row.name,
        group: `Remote · ${host}`,
        scope: 'goto',
        run: () => {
          selected.value = row.id
        },
      })),
  ]
})

useIntervalFn(() => {
  if (props.active) void reload()
}, 10000)
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col" data-testid="remote-code">
    <div v-if="!endpoint" class="m-auto flex flex-col items-center gap-3 p-6" data-testid="remote-not-connected">
      <EmptyState message="Connect to a remote Hive installation in Settings." />
      <BaseButton data-testid="remote-open-settings" @click="openSettings">Open Remote settings</BaseButton>
    </div>
    <InlineError v-if="error" :message="error" testid="remote-error" />
    <template v-if="endpoint">
      <div class="flex items-center gap-3 border-b border-border px-3 py-2">
        <span class="flex items-center gap-2 text-xs text-text-3">
          <span
            class="size-2.5 rounded-full border border-strong"
            :style="{ backgroundColor: color }"
            aria-hidden="true"
          />
          {{ activeProfile?.name }} · {{ endpoint.httpBaseURL }}
        </span>
        <BaseButton size="xs" :busy="busy" data-testid="remote-refresh" @click="reload">Refresh</BaseButton>
        <BaseButton size="xs" variant="secondary" data-testid="remote-open-settings" @click="openSettings"
          >Remote settings</BaseButton
        >
        <span class="text-xs text-text-4">Existing sessions · file transfer unavailable</span>
      </div>
      <div class="flex min-h-0 flex-1">
        <aside
          class="flex w-60 shrink-0 flex-col gap-1 overflow-y-auto border-r border-border p-2"
          data-testid="remote-sessions"
        >
          <BaseButton
            v-for="row in sessions"
            :key="row.id"
            :disabled="row.state !== 'active'"
            :variant="selected === row.id ? 'primary' : 'ghost'"
            :title="row.repo"
            :data-testid="`remote-session-${row.slug}`"
            @click="selected = row.id"
            >{{ row.name }}{{ row.state !== 'active' ? ` (${row.state})` : '' }}</BaseButton
          >
          <EmptyState v-if="!sessions.length" message="No Hive sessions on this installation." />
        </aside>
        <RemoteTerminal
          v-if="active && selectedSession"
          :key="`${endpoint.httpBaseURL}/${selectedSession.id}/${selectedSession.slug}`"
          :slug="selectedSession.slug"
          :endpoint="endpoint"
        />
        <EmptyState v-else message="Select a remote session to attach." />
      </div>
    </template>
  </div>
</template>
