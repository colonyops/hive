<script setup lang="ts">
// The servers under servers: in the hive config.yaml, and how each answered
// its last check. The list is config, so this page reads it and does not edit
// it.
import { onMounted, ref } from 'vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsRow from './settings/SettingsRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { Servers } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type { ServerStatus } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const servers = ref<ServerStatus[]>([])
const checking = ref(false)
const loaded = ref(false)
const error = ref<string | null>(null)

async function check(): Promise<void> {
  checking.value = true
  error.value = null
  try {
    servers.value = (await Servers()) ?? []
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Could not check the servers.'
  } finally {
    checking.value = false
    loaded.value = true
  }
}

function tone(state: string): 'success' | 'danger' | 'neutral' {
  if (state === 'connected') return 'success'
  if (state === 'offline' || state === 'incompatible') return 'danger'
  return 'neutral'
}

const stateLabels: Record<string, string> = {
  connected: 'Connected',
  offline: 'Offline',
  incompatible: 'Incompatible',
  connecting: 'Connecting',
}

function stateLabel(state: string): string {
  return stateLabels[state] ?? state
}

function hint(server: ServerStatus): string {
  const parts = [`ssh ${server.ssh}`]
  if (server.version) parts.push(`hive ${server.version}`)
  if (server.state === 'connected') parts.push(`${server.roundTripMs} ms`)
  return parts.join(' · ')
}

onMounted(() => {
  void check()
})
</script>

<template>
  <SettingsPage testid="settings-servers">
    <SettingsSection
      title="Configured servers"
      description="From servers: in the Hive config.yaml. Hive reaches each one over ssh."
      boxed
    >
      <template #actions>
        <BaseButton variant="secondary" size="sm" :busy="checking" data-testid="settings-servers-check" @click="check"
          >Check again</BaseButton
        >
      </template>
      <InlineError v-if="error" variant="line" class="px-4 py-3.5" :message="error" />
      <EmptyState
        v-else-if="loaded && !servers.length"
        variant="inline"
        class="px-4 py-3.5"
        message="No servers are configured."
        data-testid="settings-servers-empty"
      />
      <SettingsRow
        v-for="server in servers"
        :key="server.name"
        :label="server.name"
        :hint="server.error || hint(server)"
        :testid="`settings-server-${server.name}`"
      >
        <BaseBadge
          :tone="tone(server.state)"
          variant="pill"
          dot
          class="px-2.5 py-0.5 text-caption font-medium"
          :data-testid="`settings-server-${server.name}-state`"
          >{{ stateLabel(server.state) }}</BaseBadge
        >
      </SettingsRow>
    </SettingsSection>
  </SettingsPage>
</template>
