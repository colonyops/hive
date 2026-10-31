<script setup lang="ts">
import { ref } from 'vue'
import IconFileDiff from '~icons/lucide/file-diff'
import IconFileText from '~icons/lucide/file-text'
import IconListTodo from '~icons/lucide/list-todo'
import IconPlus from '~icons/lucide/plus'
import IconTerminal from '~icons/lucide/terminal'
import IconX from '~icons/lucide/x'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import type { MenuEntry } from '@hive/ui/types/menu'
import type { SessionPane, SessionTab } from '../../lib/sessionLayout'

const props = defineProps<{
  pane: SessionPane
  index: number
  focused: boolean
  /** Below the split's width floor, only the tab in view keeps its label. */
  compact: boolean
  label: (tab: SessionTab) => string
  attention: (tab: SessionTab) => boolean
  menuEntries: MenuEntry[]
  /** The gap a dragged tab would land in, when this row is the target. */
  dropGap: number | null
  /** In a stacked split this row is the boundary, so its empty space resizes. */
  resizable: boolean
}>()
const emit = defineEmits<{
  select: [tabId: string]
  close: [tabId: string]
  add: [entryId: string]
  'tab-drag': [event: PointerEvent, tabId: string]
  'resize-start': [event: PointerEvent]
}>()

const ICONS = {
  terminal: IconTerminal,
  window: IconTerminal,
  docs: IconFileText,
  diff: IconFileDiff,
  tasks: IconListTodo,
}

const menuOpen = ref(false)
const addButton = ref<HTMLElement | null>(null)

function shown(tab: SessionTab): boolean {
  return !props.pane.picking && tab.id === props.pane.activeTab
}

function closeLabel(tab: SessionTab): string {
  return tab.kind === 'window' ? `Hide ${props.label(tab)} from this pane` : `Close ${props.label(tab)}`
}

function pick(entryId: string): void {
  menuOpen.value = false
  emit('add', entryId)
}
</script>

<template>
  <div
    class="flex h-[30px] shrink-0 items-center gap-0.5 border-b border-border bg-sidebar px-1"
    :class="resizable ? 'cursor-row-resize' : ''"
    :data-testid="`session-tab-row-${index}`"
    @pointerdown.self="resizable && emit('resize-start', $event)"
  >
    <div class="session-tabs flex min-w-0 shrink items-center gap-0.5 overflow-x-auto" role="tablist">
      <div
        v-for="(tab, at) in pane.tabs"
        :key="tab.id"
        data-session-tab
        class="group relative flex h-[22px] touch-none select-none min-w-0 max-w-[136px] shrink items-center rounded text-small"
        :class="[
          shown(tab)
            ? focused
              ? 'bg-hover text-text'
              : 'bg-chip text-text-2'
            : 'text-text-3 hover:bg-chip hover:text-text',
          compact && !shown(tab) ? 'shrink-0' : 'min-w-[52px]',
        ]"
        :data-testid="`session-tab-${tab.id}`"
        :data-shown="shown(tab)"
        @pointerdown="tab.kind !== 'terminal' && emit('tab-drag', $event, tab.id)"
      >
        <span
          v-if="dropGap === at || (dropGap === pane.tabs.length && at === pane.tabs.length - 1)"
          class="pointer-events-none absolute inset-y-0 w-0.5 rounded bg-accent"
          :class="dropGap === at ? '-left-px' : '-right-px'"
          data-testid="session-tab-drop-marker"
        />
        <button
          type="button"
          role="tab"
          class="flex h-full min-w-0 flex-1 cursor-pointer items-center gap-1.5 rounded pl-2"
          :class="tab.kind === 'terminal' || (compact && !shown(tab)) ? 'pr-2' : 'pr-0.5'"
          :aria-selected="shown(tab)"
          :aria-label="label(tab)"
          :title="tab.kind === 'terminal' ? 'Terminal: pick a window in the tree. This tab stays open.' : label(tab)"
          @click="emit('select', tab.id)"
        >
          <component :is="ICONS[tab.kind]" class="size-[13px] shrink-0" aria-hidden="true" />
          <span v-if="!compact || shown(tab)" class="min-w-0 truncate">{{ label(tab) }}</span>
          <span
            v-if="attention(tab)"
            class="size-1.5 shrink-0 rounded-full bg-accent"
            :data-testid="`session-tab-attention-${tab.id}`"
          />
        </button>
        <button
          v-if="tab.kind !== 'terminal' && (!compact || shown(tab))"
          type="button"
          class="mr-0.5 flex size-4 shrink-0 cursor-pointer items-center justify-center rounded text-text-3 opacity-0 hover:bg-hover hover:text-text focus-visible:opacity-100 group-hover:opacity-100"
          :class="shown(tab) ? 'opacity-100' : ''"
          :aria-label="closeLabel(tab)"
          :title="closeLabel(tab)"
          :data-testid="`session-tab-close-${tab.id}`"
          @pointerdown.stop
          @click="emit('close', tab.id)"
        >
          <IconX class="size-3" aria-hidden="true" />
        </button>
      </div>
    </div>

    <div ref="addButton" class="relative ml-0.5 shrink-0">
      <IconButton
        label="Open a tab"
        :icon="IconPlus"
        size="sm"
        :active="menuOpen"
        :data-testid="`session-tab-add-${index}`"
        @click="menuOpen = !menuOpen"
      />
      <AppMenu
        v-if="menuOpen"
        align="start"
        :anchor="addButton"
        :entries="menuEntries"
        :ignore="[addButton]"
        :testid="`session-tab-menu-${index}`"
        @close="menuOpen = false"
        @select="pick"
      />
    </div>

    <slot name="actions" />
  </div>
</template>

<style scoped>
.session-tabs {
  scrollbar-width: none;
}
.session-tabs::-webkit-scrollbar {
  display: none;
}
</style>
