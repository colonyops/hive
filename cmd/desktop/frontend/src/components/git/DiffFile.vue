<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconChevronRight from '~icons/lucide/chevron-right'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import DiffHunks from './DiffHunks.vue'
import type { DiffEntry } from '../../composables/useFileDiffs'
import { gitStatus } from '../../lib/gitTree'
import type { ChangedFile } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  file: ChangedFile
  entry: DiffEntry | undefined
  open: boolean
  viewed: boolean
  current: boolean
  /** Near the viewport. Away from it the body keeps its height and drops its rows. */
  near: boolean
  mode: 'unified' | 'split'
}>()
const emit = defineEmits<{ fold: []; 'toggle-viewed': []; 'open-editor': [] }>()

const ROW_PX = 18
const OVERHEAD_PX = 54
const MAX_ESTIMATE_PX = 8000

const status = computed(() => gitStatus(props.file.status))
const diff = computed(() => props.entry?.diff)
const hasText = computed(() => {
  const d = diff.value
  return !!d?.text && !d.binary && !d.truncated && !d.unavailable && !d.gone && !d.error
})

// Close to the real height before the diff arrives, so the scrollbar and the
// jump to a far file don't shift much as files load.
const estimate = computed(() =>
  Math.min((props.file.additions + props.file.deletions) * ROW_PX + OVERHEAD_PX, MAX_ESTIMATE_PX),
)

const body = ref<HTMLElement | null>(null)
const measured = ref<number | null>(null)
watch(
  () => props.near,
  (near) => {
    if (!near && body.value) measured.value = body.value.offsetHeight
  },
)
const showRows = computed(() => hasText.value && props.near)
const placeholderHeight = computed(() => `${measured.value ?? estimate.value}px`)
</script>

<template>
  <section class="border-b border-border" :data-file="file.path" :data-testid="`changes-file-${file.path}`">
    <div
      class="sticky top-0 z-[1] flex min-h-8 cursor-pointer items-center gap-2 border-b border-border bg-raised px-2.5 py-0.5"
      :class="current ? 'changes-file-current' : ''"
      role="button"
      tabindex="-1"
      :aria-expanded="open"
      :data-testid="`changes-file-${file.path}-header`"
      @click="emit('fold')"
    >
      <IconChevronRight
        class="size-3 shrink-0 text-text-4 transition-transform"
        :class="open ? 'rotate-90' : ''"
        aria-hidden="true"
      />
      <span
        class="w-3 shrink-0 text-center font-mono text-caption font-semibold"
        :class="status.color"
        :title="status.label"
        >{{ status.letter }}</span
      >
      <span class="min-w-0 flex-1 truncate font-mono text-caption text-text" :title="file.path">
        <span v-if="file.oldPath" class="text-text-4">{{ file.oldPath }} -&gt; </span>{{ file.path }}
      </span>
      <span v-if="!file.binary" class="shrink-0 font-mono text-caption">
        <span v-if="file.additions" class="text-severity-success">+{{ file.additions }}</span>
        <span v-if="file.deletions" class="ml-1.5 text-severity-error">-{{ file.deletions }}</span>
      </span>
      <button
        type="button"
        class="inline-flex h-5 shrink-0 cursor-pointer items-center gap-1 rounded-md border px-1.5 text-caption"
        :class="
          viewed
            ? 'border-transparent bg-severity-success-tint text-severity-success'
            : 'border-strong text-text-2 hover:text-text'
        "
        :aria-pressed="viewed"
        :data-testid="`changes-file-${file.path}-viewed`"
        @click.stop="emit('toggle-viewed')"
      >
        <IconCheck v-if="viewed" class="size-3" aria-hidden="true" />
        Viewed
      </button>
    </div>
    <template v-if="open">
      <div v-if="hasText" ref="body" class="overflow-x-auto" :style="near ? undefined : { height: placeholderHeight }">
        <DiffHunks v-if="showRows" :path="file.path" :text="diff!.text" :mode="mode" />
      </div>
      <div
        v-else-if="!entry"
        class="px-3 py-2 text-xs text-text-4"
        :style="{ height: `${estimate}px` }"
        :data-testid="`changes-file-${file.path}-loading`"
      >
        Loading…
      </div>
      <InlineError v-else-if="diff?.error" class="m-2" :message="diff.error" testid="changes-file-error" />
      <div v-else class="px-3 py-3 text-xs text-text-3" :data-testid="`changes-file-${file.path}-note`">
        <template v-if="diff?.binary">Binary file. Hive shows no diff for binary files.</template>
        <div v-else-if="diff?.truncated" class="flex flex-col items-start gap-2">
          This diff is too large to show here.
          <BaseButton variant="secondary" size="xs" @click="emit('open-editor')">Open in editor</BaseButton>
        </div>
        <template v-else-if="diff?.unavailable"
          >No text to show: this is a symlink, a pipe or a nested repository.</template
        >
        <template v-else-if="diff?.gone">This file is no longer changed.</template>
        <template v-else>No content changes.</template>
      </div>
    </template>
  </section>
</template>

<style scoped>
.changes-file-current {
  box-shadow: inset 2px 0 0 var(--color-accent);
}
</style>
