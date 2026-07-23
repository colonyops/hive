<script setup lang="ts">
// A single on-disk location row inside a grouped System-settings card: a
// leading type icon, the label and monospace path on one line, and quiet
// icon-only actions. Copy is handled locally (no backend); open/reveal/
// change/reset are emitted for the parent to route through the SystemService.
// `editable` adds the Change… button and, when overridden, the Reset action
// used by the data and config directories.
import { computed } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconDatabase from '~icons/lucide/database'
import IconExternalLink from '~icons/lucide/external-link'
import IconFileText from '~icons/lucide/file-text'
import IconFolder from '~icons/lucide/folder'
import IconFolderOpen from '~icons/lucide/folder-open'
import IconRotateCcw from '~icons/lucide/rotate-ccw'
import { useClipboard } from '../../composables/useClipboard'
import BaseBadge from '../BaseBadge.vue'
import BaseIconBadge from '../BaseIconBadge.vue'

const props = withDefaults(
  defineProps<{
    label: string
    path: string
    hint?: string
    icon?: 'folder' | 'log' | 'database'
    tone?: 'accent' | 'neutral'
    exists?: boolean
    overridden?: boolean
    editable?: boolean
    testid?: string
  }>(),
  { icon: 'folder', tone: 'neutral', exists: true, overridden: false, editable: false },
)
const emit = defineEmits<{ open: []; reveal: []; change: []; reset: [] }>()

const { copy, status: copyStatus } = useClipboard()

const typeIcon = computed(() => ({ folder: IconFolder, log: IconFileText, database: IconDatabase })[props.icon])
const copyLabel = computed(() =>
  copyStatus.value === 'success' ? 'Copied' : copyStatus.value === 'error' ? 'Copy failed' : 'Copy path',
)

const iconBtnClass =
  'flex size-[30px] cursor-pointer items-center justify-center rounded-[7px] text-text-3 hover:bg-hover hover:text-text'
</script>

<template>
  <article
    class="flex items-center gap-3.5 px-4 py-3 transition-colors hover:bg-row-hover"
    :data-testid="props.testid"
  >
    <BaseIconBadge
      :size="32"
      rounded="rounded-lg"
      :class="props.tone === 'accent'
        ? 'border border-[rgba(245,158,11,0.35)] bg-[rgba(245,158,11,0.13)] text-accent'
        : 'border border-card bg-chip text-text-2'"
    >
      <component :is="typeIcon" class="size-4" />
    </BaseIconBadge>

    <div class="min-w-0 flex-1">
      <div class="flex flex-wrap items-baseline gap-x-2.5 gap-y-0.5">
        <span class="shrink-0 text-[13.5px] font-semibold text-text">{{ props.label }}</span>
        <BaseBadge
          v-if="props.overridden"
          variant="pill"
          class="shrink-0 px-2 py-0.5 text-[10.5px] font-semibold"
          :data-testid="props.testid ? `${props.testid}-overridden` : undefined"
        >Custom</BaseBadge>
        <BaseBadge
          v-if="!props.exists"
          tone="muted"
          variant="pill"
          class="shrink-0 px-2 py-0.5 text-[10.5px] font-medium"
        >Not created yet</BaseBadge>
        <span
          class="min-w-0 max-w-full truncate font-mono text-xs text-text-2"
          :title="props.path"
          :data-testid="props.testid ? `${props.testid}-path` : undefined"
        >{{ props.path }}</span>
      </div>
      <p v-if="props.hint" class="mt-0.5 text-[11.5px] text-text-3">{{ props.hint }}</p>
    </div>

    <div class="flex shrink-0 items-center gap-1">
      <button
        type="button"
        :class="iconBtnClass"
        :title="copyLabel"
        :aria-label="copyLabel"
        :data-testid="props.testid ? `${props.testid}-copy` : undefined"
        @click="copy(props.path)"
      ><component :is="copyStatus === 'success' ? IconCheck : IconCopy" class="size-[15px]" /></button>
      <button
        type="button"
        :class="iconBtnClass"
        title="Open"
        aria-label="Open"
        :data-testid="props.testid ? `${props.testid}-open` : undefined"
        @click="emit('open')"
      ><IconExternalLink class="size-[15px]" /></button>
      <button
        type="button"
        :class="iconBtnClass"
        title="Reveal"
        aria-label="Reveal"
        :data-testid="props.testid ? `${props.testid}-reveal` : undefined"
        @click="emit('reveal')"
      ><IconFolderOpen class="size-[15px]" /></button>

      <template v-if="props.editable">
        <button
          v-if="props.overridden"
          type="button"
          :class="iconBtnClass"
          title="Reset to default"
          aria-label="Reset to default"
          :data-testid="props.testid ? `${props.testid}-reset` : undefined"
          @click="emit('reset')"
        ><IconRotateCcw class="size-[15px]" /></button>
        <button
          type="button"
          class="ml-1 cursor-pointer rounded-[7px] border border-card px-3 py-1.5 text-[12.5px] font-medium text-text-2 hover:border-strong hover:text-text"
          :data-testid="props.testid ? `${props.testid}-change` : undefined"
          @click="emit('change')"
        >Change…</button>
      </template>
    </div>
  </article>
</template>
