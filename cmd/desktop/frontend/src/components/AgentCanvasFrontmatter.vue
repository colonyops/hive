<script setup lang="ts">
import { computed } from 'vue'
import BaseBadge from './ui/BaseBadge.vue'
import type { CanvasFrontmatterScalar, CanvasFrontmatterValue } from '../lib/agentWorkspacesClient'

const props = defineProps<{
  frontmatter: Record<string, CanvasFrontmatterValue>
  createdAt: number
  updatedAt: number
  testid: string
}>()

type Entry = {
  key: string
  label: string
  value: CanvasFrontmatterValue
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

function formatScalar(value: CanvasFrontmatterScalar): string {
  if (value === null) return 'None'
  if (typeof value === 'boolean') return value ? 'Yes' : 'No'
  return String(value)
}

function formatValue(value: CanvasFrontmatterValue): string {
  return Array.isArray(value) ? value.map(formatScalar).join(', ') || 'None' : formatScalar(value)
}
</script>

<template>
  <div class="mb-3" :data-testid="`${testid}-frontmatter`">
    <div class="flex flex-wrap items-center justify-end gap-1.5">
      <BaseBadge tone="muted" class="px-2 py-0.5 text-micro" :data-testid="`${testid}-frontmatter-created_at`">
        <span>Created</span>
        <time
          class="font-mono"
          :datetime="new Date(createdAt).toISOString()"
          :title="new Date(createdAt).toLocaleString()"
        >
          {{ formatTimestamp(createdAt) }}
        </time>
      </BaseBadge>
      <BaseBadge tone="muted" class="px-2 py-0.5 text-micro" :data-testid="`${testid}-frontmatter-updated_at`">
        <span>Updated</span>
        <time
          class="font-mono"
          :datetime="new Date(updatedAt).toISOString()"
          :title="new Date(updatedAt).toLocaleString()"
        >
          {{ formatTimestamp(updatedAt) }}
        </time>
      </BaseBadge>
    </div>

    <dl v-if="entries.length" class="mt-2 overflow-hidden rounded-lg border border-border bg-pane text-small">
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
              class="px-2 py-0.5 text-caption"
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
