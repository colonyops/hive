<script setup lang="ts">
import { computed } from 'vue'
import { IntervalField, SelectField, TextField, type SelectOption } from '../../fields'
import { useIntegrations } from '../../../composables/useIntegrations'
import type { Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const { credentialRefsFor, loaded: integrationsLoaded } = useIntegrations()
const connectedRefs = computed(() => credentialRefsFor('grafana'))

const credentialOptions = computed<SelectOption[]>(() => {
  const options: SelectOption[] = connectedRefs.value.map((ref) => ({ value: ref, label: ref }))
  const current = props.config.credential
  if (current && !connectedRefs.value.includes(current)) {
    options.unshift({ value: current, label: `${current} — not connected` })
  }
  return options
})

const credentialHint = computed(() => {
  if (!integrationsLoaded.value) return 'Loading connected stacks…'
  if (connectedRefs.value.length === 0) return 'No Grafana stack is connected. Connect one in Settings ▸ Integrations.'
  return 'The connected Grafana stack to fetch as. Emits one item per active IRM alert group.'
})

function update<K extends keyof Config>(key: K, value: Config[K]) {
  emit('update:config', { ...props.config, [key]: value })
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <SelectField
      v-if="credentialOptions.length > 0"
      label="Stack"
      :model-value="config.credential ?? ''"
      :options="credentialOptions"
      :hint="credentialHint"
      testid="sources.grafana_irm_alerts-editor-credential"
      @update:model-value="update('credential', $event)"
    />
    <TextField
      v-else
      label="Stack"
      :model-value="config.credential ?? ''"
      placeholder="grafana/grafana.example.com-1"
      :hint="credentialHint"
      monospace
      testid="sources.grafana_irm_alerts-editor-credential"
      @update:model-value="update('credential', $event)"
    />
    <TextField
      label="Integration"
      :model-value="config.integration ?? ''"
      placeholder="CFRPV98RPR1U8"
      hint="An IRM integration id, from its page in Grafana. This is what pins a feed to one squad's alerts. Empty fetches every integration."
      monospace
      testid="sources.grafana_irm_alerts-editor-integration"
      @update:model-value="update('integration', $event)"
    />
    <TextField
      label="Team"
      :model-value="config.team ?? ''"
      placeholder="T3HRAP3K2FE1J"
      hint="An IRM team id. Empty fetches every team."
      monospace
      testid="sources.grafana_irm_alerts-editor-team"
      @update:model-value="update('team', $event)"
    />
    <IntervalField
      :model-value="config.interval"
      testid="sources.grafana_irm_alerts-editor-interval"
      @update:model-value="(interval?: string) => update('interval', interval)"
    />
  </div>
</template>
