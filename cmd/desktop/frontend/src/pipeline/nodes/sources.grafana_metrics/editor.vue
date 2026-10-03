<script setup lang="ts">
import { CredentialField, IntervalField, TextField } from '../../fields'
import type { Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

function update<K extends keyof Config>(key: K, value: Config[K]) {
  emit('update:config', { ...props.config, [key]: value })
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <CredentialField
      provider="grafana"
      :model-value="config.credential"
      testid="sources.grafana_metrics-editor-credential"
      @update:model-value="update('credential', $event)"
    />
    <TextField
      label="Datasource UID"
      :model-value="config.datasource_uid ?? ''"
      placeholder="ceit8j1s1dfera"
      hint="The uid of the Prometheus-compatible datasource to query."
      monospace
      testid="sources.grafana_metrics-editor-datasource"
      @update:model-value="update('datasource_uid', $event)"
    />
    <TextField
      label="Query"
      :model-value="config.expr ?? ''"
      placeholder="sum(rate(http_requests_total[5m]))"
      hint="A PromQL expression. Runs once per poll."
      monospace
      testid="sources.grafana_metrics-editor-expr"
      @update:model-value="update('expr', $event)"
    />
    <TextField
      label="Title"
      :model-value="config.title ?? ''"
      placeholder="Request rate"
      hint="The feed item's title. Defaults to the query when empty."
      testid="sources.grafana_metrics-editor-title"
      @update:model-value="update('title', $event || undefined)"
    />
    <IntervalField
      :model-value="config.interval"
      testid="sources.grafana_metrics-editor-interval"
      @update:model-value="update('interval', $event)"
    />
  </div>
</template>
