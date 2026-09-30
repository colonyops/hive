<script setup lang="ts">
// sources.github has no runtime.ts (the source runs in Go). The editor embeds
// the fetch config directly — a "search" source runs a query, a
// "notifications" source drains the inbox — matching the backend
// github.Config it round-trips to.
import { computed } from 'vue'
import { IntervalField, NumberField, SelectField, TextField, type SelectOption } from '../../fields'
import { useIntegrations } from '../../../composables/useIntegrations'
import type { Config, SourceKind } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const KIND_OPTIONS: SelectOption[] = [
  { value: 'search', label: 'Search — run a GitHub query' },
  { value: 'notifications', label: 'Notifications — drain the inbox' },
]

const isSearch = computed(() => props.config.kind === 'search')

// The accounts actually connected, read from the same registry projection the
// Integrations screen renders — so this cannot offer an account the app holds
// no credential for.
const { credentialRefsFor, loaded: integrationsLoaded } = useIntegrations()
const connectedRefs = computed(() => credentialRefsFor('github'))

const credentialOptions = computed<SelectOption[]>(() => {
  const options: SelectOption[] = connectedRefs.value.map((ref) => ({ value: ref, label: ref }))
  // A node can name an account that has since been disconnected. Dropping it
  // from the list would silently rewrite the node's config on the next edit,
  // so it stays selectable and says why it is wrong.
  const current = props.config.credential
  if (current && !connectedRefs.value.includes(current)) {
    options.unshift({ value: current, label: `${current} — not connected` })
  }
  return options
})

const credentialHint = computed(() => {
  if (!integrationsLoaded.value) return 'Loading connected accounts…'
  if (connectedRefs.value.length === 0) return 'No GitHub account is connected. Connect one in Settings ▸ Integrations.'
  return 'The connected GitHub account to fetch as.'
})

function updateCredential(credential: string) {
  emit('update:config', { ...props.config, credential })
}

function updateKind(kind: string) {
  // Switching to notifications clears the now-meaningless query.
  const next: Config = { ...props.config, kind: kind as SourceKind }
  if (kind === 'notifications') next.query = ''
  emit('update:config', next)
}

function updateQuery(query: string) {
  emit('update:config', { ...props.config, query })
}

function updateLimit(limit: number) {
  emit('update:config', { ...props.config, limit: limit || undefined })
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <!-- With nothing connected and nothing already set there is no valid
         choice to offer, so the field stays a text input rather than an empty
         dropdown the user cannot act on. -->
    <SelectField
      v-if="credentialOptions.length > 0"
      label="Account"
      :model-value="config.credential ?? ''"
      :options="credentialOptions"
      :hint="credentialHint"
      testid="sources.github-editor-credential"
      @update:model-value="updateCredential"
    />
    <TextField
      v-else
      label="Account"
      :model-value="config.credential ?? ''"
      placeholder="github/octocat"
      :hint="credentialHint"
      monospace
      testid="sources.github-editor-credential"
      @update:model-value="updateCredential"
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
      @update:model-value="updateQuery"
    />
    <NumberField
      label="Limit"
      :model-value="config.limit ?? 0"
      :placeholder="isSearch ? '50 (max 100)' : '50 (max 50)'"
      hint="Max items per fetch. 0 uses the default (50)."
      testid="sources.github-editor-limit"
      @update:model-value="updateLimit"
    />
    <IntervalField
      :model-value="config.interval"
      testid="sources.github-editor-interval"
      @update:model-value="(interval?: string) => emit('update:config', { ...props.config, interval })"
    />
  </div>
</template>
