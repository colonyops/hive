<script setup lang="ts">
import {
  CredentialField,
  IntervalField,
  NumberField,
  SelectField,
  TextField,
  ToggleField,
  type SelectOption,
} from '../../fields'
import { MAX_LIMIT, ORDERINGS, STATUSES, type Config, type Ordering, type Status } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const statusOptions: SelectOption[] = STATUSES.map((value) => ({ value, label: value }))
const orderOptions: SelectOption[] = ORDERINGS.map((value) => ({ value, label: value.replace(/_/g, ' ') }))

function update<K extends keyof Config>(key: K, value: Config[K]) {
  emit('update:config', { ...props.config, [key]: value })
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <CredentialField
      provider="posthog"
      :model-value="config.credential"
      hint="The connected PostHog project to fetch as. Emits one item per issue."
      testid="sources.posthog_errors-editor-credential"
      @update:model-value="update('credential', $event)"
    />
    <SelectField
      label="Status"
      :model-value="config.status ?? 'active'"
      :options="statusOptions"
      hint="Which issues to fetch."
      testid="sources.posthog_errors-editor-status"
      @update:model-value="update('status', $event as Status)"
    />
    <SelectField
      label="Order by"
      :model-value="config.order_by ?? 'last_seen'"
      :options="orderOptions"
      hint="How issues are ranked before the limit is applied, descending."
      testid="sources.posthog_errors-editor-order-by"
      @update:model-value="update('order_by', $event as Ordering)"
    />
    <TextField
      label="Date from"
      :model-value="config.date_from ?? ''"
      placeholder="-7d"
      hint="Start of the window the counts cover, as a PostHog relative date such as -7d or -24h."
      monospace
      testid="sources.posthog_errors-editor-date-from"
      @update:model-value="update('date_from', $event || undefined)"
    />
    <NumberField
      label="Limit"
      :model-value="config.limit ?? 25"
      :min="0"
      :max="MAX_LIMIT"
      hint="Issues per poll, taken from the top of the ordering. 0 uses the default of 25."
      testid="sources.posthog_errors-editor-limit"
      @update:model-value="update('limit', $event)"
    />
    <ToggleField
      label="Include test accounts"
      :model-value="config.include_test_accounts ?? false"
      hint="Include traffic PostHog classifies as internal or test."
      testid="sources.posthog_errors-editor-include-test-accounts"
      @update:model-value="update('include_test_accounts', $event)"
    />
    <IntervalField
      :model-value="config.interval"
      testid="sources.posthog_errors-editor-interval"
      @update:model-value="update('interval', $event)"
    />
  </div>
</template>
