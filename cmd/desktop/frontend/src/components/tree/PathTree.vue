<script setup lang="ts">
// Mounts a @pierre/trees model. The tree renders in its own shadow root, so
// the theme reaches it only through the --trees-*-override properties below,
// which inherit across the boundary.
import type { FileTree } from '@pierre/trees'
import { onBeforeUnmount, onMounted, ref, watchEffect } from 'vue'

const props = defineProps<{
  model: FileTree
  /** Extra CSS for the shadow root, for row state the tree has no option for. */
  rowCss?: string
}>()

const host = ref<HTMLElement | null>(null)
let rowStyle: HTMLStyleElement | null = null

onMounted(() => {
  if (!host.value) return
  props.model.render({ fileTreeContainer: host.value })
  rowStyle = document.createElement('style')
  rowStyle.dataset.hiveRowCss = ''
  host.value.shadowRoot?.append(rowStyle)
})

watchEffect(() => {
  if (rowStyle) rowStyle.textContent = props.rowCss ?? ''
})

onBeforeUnmount(() => {
  rowStyle?.remove()
  props.model.unmount()
})
</script>

<template>
  <div ref="host" class="path-tree" />
</template>

<style scoped>
.path-tree {
  min-height: 0;
  flex: 1 1 auto;
  height: 100%;
  --trees-bg-override: transparent;
  --trees-fg-override: var(--color-text-2);
  --trees-fg-muted-override: var(--color-text-4);
  --trees-accent-override: var(--color-accent);
  --trees-bg-muted-override: var(--color-chip);
  --trees-selected-bg-override: var(--color-selection);
  --trees-selected-fg-override: var(--color-text);
  --trees-selected-focused-border-color-override: var(--color-accent);
  --trees-focus-ring-color-override: var(--color-accent);
  --trees-border-color-override: var(--color-border);
  --trees-input-bg-override: var(--color-app);
  --trees-search-bg-override: var(--color-app);
  --trees-search-fg-override: var(--color-text);
  --trees-scrollbar-thumb-override: var(--color-border);
  --trees-font-family-override: var(--font-sans);
  --trees-font-size-override: var(--text-small);
  --trees-padding-inline-override: 8px;
  --trees-git-added-color-override: var(--color-severity-success);
  --trees-git-untracked-color-override: var(--color-severity-success);
  --trees-git-modified-color-override: var(--color-severity-warning);
  --trees-git-deleted-color-override: var(--color-severity-error);
  --trees-git-renamed-color-override: var(--color-severity-info);
  --trees-git-ignored-color-override: var(--color-text-4);
}
</style>
