<script setup lang="ts">
import { computed } from 'vue'
import { IntervalField, SelectField, TextField, ToggleField, type SelectOption } from '../../fields'
import { useIntegrations } from '../../../composables/useIntegrations'
import type { Config } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const { credentialRefsFor, loaded: integrationsLoaded } = useIntegrations()
const connectedRefs = computed(() => credentialRefsFor('posthog'))

const credentialOptions = computed<SelectOption[]>(() => {
  const options: SelectOption[] = connectedRefs.value.map((ref) => ({ value: ref, label: ref }))
  const current = props.config.credential
  if (current && !connectedRefs.value.includes(current)) {
    options.unshift({ value: current, label: `${current} — not connected` })
  }
  return options
})

const credentialHint = computed(() => {
  if (!integrationsLoaded.value) return 'Loading connected projects…'
  if (connectedRefs.value.length === 0) return 'No PostHog project is connected. Connect one in Settings ▸ Integrations.'
  return 'The connected PostHog project to fetch as. Emits one item per insight alert.'
})

function update(patch: Partial<Config>) {
  emit('update:config', { ...props.config, ...patch })
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <SelectField
      v-if="credentialOptions.length > 0"
      label="Project"
      :model-value="config.credential ?? ''"
      :options="credentialOptions"
      :hint="credentialHint"
      testid="sources.posthog_alerts-editor-credential"
      @update:model-value="(credential: string) => update({ credential })"
    />
    <TextField
      v-else
      label="Project"
      :model-value="config.credential ?? ''"
      placeholder="posthog/us.posthog.com-1"
      :hint="credentialHint"
      monospace
      testid="sources.posthog_alerts-editor-credential"
      @update:model-value="(credential: string) => update({ credential })"
    />
    <ToggleField
      label="Firing only"
      :model-value="config.firing_only ?? false"
      hint="Leave off so an alert that stops firing updates its existing item instead of vanishing."
      testid="sources.posthog_alerts-editor-firing-only"
      @update:model-value="(firing_only: boolean) => update({ firing_only })"
    />
    <IntervalField
      :model-value="config.interval"
      testid="sources.posthog_alerts-editor-interval"
      @update:model-value="(interval?: string) => update({ interval })"
    />
  </div>
</template>
