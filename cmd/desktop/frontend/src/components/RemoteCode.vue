<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRemoteCode } from '../stores/useRemoteCode'
import { useRemoteCatalog } from '../stores/useRemoteCatalog'
import { useRouter } from 'vue-router'
import { useRemoteConnection } from '../stores/useRemoteConnection'
import { useCommands, type Command } from '../composables/useCommands'
import BaseButton from './ui/BaseButton.vue'
import EmptyState from './ui/EmptyState.vue'
import InlineError from './ui/InlineError.vue'
import RemoteTerminal from './RemoteTerminal.vue'
import RemoteSessionStatusBar from './RemoteSessionStatusBar.vue'
import { useTerminalStatusBar } from '../stores/useTerminalStatusBar'

const props = defineProps<{ active: boolean }>()
const router = useRouter()
const { endpoint, sessions, error, reload, disconnect } = useRemoteConnection()
const catalog = useRemoteCatalog()
const code = useRemoteCode()
const { showStatusBar } = useTerminalStatusBar()
const selected = catalog.codeId
const selectedSession = computed(() =>
  sessions.value.find((row) => row.id === selected.value && row.state === 'active'),
)

function openSettings(): void {
  void router.push({ name: 'application-settings', params: { section: 'remote' } })
}

watch(sessions, (rows) => {
  if (!rows.some((row) => row.id === selected.value && row.state === 'active')) catalog.selectCode('')
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
          catalog.selectCode(row.id)
        },
      })),
  ]
})
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col" data-testid="remote-code">
    <div v-if="!endpoint" class="m-auto flex flex-col items-center gap-3 p-6" data-testid="remote-not-connected">
      <EmptyState message="Connect to a remote Hive installation in Settings." />
      <BaseButton data-testid="remote-open-settings" @click="openSettings">Open Remote settings</BaseButton>
    </div>
    <InlineError v-if="error" :message="error" testid="remote-error" />
    <template v-if="endpoint">
      <RemoteSessionStatusBar
        v-if="showStatusBar && active && selectedSession"
        :key="`${endpoint.httpBaseURL}/${selectedSession.id}`"
        :endpoint="endpoint"
        :session-id="selectedSession.id"
      />
      <div class="flex min-h-0 flex-1">
        <RemoteTerminal
          v-if="active && selectedSession"
          :key="`${endpoint.httpBaseURL}/${selectedSession.id}/${selectedSession.slug}`"
          :slug="selectedSession.slug"
          :endpoint="endpoint"
          :window-id="code.selectedWindow.value"
          code
          @selected="code.selectWindow"
        />
        <EmptyState v-else message="Select a remote session to attach." />
      </div>
    </template>
  </div>
</template>
