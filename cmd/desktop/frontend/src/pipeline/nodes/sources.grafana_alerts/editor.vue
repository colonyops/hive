<script setup lang="ts">
import { CredentialField, GlobListField, IntervalField } from '../../fields'
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
      hint="The connected Grafana stack to fetch as. Emits one item per firing alert."
      testid="sources.grafana_alerts-editor-credential"
      @update:model-value="update('credential', $event)"
    />
    <GlobListField
      label="Matchers"
      :model-value="config.matchers ?? []"
      placeholder="squad=adaptive-telemetry&#10;severity=~critical|warning"
      hint="One label matcher per line, using =, !=, =~ or !~. An alert must match every one. Empty fetches the stack's whole active set."
      :rows="3"
      testid="sources.grafana_alerts-editor-matchers"
      @update:model-value="update('matchers', $event)"
    />
    <IntervalField
      :model-value="config.interval"
      testid="sources.grafana_alerts-editor-interval"
      @update:model-value="update('interval', $event)"
    />
  </div>
</template>
