<script setup lang="ts">
import { computed, ref } from 'vue'
import IconX from '~icons/lucide/x'
import { useElementSize } from '@vueuse/core'
import type { Panel, PanelTab } from '../../lib/panelLayout'
import { TAB_ROW_PX } from './panelTabRow'

const props = defineProps<{
  panel: Panel
  focused: boolean
  label: (tab: PanelTab) => string
  attention: (tab: PanelTab) => boolean
  /** Window names are commands and paths, so they read in the mono face. */
  mono?: (tab: PanelTab) => boolean
  /** The gap a dragged tab would land in, when this row is the target. */
  dropGap: number | null
  /** In a stacked split this row is the boundary, so its empty space resizes. */
  resizable: boolean
}>()
const emit = defineEmits<{
  select: [tabId: string]
  close: [tabId: string]
  'tab-drag': [event: PointerEvent, tabId: string]
  'resize-start': [event: PointerEvent]
}>()

function shown(tab: PanelTab): boolean {
  return !props.panel.picking && tab.id === props.panel.activeTab
}

function closeLabel(tab: PanelTab): string {
  return `Close ${props.label(tab)}`
}

// Measured per row, because either side of a split can be the narrow one. Only
// the tab in view keeps its label when every tab cannot.
const TAB_MIN_WIDTH_PX = 110
const rowEl = ref<HTMLElement | null>(null)
const { width: rowWidth } = useElementSize(rowEl)
const compact = computed(() => rowWidth.value > 0 && rowWidth.value / (props.panel.tabs.length + 1) < TAB_MIN_WIDTH_PX)
</script>

<template>
  <div
    ref="rowEl"
    class="flex shrink-0 items-stretch border-b border-border bg-sidebar pr-1"
    :class="resizable ? 'cursor-row-resize' : ''"
    :style="{ height: `${TAB_ROW_PX}px` }"
    :data-testid="`panel-tab-row-${panel.id}`"
    @pointerdown.self="resizable && emit('resize-start', $event)"
  >
    <div class="panel-tabs flex min-w-0 shrink items-stretch overflow-x-auto" role="tablist">
      <div
        v-for="(tab, at) in panel.tabs"
        :key="tab.id"
        data-panel-tab
        class="panel-tab group relative flex min-w-0 max-w-[168px] shrink touch-none select-none items-center text-small"
        :class="[
          shown(tab) ? (focused ? 'panel-tab-focused' : 'panel-tab-shown') : 'text-text-2',
          compact && !shown(tab) ? 'shrink-0' : 'min-w-[56px]',
        ]"
        :data-testid="`panel-tab-${tab.id}`"
        :data-shown="shown(tab)"
        @pointerdown="emit('tab-drag', $event, tab.id)"
      >
        <span
          v-if="dropGap === at || (dropGap === panel.tabs.length && at === panel.tabs.length - 1)"
          class="pointer-events-none absolute inset-y-1 z-10 w-0.5 rounded bg-accent"
          :class="dropGap === at ? '-left-px' : '-right-px'"
          data-testid="panel-tab-drop-marker"
        />
        <button
          type="button"
          role="tab"
          class="flex h-full min-w-0 flex-1 cursor-pointer items-center gap-1.5 pl-2.5"
          :class="compact && !shown(tab) ? 'pr-2.5' : 'pr-1'"
          :aria-selected="shown(tab)"
          :aria-label="label(tab)"
          :title="label(tab)"
          @click="emit('select', tab.id)"
        >
          <slot name="icon" :tab="tab" />
          <span v-if="!compact || shown(tab)" class="min-w-0 truncate" :class="mono?.(tab) ? 'font-mono' : ''">{{
            label(tab)
          }}</span>
          <span
            v-if="attention(tab)"
            class="size-1.5 shrink-0 rounded-full bg-accent"
            :data-testid="`panel-tab-attention-${tab.id}`"
          />
        </button>
        <button
          v-if="!compact || shown(tab)"
          type="button"
          class="mr-1 flex size-4 shrink-0 cursor-pointer items-center justify-center rounded text-text-3 opacity-0 hover:bg-hover hover:text-text focus-visible:opacity-100 group-hover:opacity-100"
          :class="shown(tab) ? 'opacity-100' : ''"
          :aria-label="closeLabel(tab)"
          :title="closeLabel(tab)"
          :data-testid="`panel-tab-close-${tab.id}`"
          @pointerdown.stop
          @click="emit('close', tab.id)"
        >
          <IconX class="size-3" aria-hidden="true" />
        </button>
      </div>
    </div>

    <div class="relative ml-0.5 flex shrink-0 items-center">
      <slot name="add" :panel="panel" />
    </div>

    <slot name="actions" />
  </div>
</template>

<style scoped>
.panel-tabs {
  scrollbar-width: none;
}
.panel-tabs::-webkit-scrollbar {
  display: none;
}
/* The tree's language: flat rows, a hover wash, and an accent rail on what is
   selected, here along the bottom edge because the tabs run sideways. */
.panel-tab:hover {
  background: var(--color-chip);
}
.panel-tab-focused {
  font-weight: 500;
  color: var(--color-accent);
}
.panel-tab-shown {
  color: var(--color-text);
}
.panel-tab-focused::after,
.panel-tab-shown::after {
  content: '';
  position: absolute;
  inset: auto 0 -1px;
  height: 2px;
  background: var(--color-accent);
}
.panel-tab-shown::after {
  background: var(--color-text-4);
}
</style>
