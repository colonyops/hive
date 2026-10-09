<script setup lang="ts">
import { computed, onUnmounted, ref, shallowRef } from 'vue'
import { useIntervalFn } from '@vueuse/core'
import { useCommands, type Command } from '../composables/useCommands'
import { errorText } from '../lib/appError'
import { listRemoteSessions, remoteEndpoint, type RemoteSession } from '../lib/remoteConnection'
import type { TerminalEndpoint } from '../lib/terminalClient'
import BaseButton from './ui/BaseButton.vue'
import EmptyState from './ui/EmptyState.vue'
import FormField from './ui/FormField.vue'
import InlineError from './ui/InlineError.vue'
import TextInput from './ui/TextInput.vue'
import RemoteTerminal from './RemoteTerminal.vue'

const props = defineProps<{ active: boolean }>()
const address = ref('http://127.0.0.1:19001')
const token = ref('')
const endpoint = shallowRef<TerminalEndpoint | null>(null)
const sessions = ref<RemoteSession[]>([])
const selected = ref('')
const busy = ref(false)
const error = ref<string | null>(null)
let generation = 0
const selectedSession = computed(() => sessions.value.find((row) => row.id === selected.value))

async function connect(): Promise<void> {
  const request = ++generation
  busy.value = true
  error.value = null
  try {
    const target = remoteEndpoint(address.value, token.value)
    const rows = await listRemoteSessions(target)
    if (request !== generation) return
    sessions.value = rows
    endpoint.value = target
    token.value = ''
  } catch (err) {
    if (request === generation) error.value = errorText(err, 'Could not connect to remote Hive.')
  } finally {
    if (request === generation) busy.value = false
  }
}

async function reload(): Promise<void> {
  if (!endpoint.value || busy.value) return
  const request = generation
  busy.value = true
  try {
    const rows = await listRemoteSessions(endpoint.value)
    if (request !== generation) return
    sessions.value = rows
    error.value = null
    if (!rows.some((row) => row.id === selected.value && row.state === 'active')) selected.value = ''
  } catch (err) {
    if (request === generation)
      error.value = errorText(err, 'Remote Hive is unreachable. Showing the last session list.')
  } finally {
    if (request === generation) busy.value = false
  }
}

function disconnect(): void {
  generation++
  endpoint.value = null
  sessions.value = []
  selected.value = ''
  token.value = ''
  busy.value = false
  error.value = null
}

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
onUnmounted(disconnect)
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col" data-testid="remote-code">
    <form v-if="!endpoint" class="mx-auto flex w-full max-w-lg flex-col gap-4 p-6" @submit.prevent="connect">
      <h2 class="text-body font-semibold">Connect to remote Hive</h2>
      <p class="text-xs text-text-3">
        Open an SSH tunnel to the remote installation, then enter its forwarded address and connection token. This
        preview supports existing Code sessions. Credentials stay in memory.
      </p>
      <FormField v-slot="{ id }" label="Forwarded address"
        ><TextInput :id="id" v-model="address" data-testid="remote-address"
      /></FormField>
      <FormField v-slot="{ id }" label="Connection token"
        ><TextInput :id="id" v-model="token" type="password" autocomplete="off" data-testid="remote-token"
      /></FormField>
      <BaseButton type="submit" :busy="busy" data-testid="remote-connect">Connect</BaseButton>
    </form>
    <InlineError v-if="error" :message="error" testid="remote-error" />
    <template v-if="endpoint">
      <div class="flex items-center gap-3 border-b border-border px-3 py-2">
        <span class="text-xs text-text-3">Remote · {{ endpoint.httpBaseURL }}</span>
        <BaseButton size="xs" :busy="busy" data-testid="remote-refresh" @click="reload">Refresh</BaseButton>
        <BaseButton size="xs" data-testid="remote-disconnect" @click="disconnect">Disconnect</BaseButton>
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
