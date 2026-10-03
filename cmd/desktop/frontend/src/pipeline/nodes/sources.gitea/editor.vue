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
import FormField from '../../../components/ui/FormField.vue'
import {
  CredentialField,
  GlobListField,
  IntervalField,
  NumberField,
  SelectField,
  TextField,
  ToggleField,
  type SelectOption,
} from '../../fields'
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

const involving = computed<Involving[]>(() => props.config.involving ?? [])

// One request per entry, so the cost of a union is worth stating where it is
// chosen rather than only in the node docs.
const involvingHint = computed(() => {
  const count = involving.value.length
  if (count === 0) return 'Every item the token can see. One request per poll.'
  if (count === 1) return 'Items matching this relationship. One request per poll.'
  return `Items matching any of these. ${count} requests per poll — Gitea cannot combine them.`
})

function update<K extends keyof Config>(key: K, value: Config[K]) {
  emit('update:config', { ...props.config, [key]: value })
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
  update(
    'involving',
    INVOLVING.filter((entry) => selected.has(entry)),
  )
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <CredentialField
      provider="gitea"
      :model-value="config.credential"
      hint="The connected Gitea or Forgejo account to fetch as."
      testid="sources.gitea-editor-credential"
      @update:model-value="update('credential', $event)"
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
        @update:model-value="update('items', $event as Items)"
      />
      <SelectField
        label="State"
        :model-value="config.state ?? 'open'"
        :options="STATE_OPTIONS"
        testid="sources.gitea-editor-state"
        @update:model-value="update('state', $event as State)"
      />
      <FormField label="Involving me" :hint="involvingHint" testid="sources.gitea-editor-involving">
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
      </FormField>
      <TextField
        label="Owner"
        :model-value="config.owner ?? ''"
        placeholder="acme"
        hint="Limit the search to one user's or organization's repositories."
        monospace
        testid="sources.gitea-editor-owner"
        @update:model-value="update('owner', $event || undefined)"
      />
      <GlobListField
        label="Labels"
        :model-value="config.labels ?? []"
        placeholder="bug&#10;needs-review"
        hint="One label per line. An item carrying any of them matches."
        testid="sources.gitea-editor-labels"
        @update:model-value="update('labels', $event.length > 0 ? $event : undefined)"
      />
      <TextField
        label="Text"
        :model-value="config.text ?? ''"
        placeholder="checkout flow"
        hint="Free-text search over title and body."
        testid="sources.gitea-editor-text"
        @update:model-value="update('text', $event || undefined)"
      />
    </template>

    <NumberField
      label="Limit"
      :model-value="config.limit ?? 0"
      :placeholder="isSearch ? '50 (max 100)' : '50 (max 50)'"
      hint="Max items per fetch. 0 uses the default (50)."
      testid="sources.gitea-editor-limit"
      @update:model-value="update('limit', $event || undefined)"
    />
    <IntervalField
      :model-value="config.interval"
      testid="sources.gitea-editor-interval"
      @update:model-value="update('interval', $event)"
    />
  </div>
</template>
