<script setup lang="ts">
import { computed, ref, shallowRef } from 'vue'
import { useEventListener } from '@vueuse/core'
import IconBan from '~icons/lucide/ban'
import IconCheck from '~icons/lucide/check'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconCopy from '~icons/lucide/copy'
import IconEllipsis from '~icons/lucide/ellipsis'
import IconLayers from '~icons/lucide/layers'
import IconRotateCcw from '~icons/lucide/rotate-ccw'
import IconTerminal from '~icons/lucide/terminal'
import IconTrash2 from '~icons/lucide/trash-2'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import AppTooltip from '@hive/ui/components/ui/AppTooltip.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import type { MenuEntry } from '@hive/ui/types/menu'
import type { TaskItem } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { relativeAge } from '../../lib/age'
import { doneToggleStatus, isTerminalStatus, statusMeta, TASK_STATUSES } from '../../lib/tasksPresentation'
import { taskStatusIcon } from './taskStatusIcon'

const props = withDefaults(
  defineProps<{
    item: TaskItem
    selected: boolean
    depth?: number
    expandable?: boolean
    collapsed?: boolean
    /** [done/total] over the item's descendants, when it has any. */
    counts?: { done: number; total: number }
    /** Display name for the linked session; absent when it isn't loaded (e.g. ended). */
    sessionName?: string
  }>(),
  {
    depth: 0,
    expandable: false,
    collapsed: false,
    counts: undefined,
    sessionName: undefined,
  },
)
const emit = defineEmits<{
  select: [id: string]
  toggle: [id: string]
  status: [item: TaskItem, status: string]
  copy: [id: string]
  delete: [id: string]
}>()

const isEpic = computed(() => props.item.type === 'epic')
// 18px per level reads clearly at up to ~4 levels deep without the row
// running out of room for its trailing chips.
const indent = computed(() => props.depth * 18 + 12)

const closed = computed(() => isTerminalStatus(props.item.status))

const rowEl = ref<HTMLElement | null>(null)
const openMenu = ref<'status' | 'actions' | null>(null)
const menuToggle = shallowRef<HTMLElement | null>(null)

function toggleMenu(menu: 'status' | 'actions', event: MouseEvent): void {
  menuToggle.value = event.currentTarget as HTMLElement
  openMenu.value = openMenu.value === menu ? null : menu
}

function openContextMenu(): void {
  menuToggle.value = null
  openMenu.value = 'actions'
}

// A right-click is not a click, so AppMenu's click-outside misses it and a
// second row's context menu would open beside this one. Capture runs first, so
// a right-click on this row still reopens the menu through openContextMenu.
useEventListener(
  () => (openMenu.value ? window : null),
  'contextmenu',
  () => {
    openMenu.value = null
  },
  { capture: true },
)

const statusEntries = computed<MenuEntry[]>(() =>
  TASK_STATUSES.map((status) => ({
    kind: 'action',
    id: status,
    label: statusMeta(status).label,
    icon: taskStatusIcon(status),
    checked: props.item.status === status,
    testid: `task-row-menu-status-${status}`,
  })),
)
const menuEntries = computed<MenuEntry[]>(() => {
  if (openMenu.value === 'status') return statusEntries.value
  return [
    ...statusEntries.value,
    { kind: 'separator' },
    { kind: 'action', id: 'copy', label: 'Copy ID', icon: IconCopy, testid: 'task-row-menu-copy' },
    { kind: 'action', id: 'delete', label: 'Delete…', icon: IconTrash2, testid: 'task-row-menu-delete' },
  ]
})

function onMenuSelect(id: string): void {
  openMenu.value = null
  if (id === 'copy') emit('copy', props.item.id)
  else if (id === 'delete') emit('delete', props.item.id)
  else if (id !== props.item.status) emit('status', props.item, id)
}
</script>

<template>
  <div
    ref="rowEl"
    class="group flex h-9 min-w-0 cursor-pointer items-center gap-2 border-b border-row pr-4 text-body transition-colors"
    :class="[
      selected ? 'bg-selection text-text' : 'text-text-2 hover:bg-row-hover hover:text-text',
      !selected && openMenu ? 'bg-row-hover text-text' : '',
    ]"
    :style="{ paddingLeft: indent + 'px' }"
    role="button"
    tabindex="0"
    :aria-selected="selected"
    :data-id="item.id"
    data-testid="task-row"
    @click="emit('select', item.id)"
    @keydown.enter.self.prevent="emit('select', item.id)"
    @contextmenu.prevent="openContextMenu"
  >
    <IconButton
      v-if="expandable"
      :label="collapsed ? 'Expand' : 'Collapse'"
      :icon="collapsed ? IconChevronRight : IconChevronDown"
      size="sm"
      :aria-expanded="!collapsed"
      data-testid="task-row-toggle"
      @click.stop="emit('toggle', item.id)"
    />
    <span v-else class="size-4.5 shrink-0" aria-hidden="true" />

    <span class="w-[92px] shrink-0 truncate font-mono text-caption text-text-4" data-testid="task-row-id">{{
      item.id
    }}</span>

    <IconButton
      :label="`Change status: ${statusMeta(item.status).label}`"
      :icon="taskStatusIcon(item.status)"
      size="sm"
      :active="openMenu === 'status'"
      aria-haspopup="menu"
      data-testid="task-row-status"
      @click.stop="toggleMenu('status', $event)"
    />

    <IconLayers v-if="isEpic" class="size-3.5 shrink-0 text-accent" aria-label="Epic" />

    <span class="min-w-0 flex-1 truncate" :class="isEpic ? 'font-medium text-text' : ''" data-testid="task-row-title">{{
      item.title
    }}</span>

    <span v-if="counts" class="shrink-0 font-mono text-caption text-text-4" data-testid="task-row-counts"
      >{{ counts.done }}/{{ counts.total }}</span
    >

    <BaseBadge
      v-if="item.blocked"
      tone="danger"
      variant="pill"
      class="shrink-0 px-2 py-0.5 text-caption font-medium"
      data-testid="task-row-blocked"
    >
      <IconBan class="size-2.5" aria-hidden="true" />Blocked
    </BaseBadge>

    <AppTooltip v-if="item.sessionId" :text="`Linked to session ${sessionName || item.sessionId}`">
      <span
        class="inline-flex shrink-0 items-center gap-1 text-micro text-text-4"
        :aria-label="`Session ${sessionName || item.sessionId}`"
        data-testid="task-row-session"
        ><IconTerminal class="size-3" aria-hidden="true"
      /></span>
    </AppTooltip>

    <!-- The age and the actions share one fixed cell, so hovering a row never
         reflows its title. -->
    <div class="relative h-6 w-[52px] shrink-0">
      <span
        class="absolute inset-0 text-right font-mono text-caption leading-6 text-text-4"
        :class="selected || openMenu ? 'invisible' : 'group-hover:invisible group-focus-within:invisible'"
        :title="new Date(item.updatedAt).toLocaleString()"
        data-testid="task-row-age"
        >{{ relativeAge(Date.parse(item.updatedAt)) }}</span
      >
      <div
        class="absolute inset-0 items-center justify-end gap-0.5"
        :class="selected || openMenu ? 'flex' : 'hidden group-hover:flex group-focus-within:flex'"
        data-testid="task-row-actions"
      >
        <IconButton
          :label="closed ? 'Reopen' : 'Mark done'"
          :tooltip="closed ? 'Reopen (d)' : 'Mark done (d)'"
          :icon="closed ? IconRotateCcw : IconCheck"
          data-testid="task-row-done"
          @click.stop="emit('status', item, doneToggleStatus(item.status))"
        />
        <IconButton
          label="More actions"
          :icon="IconEllipsis"
          :active="openMenu === 'actions'"
          aria-haspopup="menu"
          data-testid="task-row-more"
          @click.stop="toggleMenu('actions', $event)"
        />
      </div>
    </div>

    <AppMenu
      v-if="openMenu"
      :entries="menuEntries"
      :anchor="openMenu === 'status' ? menuToggle : rowEl"
      :align="openMenu === 'status' ? 'start' : 'end'"
      :ignore="[menuToggle]"
      testid="task-row-menu"
      @select="onMenuSelect"
      @close="openMenu = null"
    />
  </div>
</template>
