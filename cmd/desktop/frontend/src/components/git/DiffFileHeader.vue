<script setup lang="ts">
import { computed } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconChevronRight from '~icons/lucide/chevron-right'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import { documentIcon } from '../drive/driveIcon'
import type { DiffEntry } from '../../composables/useFileDiffs'
import { diffHasText } from '../../lib/diffItems'
import { gitFileName, gitStatus } from '../../lib/gitTree'
import type { ChangedFile } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  file: ChangedFile
  entry: DiffEntry | undefined
  open: boolean
  viewed: boolean
  current: boolean
}>()
const emit = defineEmits<{ fold: []; 'toggle-viewed': []; 'open-editor': [] }>()

const status = computed(() => gitStatus(props.file.status))
const fileIcon = computed(() => documentIcon(gitFileName(props.file.path), ''))
const diff = computed(() => props.entry?.diff)
const note = computed(() => {
  const d = diff.value
  if (!props.open || !d || diffHasText(d)) return null
  if (d.error) return d.error
  if (d.binary) return 'Binary file'
  if (d.truncated) return 'Diff too large to show'
  if (d.unavailable) return 'A symlink, pipe or nested repository'
  if (d.gone) return 'No longer changed'
  return 'No content changes'
})
</script>

<template>
  <!-- Exactly DIFF_METRICS.headerPx tall: CodeView lays out every file from that height. -->
  <div
    class="flex h-8 cursor-pointer items-center gap-2 border-b border-border bg-raised px-2.5"
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
    <component :is="fileIcon.icon" class="size-3.5 shrink-0" :class="fileIcon.color" aria-hidden="true" />
    <span
      class="w-3 shrink-0 text-center font-mono text-body font-semibold"
      :class="status.color"
      :title="status.label"
      >{{ status.letter }}</span
    >
    <span class="min-w-0 flex-1 truncate font-mono text-body text-text" :title="file.path">
      <span v-if="file.oldPath" class="text-text-4">{{ file.oldPath }} -&gt; </span>{{ file.path }}
    </span>
    <span
      v-if="open && !entry"
      class="shrink-0 text-caption text-text-4"
      :data-testid="`changes-file-${file.path}-loading`"
      >Loading…</span
    >
    <span
      v-if="note"
      class="max-w-[50%] shrink truncate text-caption"
      :class="diff?.error ? 'text-severity-error' : 'text-text-3'"
      :title="note"
      :data-testid="`changes-file-${file.path}-note`"
      >{{ note }}</span
    >
    <BaseButton v-if="open && diff?.truncated" variant="secondary" size="xs" @click.stop="emit('open-editor')">
      Open in editor
    </BaseButton>
    <span v-if="!file.binary" class="shrink-0 font-mono text-body">
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
</template>

<style scoped>
.changes-file-current {
  box-shadow: inset 2px 0 0 var(--color-accent);
}
</style>
