<script setup lang="ts">
import { computed } from 'vue'
import { IntervalField, NumberField, SelectField, TextField, ToggleField, type SelectOption } from '../../fields'
import { useIntegrations } from '../../../composables/useIntegrations'
import { MAX_LIMIT, ORDERINGS, STATUSES, type Config, type Ordering, type Status } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const { credentialRefsFor, loaded: integrationsLoaded } = useIntegrations()
const connectedRefs = computed(() => credentialRefsFor('posthog'))

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
  if (!integrationsLoaded.value) return 'Loading connected projects…'
  if (connectedRefs.value.length === 0) return 'No PostHog project is connected. Connect one in Settings ▸ Integrations.'
  return 'The connected PostHog project to fetch as. Emits one item per issue.'
})

const statusOptions: SelectOption[] = STATUSES.map((value) => ({ value, label: value }))
const orderOptions: SelectOption[] = ORDERINGS.map((value) => ({ value, label: value.replace(/_/g, ' ') }))

function update(patch: Partial<Config>) {
  emit('update:config', { ...props.config, ...patch })
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <!-- Falls back to a text input when no project is connected, rather than an
         empty dropdown. -->
    <SelectField
      v-if="credentialOptions.length > 0"
      label="Project"
      :model-value="config.credential ?? ''"
      :options="credentialOptions"
      :hint="credentialHint"
      testid="sources.posthog_errors-editor-credential"
      @update:model-value="(credential: string) => update({ credential })"
    />
    <TextField
      v-else
      label="Project"
      :model-value="config.credential ?? ''"
      placeholder="posthog/us.posthog.com-1"
      :hint="credentialHint"
      monospace
      testid="sources.posthog_errors-editor-credential"
      @update:model-value="(credential: string) => update({ credential })"
    />
    <SelectField
      label="Status"
      :model-value="config.status ?? 'active'"
      :options="statusOptions"
      hint="Which issues to fetch."
      testid="sources.posthog_errors-editor-status"
      @update:model-value="(status: string) => update({ status: status as Status })"
    />
    <SelectField
      label="Order by"
      :model-value="config.order_by ?? 'last_seen'"
      :options="orderOptions"
      hint="How issues are ranked before the limit is applied, descending."
      testid="sources.posthog_errors-editor-order-by"
      @update:model-value="(order_by: string) => update({ order_by: order_by as Ordering })"
    />
    <TextField
      label="Date from"
      :model-value="config.date_from ?? ''"
      placeholder="-7d"
      hint="Start of the window the counts cover, as a PostHog relative date such as -7d or -24h."
      monospace
      testid="sources.posthog_errors-editor-date-from"
      @update:model-value="(date_from: string) => update({ date_from: date_from || undefined })"
    />
    <NumberField
      label="Limit"
      :model-value="config.limit ?? 25"
      :min="0"
      :max="MAX_LIMIT"
      hint="Issues per poll, taken from the top of the ordering. 0 uses the default of 25."
      testid="sources.posthog_errors-editor-limit"
      @update:model-value="(limit: number) => update({ limit })"
    />
    <ToggleField
      label="Include test accounts"
      :model-value="config.include_test_accounts ?? false"
      hint="Include traffic PostHog classifies as internal or test."
      testid="sources.posthog_errors-editor-include-test-accounts"
      @update:model-value="(include_test_accounts: boolean) => update({ include_test_accounts })"
    />
    <IntervalField
      :model-value="config.interval"
      testid="sources.posthog_errors-editor-interval"
      @update:model-value="(interval?: string) => update({ interval })"
    />
  </div>
</template>
