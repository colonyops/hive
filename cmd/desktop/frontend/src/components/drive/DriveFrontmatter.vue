<script setup lang="ts">
import { computed } from 'vue'
import IconChevronDown from '~icons/lucide/chevron-down'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import type { FrontmatterScalar, FrontmatterValue } from '../../lib/drive'
import { useCanvasSettings } from '../../stores/useCanvasSettings'

const props = defineProps<{
  frontmatter: Record<string, FrontmatterValue>
  /** Unix milliseconds; 0 where the file system records no birth time. */
  createdAt: number
  updatedAt: number
  testid: string
}>()

const { frontmatterExpanded, toggleFrontmatter } = useCanvasSettings()

type Entry = {
  key: string
  label: string
  value: FrontmatterValue
}

function labelFor(key: string): string {
  if (key === 'created_at') return 'Created'
  if (key === 'updated_at') return 'Updated'
  const words = key.replace(/[._-]+/g, ' ')
  return words.charAt(0).toUpperCase() + words.slice(1)
}

const entries = computed<Entry[]>(() =>
  Object.entries(props.frontmatter)
    .sort(([left], [right]) => {
      if (left === 'tags') return -1
      if (right === 'tags') return 1
      return left.localeCompare(right)
    })
    .map(([key, value]) => ({ key, label: labelFor(key), value })),
)

function formatTimestamp(value: number): string {
  const date = new Date(value)
  const options: Intl.DateTimeFormatOptions = {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }
  if (date.getFullYear() !== new Date().getFullYear()) options.year = 'numeric'
  return date.toLocaleString([], options)
}

function formatScalar(value: FrontmatterScalar): string {
  if (value === null) return 'None'
  if (typeof value === 'boolean') return value ? 'Yes' : 'No'
  return String(value)
}

// Markdown front matter can nest; a canvas's never does.
function formatValue(value: FrontmatterValue): string {
  if (Array.isArray(value)) return value.map(formatScalar).join(', ') || 'None'
  if (value !== null && typeof value === 'object') return JSON.stringify(value)
  return formatScalar(value)
}
</script>

<template>
  <div class="mb-3" :data-testid="`${testid}-frontmatter`">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <button
        type="button"
        class="flex cursor-pointer items-center gap-1 rounded-md px-1.5 py-1 text-caption font-semibold text-text-2 hover:bg-chip"
        :aria-expanded="frontmatterExpanded"
        :aria-label="frontmatterExpanded ? 'Collapse properties' : 'Expand properties'"
        :data-testid="`${testid}-frontmatter-toggle`"
        @click="toggleFrontmatter"
      >
        <IconChevronDown
          class="size-3.5 text-text-3 transition-transform"
          :class="frontmatterExpanded ? '' : '-rotate-90'"
          aria-hidden="true"
        />
        Properties
      </button>

      <dl v-if="frontmatterExpanded" class="ml-auto flex min-w-0 flex-wrap items-baseline justify-end gap-x-4 gap-y-1">
        <div
          v-if="createdAt > 0"
          class="inline-flex items-baseline gap-1.5"
          :data-testid="`${testid}-frontmatter-created_at`"
        >
          <dt class="text-micro font-medium uppercase tracking-wide text-text-4">Created</dt>
          <dd>
            <time
              class="font-mono text-caption text-text-2"
              :datetime="new Date(createdAt).toISOString()"
              :title="new Date(createdAt).toLocaleString()"
            >
              {{ formatTimestamp(createdAt) }}
            </time>
          </dd>
        </div>
        <div
          v-if="updatedAt > 0"
          class="inline-flex items-baseline gap-1.5"
          :data-testid="`${testid}-frontmatter-updated_at`"
        >
          <dt class="text-micro font-medium uppercase tracking-wide text-text-4">Updated</dt>
          <dd>
            <time
              class="font-mono text-caption text-text-2"
              :datetime="new Date(updatedAt).toISOString()"
              :title="new Date(updatedAt).toLocaleString()"
            >
              {{ formatTimestamp(updatedAt) }}
            </time>
          </dd>
        </div>
      </dl>
    </div>

    <dl
      v-if="frontmatterExpanded && entries.length"
      class="mt-2 overflow-hidden rounded-lg border border-border bg-pane text-small"
    >
      <div
        v-for="entry in entries"
        :key="entry.key"
        class="grid grid-cols-[minmax(90px,0.35fr)_minmax(0,1fr)] gap-3 border-b border-border px-3 py-2 last:border-b-0"
        :data-testid="`${testid}-frontmatter-${entry.key}`"
      >
        <dt class="font-mono text-caption font-medium text-text-4">{{ entry.label }}</dt>
        <dd class="min-w-0 text-text-2">
          <div v-if="entry.key === 'tags' && Array.isArray(entry.value)" class="flex flex-wrap gap-1.5">
            <BaseBadge
              v-for="(value, index) in entry.value"
              :key="index"
              variant="chip"
              class="border border-row px-2 py-0.5 text-caption !text-text-2"
            >
              {{ formatScalar(value) }}
            </BaseBadge>
            <span v-if="!entry.value.length" class="text-text-4">None</span>
          </div>
          <span v-else class="break-words">{{ formatValue(entry.value) }}</span>
        </dd>
      </div>
    </dl>
  </div>
</template>
