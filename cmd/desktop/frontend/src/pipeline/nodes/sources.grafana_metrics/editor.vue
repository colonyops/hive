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
  // Keep a since-disconnected credential selectable; dropping it would silently
  // rewrite the node's config on the next edit.
  const current = props.config.credential
  if (current && !connectedRefs.value.includes(current)) {
    options.unshift({ value: current, label: `${current} — not connected` })
  }
  return options
})

const credentialHint = computed(() => {
  if (!integrationsLoaded.value) return 'Loading connected stacks…'
  if (connectedRefs.value.length === 0) return 'No Grafana stack is connected. Connect one in Settings ▸ Integrations.'
  return 'The connected Grafana stack to fetch as.'
})

function updateCredential(credential: string) {
  emit('update:config', { ...props.config, credential })
}

function updateDatasource(datasource_uid: string) {
  emit('update:config', { ...props.config, datasource_uid })
}

function updateExpr(expr: string) {
  emit('update:config', { ...props.config, expr })
}

function updateTitle(title: string) {
  emit('update:config', { ...props.config, title: title || undefined })
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <!-- Falls back to a text input when no stack is connected, rather than an
         empty dropdown. -->
    <SelectField
      v-if="credentialOptions.length > 0"
      label="Stack"
      :model-value="config.credential ?? ''"
      :options="credentialOptions"
      :hint="credentialHint"
      testid="sources.grafana_metrics-editor-credential"
      @update:model-value="updateCredential"
    />
    <TextField
      v-else
      label="Stack"
      :model-value="config.credential ?? ''"
      placeholder="grafana/grafana.example.com-1"
      :hint="credentialHint"
      monospace
      testid="sources.grafana_metrics-editor-credential"
      @update:model-value="updateCredential"
    />
    <TextField
      label="Datasource UID"
      :model-value="config.datasource_uid ?? ''"
      placeholder="ceit8j1s1dfera"
      hint="The uid of the Prometheus-compatible datasource to query."
      monospace
      testid="sources.grafana_metrics-editor-datasource"
      @update:model-value="updateDatasource"
    />
    <TextField
      label="Query"
      :model-value="config.expr ?? ''"
      placeholder="sum(rate(http_requests_total[5m]))"
      hint="A PromQL expression. Runs once per poll."
      monospace
      testid="sources.grafana_metrics-editor-expr"
      @update:model-value="updateExpr"
    />
    <TextField
      label="Title"
      :model-value="config.title ?? ''"
      placeholder="Request rate"
      hint="The feed item's title. Defaults to the query when empty."
      testid="sources.grafana_metrics-editor-title"
      @update:model-value="updateTitle"
    />
    <IntervalField
      :model-value="config.interval"
      testid="sources.grafana_metrics-editor-interval"
      @update:model-value="(interval?: string) => emit('update:config', { ...props.config, interval })"
    />
  </div>
</template>
