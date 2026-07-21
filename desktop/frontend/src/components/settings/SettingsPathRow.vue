<script setup lang="ts">
// A single on-disk location row for the System settings screen: a label, the
// path in monospace, and action buttons. Copy is handled locally (no backend);
// open/reveal/change/reset are emitted for the parent to route through the
// SystemService. `editable` adds the Change…/Reset controls used by the data
// and config directories.
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconExternalLink from '~icons/lucide/external-link'
import IconFolder from '~icons/lucide/folder'
import IconFolderOpen from '~icons/lucide/folder-open'
import IconRotateCcw from '~icons/lucide/rotate-ccw'
import { useClipboard } from '../../composables/useClipboard'
import BaseBadge from '../BaseBadge.vue'

const props = withDefaults(
  defineProps<{
    label: string
    path: string
    hint?: string
    exists?: boolean
    overridden?: boolean
    editable?: boolean
    testid?: string
  }>(),
  { exists: true, overridden: false, editable: false },
)
const emit = defineEmits<{ open: []; reveal: []; change: []; reset: [] }>()

const { copy, status: copyStatus } = useClipboard()

const btnClass =
  'flex cursor-pointer items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-[12px] font-medium text-text-2 hover:bg-chip hover:text-text'
</script>

<template>
  <article class="rounded-lg border border-border bg-raised p-4" :data-testid="props.testid">
    <div class="flex items-center gap-2">
      <span class="text-[13.5px] font-semibold text-text">{{ props.label }}</span>
      <BaseBadge
        v-if="props.overridden"
        variant="pill"
        class="px-2 py-0.5 text-[10.5px] font-semibold"
        :data-testid="props.testid ? `${props.testid}-overridden` : undefined"
      >Custom</BaseBadge>
      <BaseBadge
        v-if="!props.exists"
        tone="muted"
        variant="pill"
        class="px-2 py-0.5 text-[10.5px] font-medium"
      >Not created yet</BaseBadge>
    </div>

    <div
      class="mt-1.5 truncate font-mono text-[12px] text-text-2"
      :title="props.path"
      :data-testid="props.testid ? `${props.testid}-path` : undefined"
    >{{ props.path }}</div>

    <p v-if="props.hint" class="mt-1 text-xs leading-relaxed text-text-4">{{ props.hint }}</p>

    <div class="mt-3 flex flex-wrap items-center gap-2">
      <button
        type="button"
        :class="btnClass"
        :data-testid="props.testid ? `${props.testid}-copy` : undefined"
        @click="copy(props.path)"
      >
        <component :is="copyStatus === 'success' ? IconCheck : IconCopy" class="size-3.5" />
        {{ copyStatus === 'success' ? 'Copied' : copyStatus === 'error' ? 'Copy failed' : 'Copy path' }}
      </button>
      <button
        type="button"
        :class="btnClass"
        :data-testid="props.testid ? `${props.testid}-open` : undefined"
        @click="emit('open')"
      ><IconExternalLink class="size-3.5" />Open</button>
      <button
        type="button"
        :class="btnClass"
        :data-testid="props.testid ? `${props.testid}-reveal` : undefined"
        @click="emit('reveal')"
      ><IconFolderOpen class="size-3.5" />Reveal</button>

      <template v-if="props.editable">
        <div class="flex-1" />
        <button
          type="button"
          :class="btnClass"
          :data-testid="props.testid ? `${props.testid}-change` : undefined"
          @click="emit('change')"
        ><IconFolder class="size-3.5" />Change…</button>
        <button
          v-if="props.overridden"
          type="button"
          :class="btnClass"
          :data-testid="props.testid ? `${props.testid}-reset` : undefined"
          @click="emit('reset')"
        ><IconRotateCcw class="size-3.5" />Reset</button>
      </template>
    </div>
  </article>
</template>
