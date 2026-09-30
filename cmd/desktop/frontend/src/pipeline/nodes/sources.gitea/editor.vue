<script setup lang="ts">
// sources.gitea has no runtime.ts (the source runs in Go). The editor embeds
// the fetch config directly, matching the backend gitea.Config it round-trips
// to: a "search" source runs a filtered query, a "notifications" source drains
// the inbox.
//
// The search half is typed controls rather than a query box because Gitea has
// no search syntax — its endpoint takes discrete parameters — so the fields
// here are the whole vocabulary a search has.
import { computed } from 'vue'
import { FieldRow, GlobListField, IntervalField, NumberField, SelectField, TextField, ToggleField, type SelectOption } from '../../fields'
import { useIntegrations } from '../../../composables/useIntegrations'
import { INVOLVING, ITEMS, STATES, type Config, type Involving, type Items, type Kind, type State } from './config'

const props = defineProps<{ config: Config; errors?: string[] }>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const KIND_OPTIONS: SelectOption[] = [
  { value: 'search', label: 'Search — run a filtered query' },
  { value: 'notifications', label: 'Notifications — drain the inbox' },
]

const ITEM_OPTIONS: SelectOption[] = [
  { value: 'all', label: 'All — issues and pull requests' },
  { value: 'issues', label: 'Issues only' },
  { value: 'pulls', label: 'Pull requests only' },
]

const STATE_OPTIONS: SelectOption[] = [
  { value: 'open', label: 'Open' },
  { value: 'closed', label: 'Closed' },
  { value: 'all', label: 'All' },
]

const INVOLVING_LABELS: Record<Involving, string> = {
  created: 'Created by me',
  assigned: 'Assigned to me',
  mentioned: 'Mentions me',
  review_requested: 'My review is requested',
  reviewed: 'I have reviewed',
}

const isSearch = computed(() => props.config.kind === 'search')

// The accounts actually connected, read from the same registry projection the
// Integrations screen renders — so this cannot offer an account the app holds
// no credential for.
const { credentialRefsFor, loaded: integrationsLoaded } = useIntegrations()
const connectedRefs = computed(() => credentialRefsFor('gitea'))

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
  if (connectedRefs.value.length === 0) return 'No Gitea instance is connected. Connect one in Settings ▸ Integrations.'
  return 'The connected Gitea or Forgejo account to fetch as.'
})

const involving = computed<Involving[]>(() => props.config.involving ?? [])

// One request per entry, so the cost of a union is worth stating where it is
// chosen rather than only in the node docs.
const involvingHint = computed(() => {
  const count = involving.value.length
  if (count === 0) return 'Every item the token can see. One request per poll.'
  if (count === 1) return 'Items matching this relationship. One request per poll.'
  return `Items matching any of these. ${count} requests per poll — Gitea cannot combine them.`
})

function update(patch: Partial<Config>) {
  emit('update:config', { ...props.config, ...patch })
}

function updateKind(kind: string) {
  const next: Config = { ...props.config, kind: kind as Kind }
  if (kind === 'notifications') {
    // The inbox cannot be filtered server-side, and Go rejects a notifications
    // source that still carries these — so switching drops them rather than
    // leaving a node that looks filtered and will not save.
    next.items = undefined
    next.state = undefined
    next.involving = undefined
    next.owner = undefined
    next.labels = undefined
    next.text = undefined
  } else {
    next.items = next.items ?? 'all'
    next.state = next.state ?? 'open'
  }
  emit('update:config', next)
}

function toggleInvolving(value: Involving, on: boolean) {
  // Rebuilt from the declared order, so the stored list does not depend on
  // which checkbox the user happened to tick first.
  const selected = new Set(involving.value)
  if (on) selected.add(value)
  else selected.delete(value)
  update({ involving: INVOLVING.filter((entry) => selected.has(entry)) })
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
      testid="sources.gitea-editor-credential"
      @update:model-value="(credential: string) => update({ credential })"
    />
    <TextField
      v-else
      label="Account"
      :model-value="config.credential ?? ''"
      placeholder="gitea/git.example.com-octocat"
      :hint="credentialHint"
      monospace
      testid="sources.gitea-editor-credential"
      @update:model-value="(credential: string) => update({ credential })"
    />
    <SelectField
      label="Kind"
      :model-value="config.kind"
      :options="KIND_OPTIONS"
      testid="sources.gitea-editor-kind"
      @update:model-value="updateKind"
    />

    <template v-if="isSearch">
      <SelectField
        label="Items"
        :model-value="config.items ?? 'all'"
        :options="ITEM_OPTIONS"
        testid="sources.gitea-editor-items"
        @update:model-value="(items: string) => update({ items: items as Items })"
      />
      <SelectField
        label="State"
        :model-value="config.state ?? 'open'"
        :options="STATE_OPTIONS"
        testid="sources.gitea-editor-state"
        @update:model-value="(state: string) => update({ state: state as State })"
      />
      <FieldRow label="Involving me" :hint="involvingHint" testid="sources.gitea-editor-involving">
        <div class="flex flex-col gap-2">
          <ToggleField
            v-for="option in INVOLVING"
            :key="option"
            :label="INVOLVING_LABELS[option]"
            :model-value="involving.includes(option)"
            :testid="`sources.gitea-editor-involving-${option}`"
            @update:model-value="(on: boolean) => toggleInvolving(option, on)"
          />
        </div>
      </FieldRow>
      <TextField
        label="Owner"
        :model-value="config.owner ?? ''"
        placeholder="acme"
        hint="Limit the search to one user's or organization's repositories."
        monospace
        testid="sources.gitea-editor-owner"
        @update:model-value="(owner: string) => update({ owner: owner || undefined })"
      />
      <GlobListField
        label="Labels"
        :model-value="config.labels ?? []"
        placeholder="bug&#10;needs-review"
        hint="One label per line. An item carrying any of them matches."
        testid="sources.gitea-editor-labels"
        @update:model-value="(labels: string[]) => update({ labels: labels.length > 0 ? labels : undefined })"
      />
      <TextField
        label="Text"
        :model-value="config.text ?? ''"
        placeholder="checkout flow"
        hint="Free-text search over title and body."
        testid="sources.gitea-editor-text"
        @update:model-value="(text: string) => update({ text: text || undefined })"
      />
    </template>

    <NumberField
      label="Limit"
      :model-value="config.limit ?? 0"
      :placeholder="isSearch ? '50 (max 100)' : '50 (max 50)'"
      hint="Max items per fetch. 0 uses the default (50)."
      testid="sources.gitea-editor-limit"
      @update:model-value="(limit: number) => update({ limit: limit || undefined })"
    />
    <IntervalField
      :model-value="config.interval"
      testid="sources.gitea-editor-interval"
      @update:model-value="(interval?: string) => update({ interval })"
    />
  </div>
</template>
