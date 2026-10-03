<script setup lang="ts">
import { CredentialField, IntervalField, ToggleField } from '../../fields'
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
      provider="posthog"
      :model-value="config.credential"
      hint="The connected PostHog project to fetch as. Emits one item per insight alert."
      testid="sources.posthog_alerts-editor-credential"
      @update:model-value="update('credential', $event)"
    />
    <ToggleField
      label="Firing only"
      :model-value="config.firing_only ?? false"
      hint="Leave off so an alert that stops firing updates its existing item instead of vanishing."
      testid="sources.posthog_alerts-editor-firing-only"
      @update:model-value="update('firing_only', $event)"
    />
    <IntervalField
      :model-value="config.interval"
      testid="sources.posthog_alerts-editor-interval"
      @update:model-value="update('interval', $event)"
    />
  </div>
</template>
