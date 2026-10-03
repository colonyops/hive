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
      hint="The connected Grafana stack to fetch as. Emits one item per active IRM alert group."
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
      @update:model-value="update('interval', $event)"
    />
  </div>
</template>
