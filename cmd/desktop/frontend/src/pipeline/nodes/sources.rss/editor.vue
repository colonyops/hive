<script setup lang="ts">
// sources.rss has no runtime.ts: the fetch and the parse run in Go, on the poll tick.
import { computed } from 'vue'
import { defaultRssSourceIcon, feedIconComponent, feedIconOptions } from '../../../lib/feedIcons'
import { IntervalField, MarkImageField, NumberField, SelectField, TextField, type MarkImageClient } from '../../fields'
import { DEFAULT_LIMIT, MAX_LIMIT, type Config } from './config'

const props = defineProps<{
  config: Config
  errors?: string[]
  /** The mark picker's backend seam, forwarded to MarkImageField for tests. */
  client?: MarkImageClient
}>()
const emit = defineEmits<{ 'update:config': [config: Config] }>()

const iconOptions = feedIconOptions.map((o) => ({ value: o.value, label: o.label, icon: o.component }))

function update(patch: Partial<Config>) {
  emit('update:config', { ...props.config, ...patch })
}

// Zero is how an emptied number input reads; it means "use the default", which
// the config expresses by omitting the key rather than writing `limit: 0`.
function updateLimit(limit: number) {
  update({ limit: limit > 0 ? limit : undefined })
}

// The mark picker's fallback preview, and what items render with no image set.
const iconGlyph = computed(() => feedIconComponent(props.config.icon || defaultRssSourceIcon))
</script>

<template>
  <div class="flex flex-col gap-4">
    <TextField
      label="Feed URL"
      :model-value="config.url ?? ''"
      placeholder="https://example.com/feed.xml"
      hint="RSS, Atom or JSON Feed. It must be readable without credentials, since this node sends no token."
      monospace
      testid="sources.rss-editor-url"
      @update:model-value="(url: string) => update({ url })"
    />
    <NumberField
      label="Entry limit"
      :model-value="config.limit ?? 0"
      :min="0"
      :max="MAX_LIMIT"
      :placeholder="String(DEFAULT_LIMIT)"
      :hint="`How many of the feed's newest entries to ingest per fetch. Empty is ${DEFAULT_LIMIT}; the first fetch ingests everything still in the window.`"
      testid="sources.rss-editor-limit"
      @update:model-value="updateLimit"
    />
    <IntervalField
      :model-value="config.interval"
      testid="sources.rss-editor-interval"
      @update:model-value="(interval?: string) => update({ interval })"
    />
    <SelectField
      label="Item icon"
      :model-value="config.icon || defaultRssSourceIcon"
      :options="iconOptions"
      searchable
      search-placeholder="Search icons…"
      hint="Shown on this source's items when no image is set."
      testid="sources.rss-editor-icon"
      @update:model-value="(icon: string) => update({ icon: icon || undefined })"
    />
    <MarkImageField
      :model-value="config.image"
      :glyph="iconGlyph"
      :client="props.client"
      testid="sources.rss-editor-mark"
      @update:model-value="(image: string | undefined) => update({ image })"
    />
  </div>
</template>
