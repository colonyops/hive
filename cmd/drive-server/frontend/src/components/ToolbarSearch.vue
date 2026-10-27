<script setup lang="ts">
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Kbd from '@hive/ui/components/ui/Kbd.vue'
import SearchField from '@hive/ui/components/ui/SearchField.vue'
import SegmentedControl from '@hive/ui/components/ui/SegmentedControl.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import { useListKeyboardNav } from '@hive/ui/composables/useListKeyboardNav'
import { useEventListener } from '@vueuse/core'
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useSearch } from '../lib/search'
import { documentRoute, spaceFromKey, type SpaceRef } from '../lib/space'
import { documentLabel } from '../lib/tree'
import type { SearchHit } from '../types'
import SearchSnippet from './SearchSnippet.vue'

const props = defineProps<{ space: SpaceRef | null }>()
const query = defineModel<string>({ required: true })

const router = useRouter()
const field = ref<{ focus(): void; select(): void } | null>(null)
const root = ref<HTMLElement | null>(null)
const focused = ref(false)
const allSpaces = ref(false)
const active = ref(0)

const scopeChoice = computed({
  get: () => (allSpaces.value ? 'all' : 'space'),
  set: (value: 'space' | 'all') => {
    allSpaces.value = value === 'all'
  },
})
const scopeOptions = [
  { value: 'space' as const, label: 'This space' },
  { value: 'all' as const, label: 'All spaces' },
]

const { hits, loading, error, unavailable } = useSearch(
  () => query.value,
  () => (props.space && !allSpaces.value ? props.space.key : undefined),
)
const open = computed(() => focused.value && query.value.trim() !== '')

const nav = useListKeyboardNav({
  active,
  count: () => hits.value.length,
  row: (index) => root.value?.querySelector(`[data-hit-index="${index}"]`),
})
watch(hits, () => {
  active.value = 0
})

function openHit(hit: SearchHit): void {
  void router.push(documentRoute(spaceFromKey(hit.document.space), hit.document.path))
  query.value = ''
  if (document.activeElement instanceof HTMLElement) document.activeElement.blur()
}

function onKeydown(event: KeyboardEvent): void {
  if (!open.value) return
  if (nav.onKeydown(event)) return
  const hit = hits.value[active.value]
  if (event.key === 'Enter' && hit) {
    event.preventDefault()
    openHit(hit)
  }
}

function onFocusOut(event: FocusEvent): void {
  if (!root.value?.contains(event.relatedTarget as Node | null)) focused.value = false
}

function isTyping(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLElement && (target.isContentEditable || /^(input|textarea|select)$/i.test(target.tagName))
  )
}

// "/" is left alone while the user types in another field.
useEventListener(window, 'keydown', (event: KeyboardEvent) => {
  const shortcut = (event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k'
  const slash = event.key === '/' && !event.metaKey && !event.ctrlKey && !event.altKey && !isTyping(event.target)
  if (!shortcut && !slash) return
  event.preventDefault()
  field.value?.focus()
  field.value?.select()
})
</script>

<template>
  <div ref="root" class="relative w-full max-w-3xl" @focusin="focused = true" @focusout="onFocusOut">
    <SearchField
      ref="field"
      v-model="query"
      placeholder="Search documents"
      aria-label="Search documents"
      class="drive-search h-12 rounded-full! border-transparent! bg-chip! px-4! focus-within:bg-raised! focus-within:shadow-popover"
      testid="search-input"
      @keydown="onKeydown"
    />
    <Kbd
      v-if="!query && !focused"
      variant="boxed"
      class="pointer-events-none absolute right-4 hidden md:block top-1/2 -translate-y-1/2"
      aria-hidden="true"
      >/</Kbd
    >
    <div
      v-if="open"
      class="absolute inset-x-0 top-full z-20 mt-2 overflow-hidden rounded-2xl border border-card bg-raised shadow-popover"
      data-testid="search-results"
      @mousedown.prevent
    >
      <div v-if="space" class="flex items-center justify-between gap-2 border-b border-row px-4 py-2.5">
        <span class="min-w-0 truncate text-small text-text-3">
          Searching {{ allSpaces ? 'all spaces' : space.title }}
        </span>
        <SegmentedControl
          v-model="scopeChoice"
          variant="compact"
          size="sm"
          aria-label="Search scope"
          :options="scopeOptions"
          testid="search-scope"
        />
      </div>
      <ul v-if="hits.length" class="max-h-[28rem] overflow-y-auto py-1.5" role="listbox" aria-label="Search results">
        <li
          v-for="(hit, index) in hits"
          :key="hit.document.id"
          role="option"
          :aria-selected="index === active"
          :data-hit-index="index"
          class="flex cursor-pointer flex-col gap-0.5 px-4 py-2.5"
          :class="index === active ? 'bg-chip' : ''"
          data-testid="search-hit"
          @mousemove="active = index"
          @click="openHit(hit)"
        >
          <span class="truncate text-body font-medium text-text">{{ documentLabel(hit.document) }}</span>
          <span class="truncate text-small text-text-4">
            {{ spaceFromKey(hit.document.space).title }} / {{ hit.document.path }}
          </span>
          <span v-if="hit.snippet" class="line-clamp-2 text-small leading-snug text-text-3">
            <SearchSnippet :snippet="hit.snippet" />
          </span>
        </li>
      </ul>
      <div v-else class="px-4 py-4">
        <InlineError v-if="error" variant="line" :message="error" testid="search-error" />
        <EmptyState
          v-else-if="unavailable"
          variant="inline"
          message="This server has no content search yet."
          data-testid="search-unavailable"
        />
        <span v-else-if="loading" class="flex items-center gap-2 text-body text-text-3">
          <Spinner />
          Searching
        </span>
        <EmptyState v-else variant="inline" message="No matching documents." data-testid="search-empty" />
      </div>
    </div>
  </div>
</template>
