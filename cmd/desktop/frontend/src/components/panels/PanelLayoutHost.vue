<script setup lang="ts">
import { computed, ref } from 'vue'
import IconColumns2 from '~icons/lucide/columns-2'
import IconRotateCw from '~icons/lucide/rotate-cw'
import IconRows2 from '~icons/lucide/rows-2'
import IconX from '~icons/lucide/x'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import {
  closePanel,
  focusPanel,
  panelBoxes,
  panelWithTab,
  panelsOf,
  resizeSplit,
  selectTab,
  splitBoxes,
  splitPanel,
  tabById,
  turnSplit,
  type Panel,
  type PanelBox,
  type PanelLayout,
  type PanelSplit,
  type PanelTab,
} from '../../lib/panelLayout'
import PanelTabRow from './PanelTabRow.vue'
import { TAB_ROW_PX } from './panelTabRow'
import { usePanelDrag } from './usePanelDrag'

const props = withDefaults(
  defineProps<{
    layout: PanelLayout
    apply: (rule: (layout: PanelLayout) => PanelLayout) => void
    moveTab: (tabId: string, panel: string, gap: number) => void
    tabLabel: (tab: PanelTab) => string
    tabAttention?: (tab: PanelTab) => boolean
    /** Window names are commands and paths, so they read in the mono face. */
    tabMono?: (tab: PanelTab) => boolean
    /** A tab drawn in the floating layer has no content of its own to wrap. */
    hasContent?: (tab: PanelTab) => boolean
    contentKey?: string
    hidden?: boolean
  }>(),
  {
    tabAttention: () => false,
    tabMono: () => false,
    hasContent: () => true,
    contentKey: '',
    hidden: false,
  },
)
const emit = defineEmits<{ 'close-tab': [panelId: string, tabId: string] }>()

defineSlots<{
  icon(props: { tab: PanelTab }): unknown
  add(props: { panel: Panel }): unknown
  actions(props: { panel: Panel; lead: boolean }): unknown
  picker(props: { panel: Panel }): unknown
  tab(props: { panel: Panel; tab: PanelTab; shown: boolean; focused: boolean }): unknown
  floating(props: { region: (panelId: string) => Record<string, string> }): unknown
}>()

const BORDER_PX = 1

const panelList = computed(() => panelsOf(props.layout))
const boxes = computed(() => panelBoxes(props.layout))
const splits = computed(() => splitBoxes(props.layout))
const root = ref<HTMLElement | null>(null)
const { tabDrop, dragging, pointer, startTabDrag, startResize, stepResize } = usePanelDrag(
  { apply: (rule) => props.apply(rule), moveTabTo: (tabId, panel, gap) => props.moveTab(tabId, panel, gap) },
  root,
)

// The drag image a pointer drag lacks, and the panel the tab would join. Its
// own panel is not washed: there the gap marker alone says where it lands.
const draggedTab = computed(() => (dragging.value ? tabById(props.layout, dragging.value) : undefined))
const dropTargetPanel = computed(() => {
  if (!dragging.value || !tabDrop.value) return ''
  return panelWithTab(props.layout, dragging.value)?.id === tabDrop.value.panel ? '' : tabDrop.value.panel
})
const GHOST_OFFSET_PX = 12

const percent = (share: number): string => `${share * 100}%`

function panelStyle(panelId: string): Record<string, string> {
  const box = boxes.value.get(panelId)
  if (!box) return {}
  return { left: percent(box.x), top: percent(box.y), width: percent(box.width), height: percent(box.height) }
}

// The content box under a panel's tab row, for the floating layer: content that
// must keep one DOM element for its lifetime sits there and is placed by position.
// The one place this geometry is computed; a second formula would drift from it.
function region(panelId: string): Record<string, string> {
  const box = boxes.value.get(panelId)
  if (!box) return { inset: `${TAB_ROW_PX}px 0 0 0` }
  const left = box.x > 0 ? BORDER_PX : 0
  const top = TAB_ROW_PX + (box.y > 0 ? BORDER_PX : 0)
  return {
    left: `calc(${percent(box.x)} + ${left}px)`,
    top: `calc(${percent(box.y)} + ${top}px)`,
    width: `calc(${percent(box.width)} - ${left}px)`,
    height: `calc(${percent(box.height)} - ${top}px)`,
  }
}

function dividerStyle(split: PanelSplit, box: PanelBox): Record<string, string> {
  if (split.axis === 'row') {
    return {
      left: `calc(${percent(box.x + box.width * split.ratio)} - 3px)`,
      top: percent(box.y),
      height: percent(box.height),
      width: '6px',
    }
  }
  return {
    top: `calc(${percent(box.y + box.height * split.ratio)} - 3px)`,
    left: percent(box.x),
    width: percent(box.width),
    height: '6px',
  }
}

// A panel under a stacked boundary: its tab row is a grab area for that boundary.
function boundaryAbove(panelId: string): { split: PanelSplit; box: PanelBox } | undefined {
  const panel = boxes.value.get(panelId)
  if (!panel || panel.y === 0) return undefined
  return splits.value.find(({ split, box }) => {
    if (split.axis !== 'column') return false
    const at = box.y + box.height * split.ratio
    return Math.abs(at - panel.y) < 1e-6 && panel.x >= box.x - 1e-6 && panel.x + panel.width <= box.x + box.width + 1e-6
  })
}

function startRowResize(event: PointerEvent, panelId: string): void {
  const boundary = boundaryAbove(panelId)
  if (boundary) startResize(event, boundary.split, boundary.box)
}

// The session's own actions sit where one pane had them, in the top right
// corner; exactly one panel reaches it however the layout is split.
const leadPanel = computed(() => {
  for (const panel of panelList.value) {
    const box = boxes.value.get(panel.id)
    if (box && box.y === 0 && Math.abs(box.x + box.width - 1) < 1e-6) return panel.id
  }
  return ''
})

function tabShown(panel: Panel, tabId: string): boolean {
  return !panel.picking && panel.activeTab === tabId
}

function contentTabs(tabs: readonly PanelTab[]): PanelTab[] {
  return tabs.filter((tab) => props.hasContent(tab))
}
</script>

<template>
  <!-- Hidden rather than unmounted: a pooled session's terminals keep their
       elements, or each would lose its screen. -->
  <div
    ref="root"
    class="relative min-h-0 min-w-0 flex-1"
    :class="hidden ? 'hidden' : 'block'"
    data-testid="panel-layout"
  >
    <section
      v-for="panel in panelList"
      :key="panel.id"
      class="absolute flex min-h-0 min-w-0 flex-col"
      :class="[
        (boxes.get(panel.id)?.x ?? 0) > 0 ? 'border-l border-strong' : '',
        (boxes.get(panel.id)?.y ?? 0) > 0 ? 'border-t border-strong' : '',
      ]"
      :style="panelStyle(panel.id)"
      :data-panel="panel.id"
      :aria-label="`Panel ${panel.id}`"
      :data-testid="`panel-${panel.id}`"
      @pointerdown.capture="apply((current) => focusPanel(current, panel.id))"
    >
      <PanelTabRow
        :panel="panel"
        :focused="layout.focusedPanel === panel.id"
        :label="tabLabel"
        :attention="tabAttention"
        :mono="tabMono"
        :drop-gap="tabDrop?.panel === panel.id ? tabDrop.gap : null"
        :dragging="dragging"
        :resizable="!!boundaryAbove(panel.id)"
        @tab-drag="startTabDrag"
        @resize-start="(event) => startRowResize(event, panel.id)"
        @select="(tabId) => apply((current) => selectTab(current, panel.id, tabId))"
        @close="(tabId) => emit('close-tab', panel.id, tabId)"
      >
        <template #icon="{ tab }">
          <slot name="icon" :tab="tab" />
        </template>
        <template #add="{ panel: addPanel }">
          <slot name="add" :panel="addPanel" />
        </template>
        <template #actions>
          <div class="ml-auto flex shrink-0 items-center gap-px pl-2">
            <slot name="actions" :panel="panel" :lead="panel.id === leadPanel" />
            <IconButton
              label="Split right"
              :icon="IconColumns2"
              :data-testid="`panel-split-row-${panel.id}`"
              @click="apply((current) => splitPanel(current, panel.id, 'row'))"
            />
            <IconButton
              label="Split down"
              :icon="IconRows2"
              :data-testid="`panel-split-column-${panel.id}`"
              @click="apply((current) => splitPanel(current, panel.id, 'column'))"
            />
            <IconButton
              v-if="panelList.length > 1"
              label="Close this panel, keeping its tabs"
              :icon="IconX"
              :data-testid="`panel-close-${panel.id}`"
              @click="apply((current) => closePanel(current, panel.id))"
            />
          </div>
        </template>
      </PanelTabRow>

      <div class="relative flex min-h-0 min-w-0 flex-1 flex-col">
        <slot v-if="panel.picking" name="picker" :panel="panel" />
        <!-- Mounted while open, so a tab switch keeps a diff's scroll and picks;
             a session switch re-keys them, so only the visible session pays. -->
        <div
          v-for="tab in contentTabs(panel.tabs)"
          v-show="tabShown(panel, tab.id)"
          :key="`${contentKey}:${tab.id}`"
          class="absolute inset-0 flex min-h-0 min-w-0 flex-col bg-app"
          :data-testid="`panel-view-${tab.kind}`"
        >
          <slot
            name="tab"
            :panel="panel"
            :tab="tab"
            :shown="tabShown(panel, tab.id)"
            :focused="layout.focusedPanel === panel.id"
          />
        </div>
      </div>
    </section>

    <slot name="floating" :region="region" />

    <!-- Above the floating layer, which paints over the sections it belongs to. -->
    <Transition name="panel-drop">
      <div
        v-if="dropTargetPanel"
        :key="dropTargetPanel"
        class="panel-drop-target pointer-events-none absolute z-30"
        :style="panelStyle(dropTargetPanel)"
        data-testid="panel-drop-target"
      />
    </Transition>

    <Teleport to="body">
      <div
        v-if="draggedTab && pointer"
        class="pointer-events-none fixed z-50 flex items-center gap-1.5 rounded border border-strong bg-raised px-2.5 py-1 text-small text-text shadow-lg"
        :style="{ left: `${pointer.x + GHOST_OFFSET_PX}px`, top: `${pointer.y + GHOST_OFFSET_PX}px` }"
        data-testid="panel-drag-ghost"
      >
        <slot name="icon" :tab="draggedTab" />
        <span class="max-w-[168px] truncate" :class="tabMono(draggedTab) ? 'font-mono' : ''">{{
          tabLabel(draggedTab)
        }}</span>
      </div>
    </Teleport>

    <div
      v-for="{ split, box } in splits"
      :key="split.id"
      class="group/divider absolute z-20 touch-none"
      :class="split.axis === 'column' ? 'cursor-row-resize' : 'cursor-col-resize'"
      :style="dividerStyle(split, box)"
      role="separator"
      tabindex="0"
      :aria-orientation="split.axis === 'column' ? 'horizontal' : 'vertical'"
      :aria-valuenow="Math.round(split.ratio * 100)"
      aria-label="Resize the panels"
      :data-testid="`panel-divider-${split.id}`"
      @pointerdown="startResize($event, split, box)"
      @dblclick="apply((current) => resizeSplit(current, split.id, 0.5))"
      @keydown.left.prevent="stepResize(split, -1)"
      @keydown.up.prevent="stepResize(split, -1)"
      @keydown.right.prevent="stepResize(split, 1)"
      @keydown.down.prevent="stepResize(split, 1)"
    >
      <button
        type="button"
        class="absolute flex size-5 cursor-pointer items-center justify-center rounded-full border border-strong bg-raised text-text-3 opacity-0 hover:text-text focus-visible:opacity-100 group-hover/divider:opacity-100"
        :class="split.axis === 'column' ? 'left-3 top-1/2 -translate-y-1/2' : 'left-1/2 top-9 -translate-x-1/2'"
        aria-label="Turn this split"
        title="Turn this split"
        :data-testid="`panel-turn-${split.id}`"
        @pointerdown.stop
        @click="apply((current) => turnSplit(current, split.id))"
      >
        <IconRotateCw class="size-3" aria-hidden="true" />
      </button>
    </div>
  </div>
</template>

<style scoped>
/* The tree's insertion line, drawn around the whole panel: an inset ring, so a
   terminal under it keeps its size, over a wash that reads through the content. */
.panel-drop-target {
  background: color-mix(in srgb, var(--color-accent) 10%, transparent);
  box-shadow: inset 0 0 0 2px var(--color-accent);
}
.panel-drop-enter-active,
.panel-drop-leave-active {
  transition: opacity 0.15s ease;
}
.panel-drop-enter-from,
.panel-drop-leave-to {
  opacity: 0;
}
@media (prefers-reduced-motion: reduce) {
  .panel-drop-enter-active,
  .panel-drop-leave-active {
    transition: none;
  }
}
</style>
