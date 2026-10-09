<script setup lang="ts">
import { computed, ref } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconSparkles from '~icons/lucide/sparkles'
import type { Entry as DiagnosticEntry } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/diagnostics/models'
import BaseBadge from './ui/BaseBadge.vue'
import IconButton from './ui/IconButton.vue'

const props = defineProps<{
  entry: DiagnosticEntry
  wrapMessages: boolean
  copied: boolean
  actionsDisabled: boolean
}>()
const emit = defineEmits<{ copy: []; investigate: [] }>()
const expanded = ref(false)

const fields = computed<[string, string][]>(() =>
  Object.entries(props.entry.fields ?? {})
    .filter((field): field is [string, string] => typeof field[1] === 'string')
    .sort(([left], [right]) => left.localeCompare(right)),
)

function fullTime(value: string): string {
  return value ? new Date(value).toLocaleString() : 'No timestamp'
}

function shortTime(value: string): string {
  if (!value) return '--:--:--'
  const date = new Date(value)
  return date.toLocaleTimeString([], { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function sourceLabel(id: string): string {
  if (id === 'desktop') return 'DESKTOP'
  if (id === 'cli') return 'CLI'
  if (id === 'jobs') return 'JOB'
  return id.toUpperCase()
}

function levelLabel(value: string): string {
  return { error: 'ERR', warn: 'WRN', info: 'INF', debug: 'DBG', unknown: 'UNK' }[value] ?? value.toUpperCase()
}

function onToggle(event: Event): void {
  expanded.value = (event.currentTarget as HTMLDetailsElement).open
}
</script>

<template>
  <details
    :id="entry.id"
    class="group border-b border-l-2 border-row font-mono text-caption"
    :class="{
      'border-l-severity-error': entry.level === 'error',
      'border-l-severity-warning': entry.level === 'warn',
      'border-l-severity-success': entry.level === 'info',
      'border-l-text-4': entry.level === 'debug' || entry.level === 'unknown',
    }"
    data-testid="diagnostics-entry"
    :data-entry-id="entry.id"
    @toggle="onToggle"
  >
    <summary class="flex cursor-pointer list-none items-start gap-2 px-2 py-1.5 hover:bg-hover">
      <time class="w-17 shrink-0 text-text-4" :title="fullTime(entry.time)">{{ shortTime(entry.time) }}</time>
      <span class="w-13 shrink-0 text-text-3">{{ sourceLabel(entry.source) }}</span>
      <span
        class="w-8 shrink-0 font-semibold"
        :class="{
          'text-severity-error': entry.level === 'error',
          'text-severity-warning': entry.level === 'warn',
          'text-text-3': entry.level !== 'error' && entry.level !== 'warn',
        }"
      >
        {{ levelLabel(entry.level) }}
      </span>
      <span class="min-w-0 flex-1 text-text-2" :class="wrapMessages ? 'whitespace-pre-wrap break-words' : 'truncate'">
        {{ entry.message }}
      </span>
    </summary>
    <div v-if="expanded" class="border-t border-row bg-raised px-3 py-2.5" data-testid="diagnostics-entry-raw">
      <div class="mb-2 flex flex-wrap items-center gap-2">
        <a class="text-accent hover:underline" :href="`#${entry.id}`">{{ entry.id }}</a>
        <BaseBadge v-if="entry.truncated" tone="accent" variant="pill" class="px-2 py-0.5 text-micro">
          Entry truncated
        </BaseBadge>
        <IconButton
          class="ml-auto"
          :label="copied ? 'Entry copied' : 'Copy entry'"
          :icon="copied ? IconCheck : IconCopy"
          :active="copied"
          size="md"
          data-testid="diagnostics-entry-copy"
          :data-entry-id="entry.id"
          @click="emit('copy')"
        />
        <IconButton
          label="Investigate this entry"
          :icon="IconSparkles"
          size="md"
          :disabled="actionsDisabled"
          data-testid="diagnostics-entry-investigate"
          :data-entry-id="entry.id"
          @click="emit('investigate')"
        />
      </div>
      <dl v-if="fields.length" class="mb-2 grid grid-cols-[max-content_minmax(0,1fr)] gap-x-3 gap-y-0.5">
        <template v-for="[key, value] in fields" :key="key">
          <dt class="text-text-4">{{ key }}</dt>
          <dd class="break-all text-text-2">{{ value }}</dd>
        </template>
      </dl>
      <pre class="whitespace-pre-wrap break-words text-text-3">{{ entry.raw }}</pre>
    </div>
  </details>
</template>
