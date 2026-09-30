<script setup lang="ts">
import { computed } from 'vue'
import { GlobListField, IntervalField, SelectField, TextField, type SelectOption } from '../../fields'
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
  return 'The connected Grafana stack to fetch as. Emits one item per firing alert.'
})

function updateCredential(credential: string) {
  emit('update:config', { ...props.config, credential })
}

function updateMatchers(matchers: string[]) {
  emit('update:config', { ...props.config, matchers })
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
      testid="sources.grafana_alerts-editor-credential"
      @update:model-value="updateCredential"
    />
    <TextField
      v-else
      label="Stack"
      :model-value="config.credential ?? ''"
      placeholder="grafana/grafana.example.com-1"
      :hint="credentialHint"
      monospace
      testid="sources.grafana_alerts-editor-credential"
      @update:model-value="updateCredential"
    />
    <GlobListField
      label="Matchers"
      :model-value="config.matchers ?? []"
      placeholder="squad=adaptive-telemetry&#10;severity=~critical|warning"
      hint="One label matcher per line, using =, !=, =~ or !~. An alert must match every one. Empty fetches the stack's whole active set."
      :rows="3"
      testid="sources.grafana_alerts-editor-matchers"
      @update:model-value="updateMatchers"
    />
    <IntervalField
      :model-value="config.interval"
      testid="sources.grafana_alerts-editor-interval"
      @update:model-value="(interval?: string) => emit('update:config', { ...props.config, interval })"
    />
  </div>
</template>
