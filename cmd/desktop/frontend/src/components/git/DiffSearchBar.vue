<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronUp from '~icons/lucide/chevron-up'
import IconX from '~icons/lucide/x'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import SearchField from '@hive/ui/components/ui/SearchField.vue'
import { useEscapeToClose } from '@hive/ui/composables/useEscapeToClose'
import { MAX_SEARCH_MATCHES } from '../../lib/diffReview'

const props = defineProps<{
  count: number
  index: number
  /** How many files have been read, and so searched, out of `total`. */
  searched: number
  total: number
}>()
const emit = defineEmits<{ step: [delta: 1 | -1]; close: [] }>()

const query = defineModel<string>({ required: true })
const field = ref<{ focus: () => void } | null>(null)

useEscapeToClose(() => emit('close'))
onMounted(() => field.value?.focus())
defineExpose({ focus: () => field.value?.focus() })

const position = computed(() => {
  if (!query.value.trim()) return ''
  if (!props.count) return 'No matches'
  const total = props.count >= MAX_SEARCH_MATCHES ? `${MAX_SEARCH_MATCHES}+` : props.count
  return `${props.index + 1} of ${total}`
})

function onKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Enter') return
  event.preventDefault()
  emit('step', event.shiftKey ? -1 : 1)
}
</script>

<template>
  <div class="flex shrink-0 items-center gap-2 border-b border-border bg-pane px-2.5 py-1" data-testid="changes-search">
    <SearchField
      ref="field"
      v-model="query"
      class="min-w-0 flex-1"
      variant="bar"
      placeholder="Search the diffs…"
      aria-label="Search the diffs"
      testid="changes-search-input"
      @keydown="onKeydown"
    />
    <span
      v-if="searched < total"
      class="shrink-0 text-caption text-text-4"
      :title="`Search reads the ${searched} diffs loaded so far. Scroll to load more.`"
      data-testid="changes-search-partial"
      >{{ searched }}/{{ total }} files</span
    >
    <span class="shrink-0 font-mono text-caption text-text-3" data-testid="changes-search-count">{{ position }}</span>
    <IconButton
      label="Previous match"
      :icon="IconChevronUp"
      size="sm"
      :disabled="!count"
      data-testid="changes-search-prev"
      @click="emit('step', -1)"
    />
    <IconButton
      label="Next match"
      :icon="IconChevronDown"
      size="sm"
      :disabled="!count"
      data-testid="changes-search-next"
      @click="emit('step', 1)"
    />
    <IconButton
      label="Close search"
      :icon="IconX"
      size="sm"
      data-testid="changes-search-close"
      @click="emit('close')"
    />
  </div>
</template>
