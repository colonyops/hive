<script setup lang="ts">
// The title bar's overflow menu: views and pages that open over whatever is on
// screen. Each entry is a palette command, so the menu, the palette and the
// keymap stay one surface and a rebound key shows up here as its hint.
import { computed, ref } from 'vue'
import IconBookOpen from '~icons/lucide/book-open'
import IconLayoutGrid from '~icons/lucide/layout-grid'
import IconListTodo from '~icons/lucide/list-todo'
import IconSquareTerminal from '~icons/lucide/square-terminal'
import { formatCombo, useKeybindings } from '../composables/useKeybindings'
import type { MenuEntry } from '../types/menu'
import AppMenu from './ui/AppMenu.vue'
import IconButton from './ui/IconButton.vue'

defineProps<{ active?: boolean }>()
const emit = defineEmits<{ 'run-command': [commandId: string] }>()

const wrap = ref<HTMLElement | null>(null)
const open = ref(false)
const { combosFor } = useKeybindings()

function kbdFor(commandId: string): string | undefined {
  const combo = combosFor(commandId)[0]
  return combo ? formatCombo(combo) : undefined
}

const entries = computed<MenuEntry[]>(() => [
  {
    kind: 'action',
    id: 'action-runs.toggle',
    label: 'Action runs',
    icon: IconSquareTerminal,
    kbd: kbdFor('action-runs.toggle'),
    testid: 'titlebar-menu-action-runs',
  },
  {
    kind: 'action',
    id: 'tasks.toggle',
    label: 'Tasks',
    icon: IconListTodo,
    kbd: kbdFor('tasks.toggle'),
    testid: 'titlebar-menu-tasks',
  },
  {
    kind: 'action',
    id: 'canvas.toggle',
    label: 'Canvases',
    icon: IconBookOpen,
    kbd: kbdFor('canvas.toggle'),
    testid: 'titlebar-menu-canvases',
  },
])

function select(commandId: string): void {
  open.value = false
  emit('run-command', commandId)
}
</script>

<template>
  <div ref="wrap" class="relative ml-1" style="--wails-draggable: no-drag">
    <IconButton
      label="Open views menu"
      tooltip="Views"
      :icon="IconLayoutGrid"
      size="lg"
      aria-haspopup="menu"
      :aria-expanded="open"
      :active="open || active"
      data-testid="titlebar-menu"
      @click="open = !open"
    />
    <AppMenu
      v-if="open"
      :entries="entries"
      :ignore="[wrap]"
      width="220px"
      testid="titlebar-menu-panel"
      @select="select"
      @close="open = false"
    />
  </div>
</template>
