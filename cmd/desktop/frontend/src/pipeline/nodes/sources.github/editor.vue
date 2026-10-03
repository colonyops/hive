<script setup lang="ts">
// sources.github has no runtime.ts (the source runs in Go). The editor embeds
// the fetch config directly — a "search" source runs a query, a
// "notifications" source drains the inbox — matching the backend
// github.Config it round-trips to.
import { computed } from 'vue'
import { CredentialField, IntervalField, NumberField, SelectField, TextField, type SelectOption } from '../../fields'
import type { Config, SourceKind } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const KIND_OPTIONS: SelectOption[] = [
  { value: 'search', label: 'Search — run a GitHub query' },
  { value: 'notifications', label: 'Notifications — drain the inbox' },
]

const isSearch = computed(() => props.config.kind === 'search')

function update<K extends keyof Config>(key: K, value: Config[K]) {
  emit('update:config', { ...props.config, [key]: value })
}

function updateKind(kind: string) {
  // Switching to notifications clears the now-meaningless query.
  const next: Config = { ...props.config, kind: kind as SourceKind }
  if (kind === 'notifications') next.query = ''
  emit('update:config', next)
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <CredentialField
      provider="github"
      :model-value="config.credential"
      testid="sources.github-editor-credential"
      @update:model-value="update('credential', $event)"
    />
    <SelectField
      label="Kind"
      :model-value="config.kind"
      :options="KIND_OPTIONS"
      testid="sources.github-editor-kind"
      @update:model-value="updateKind"
    />
    <TextField
      v-if="isSearch"
      label="Query"
      :model-value="config.query ?? ''"
      placeholder="is:open is:pr archived:false"
      hint="A GitHub search query. Costs one search request per poll."
      monospace
      testid="sources.github-editor-query"
      @update:model-value="update('query', $event)"
    />
    <NumberField
      label="Limit"
      :model-value="config.limit ?? 0"
      :placeholder="isSearch ? '50 (max 100)' : '50 (max 50)'"
      hint="Max items per fetch. 0 uses the default (50)."
      testid="sources.github-editor-limit"
      @update:model-value="update('limit', $event || undefined)"
    />
    <IntervalField
      :model-value="config.interval"
      testid="sources.github-editor-interval"
      @update:model-value="update('interval', $event)"
    />
  </div>
</template>
