<script setup lang="ts">
// Presentation only: the Agents area reaches its backend over the loopback
// HTTP client and the terminal view over Wails bindings, so a component that
// fetched anything could serve only one of them.
import IconCode from '~icons/lucide/code'
import IconFolder from '~icons/lucide/folder'
import IconFolderOpen from '~icons/lucide/folder-open'
import AppTooltip from './AppTooltip.vue'

withDefaults(defineProps<{
  label?: string
  path?: string
  error?: string
  editorTitle?: string
  /** Prefix for this bar's test ids, so each area keeps its own. */
  testid: string
}>(), { label: '', path: '', error: '', editorTitle: '' })

defineEmits<{ 'open-editor': []; reveal: [] }>()
</script>

<template>
  <!-- Every item is a 24px-tall box with its own px-1.5, so spacing stays even
       whether neighbours are text, an icon or a button. -->
  <div class="flex shrink-0 items-center gap-1 border-b border-border px-1.5 py-1" :data-testid="testid">
    <div v-if="label" class="flex h-6 min-w-0 items-center gap-1.5 px-1.5">
      <IconFolder class="size-3.5 shrink-0 text-text-4" aria-hidden="true" />
      <span
        class="min-w-0 truncate text-[11px] text-text-3"
        :title="path"
        :data-testid="`${testid}-workspace`"
      >{{ label }}</span>
    </div>

    <div class="flex min-w-0 flex-1 items-center overflow-hidden"><slot /></div>

    <span
      v-if="error"
      class="flex h-6 shrink-0 items-center truncate px-1.5 text-[11px] text-severity-error"
      :data-testid="`${testid}-error`"
    >{{ error }}</span>
    <!-- size-6/rounded-[7px] is SessionStatusChips' chip metric too; changing
         it here means changing it there. -->
    <AppTooltip v-if="editorTitle" :text="`Open in ${editorTitle}`">
      <button
        type="button"
        class="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-[7px] text-text-3 hover:bg-chip hover:text-text"
        :aria-label="`Open in ${editorTitle}`"
        :data-testid="`${testid}-open-editor`"
        @click="$emit('open-editor')"
      ><IconCode class="size-3.5" /></button>
    </AppTooltip>
    <AppTooltip text="Show in Finder">
      <button
        type="button"
        class="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-[7px] text-text-3 hover:bg-chip hover:text-text"
        aria-label="Show in Finder"
        :data-testid="`${testid}-reveal`"
        @click="$emit('reveal')"
      ><IconFolderOpen class="size-3.5" /></button>
    </AppTooltip>
    <!-- Optional: an area with nothing to add here renders the row exactly as
         before, which is what keeps this shared bar drop-in for Agents too. -->
    <slot name="actions" />
  </div>
</template>
