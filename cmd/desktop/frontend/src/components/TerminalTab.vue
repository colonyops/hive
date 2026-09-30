<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import TerminalPane from './TerminalPane.vue'
import { startDrag } from '../composables/useDragGesture'
import type { TerminalWindowTab, UseTerminalWindows } from '../composables/useTerminalWindows'
import { dividerTouches, draggedExtent, paneDividers, visiblePaneRects, type PaneDivider } from '../lib/terminalLayout'

const props = defineProps<{ tab: TerminalWindowTab; session: UseTerminalWindows; active: boolean }>()

const host = ref<HTMLElement | null>(null)

onMounted(() => {
  if (host.value) props.session.attachTab(props.tab.windowId, host.value)
})

const cell = computed(() => props.session.cell.value)
const dividers = computed(() => paneDividers(props.tab))
const split = computed(() => props.tab.panes.length > 1)

// Before xterm reports cell metrics, show only the active pane at full size.
const placements = computed(() => {
  const size = cell.value
  const placed = new Map<string, Record<string, string>>()
  for (const rect of visiblePaneRects(props.tab)) {
    if (size) {
      placed.set(rect.paneId, {
        left: `${rect.x * size.width}px`,
        top: `${rect.y * size.height}px`,
        width: `${rect.width * size.width}px`,
        height: `${rect.height * size.height}px`,
      })
    } else if (rect.paneId === props.tab.activePane) {
      placed.set(rect.paneId, { inset: '0' })
    }
  }
  return placed
})

// The border tmux leaves between two panes is one cell wide; the line is drawn
// down its middle and the whole cell is the drag target.
function dividerStyle(divider: PaneDivider): Record<string, string> | undefined {
  const size = cell.value
  if (!size) return undefined
  if (divider.axis === 'x') {
    return {
      left: `${divider.at * size.width}px`,
      top: `${divider.from * size.height}px`,
      width: `${size.width}px`,
      height: `${(divider.to - divider.from) * size.height}px`,
    }
  }
  return {
    left: `${divider.from * size.width}px`,
    top: `${divider.at * size.height}px`,
    width: `${(divider.to - divider.from) * size.width}px`,
    height: `${size.height}px`,
  }
}

function dividerKey(divider: PaneDivider): string {
  return `${divider.axis}:${divider.beforePanes[0]}:${divider.afterPanes[0]}`
}

function dividerClass(divider: PaneDivider): string[] {
  return [
    divider.axis === 'x' ? 'flex-col' : '',
    divider.before ? `terminal-divider-draggable ${divider.axis === 'x' ? 'cursor-col-resize' : 'cursor-row-resize'}` : '',
    dividerTouches(divider, props.tab.activePane) ? 'terminal-divider-active' : '',
  ]
}

// tmux owns layout. Dragging requests a pane size; the streamed layout redraws
// the panes.
function startDividerDrag(divider: PaneDivider, event: PointerEvent): void {
  const size = cell.value
  const box = host.value?.getBoundingClientRect()
  if (!divider.before || !size || !box) return
  event.preventDefault()
  let last = divider.extent
  startDrag(event, {
    pointerCapture: true,
    onMove: (move) => {
      const position = divider.axis === 'x'
        ? (move.clientX - box.left) / size.width
        : (move.clientY - box.top) / size.height
      const extent = draggedExtent(divider, position)
      if (extent === last) return
      last = extent
      void props.session.resizePane(divider.before, divider.axis === 'x' ? { width: extent } : { height: extent })
    },
  })
}
</script>

<template>
  <!-- Hidden rather than unmounted: a Terminal binds to one element for its
       lifetime, so unmounting an inactive tab would cost every pane its
       screen.

       overflow-auto, not hidden: the grid is tmux's size, not this box's, so
       when tmux gives the window more columns than fit — another client is
       attached wider — the pane scrolls instead of reflowing. The host stays
       100% of the box so the size vote still measures the pane, and a grid
       smaller than the box sits top-left. -->
  <div
    v-show="active"
    class="min-h-0 min-w-0 flex-1 overflow-auto bg-app px-2 py-1.5"
    data-terminal-input-scope
    data-testid="terminal-pane"
    :data-window-id="tab.windowId"
  >
    <div ref="host" class="relative size-full">
      <TerminalPane
        v-for="pane in tab.panes"
        :key="pane.uid"
        v-show="placements.has(pane.paneId)"
        :pane="pane"
        :active="split && pane.paneId === tab.activePane"
        :style="placements.get(pane.paneId)"
        @mount="session.attachPane"
        @select="session.selectPane"
      />
      <div
        v-for="divider in dividers"
        :key="dividerKey(divider)"
        class="terminal-divider absolute z-10 flex items-center justify-center"
        :class="dividerClass(divider)"
        :style="dividerStyle(divider)"
        role="separator"
        :aria-orientation="divider.axis === 'x' ? 'vertical' : 'horizontal'"
        data-testid="terminal-pane-divider"
        :data-axis="divider.axis"
        :data-before="divider.before"
        @pointerdown="startDividerDrag(divider, $event)"
      >
        <div :class="divider.axis === 'x' ? 'h-full w-px' : 'h-px w-full'" class="terminal-divider-line" />
      </div>
      <div
        v-if="tab.zoomed && split"
        class="pointer-events-none absolute right-2 top-1 z-10 rounded border border-strong bg-raised/90 px-1.5 py-0.5 font-mono text-[10px] text-text-3"
        data-testid="terminal-pane-zoomed"
      >zoomed</div>
    </div>
  </div>
</template>

<style scoped>
/* Persistent dividers use neutral borders; an accent would read as a permanent highlight. */
.terminal-divider { touch-action: none; }
.terminal-divider-line { background: var(--color-border); }
.terminal-divider-active .terminal-divider-line { background: var(--color-strong); }
.terminal-divider-draggable:hover .terminal-divider-line,
.terminal-divider-draggable:active .terminal-divider-line { background: var(--color-text-4); }
</style>
