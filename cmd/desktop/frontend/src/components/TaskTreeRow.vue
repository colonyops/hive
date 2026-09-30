<script setup lang="ts">
// One row in the tasks tree (TasksView). Depth/expand state are computed by
// the parent — buildTaskTree + collapsedIds — and handed down as props, so
// this component stays a pure row: it never re-derives tree structure or
// touches useTasks() itself.
import { computed } from 'vue'
import IconBan from '~icons/lucide/ban'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconCircleDot from '~icons/lucide/circle-dot'
import IconLayers from '~icons/lucide/layers'
import IconTerminal from '~icons/lucide/terminal'
import { relativeAge } from '../lib/age'
import { statusMeta, type TaskTreeNode } from '../lib/tasksPresentation'

const props = defineProps<{
  node: TaskTreeNode
  depth: number
  selected: boolean
  /** Whether this node's children are hidden — meaningless when it has none. */
  collapsed: boolean
  /** Display name for the linked session; absent when it isn't loaded (e.g. ended). */
  sessionName?: string
}>()
const emit = defineEmits<{ select: [id: string]; toggle: [id: string] }>()

const isEpic = computed(() => props.node.item.type === 'epic')
const status = computed(() => statusMeta(props.node.item.status))
// 18px per level reads clearly at up to ~4 levels deep without the row
// running out of room for its trailing chips.
const indent = computed(() => props.depth * 18 + 10)
</script>

<template>
  <div
    class="flex min-w-0 cursor-pointer items-center gap-1.5 py-[7px] pr-3 text-[12.5px] transition-colors"
    :class="selected ? 'bg-selection text-text' : 'text-text-2 hover:bg-row-hover hover:text-text'"
    :style="{ paddingLeft: indent + 'px' }"
    role="button"
    tabindex="0"
    :data-id="node.item.id"
    data-testid="task-tree-row"
    @click="emit('select', node.item.id)"
    @keydown.enter.prevent="emit('select', node.item.id)"
  >
    <button
      v-if="node.children.length"
      type="button"
      class="flex size-4 shrink-0 items-center justify-center text-text-4 hover:text-text"
      data-testid="task-tree-toggle"
      @click.stop="emit('toggle', node.item.id)"
    ><component :is="collapsed ? IconChevronRight : IconChevronDown" class="size-3" /></button>
    <span v-else class="size-4 shrink-0" aria-hidden="true" />

    <component :is="isEpic ? IconLayers : IconCircleDot" class="size-3.5 shrink-0" :class="isEpic ? 'text-accent' : 'text-text-4'" aria-hidden="true" />

    <span class="min-w-0 flex-1 truncate">{{ node.item.title }}</span>

    <span v-if="isEpic" class="shrink-0 font-mono text-[10.5px] text-text-4" data-testid="task-tree-counts">[{{ node.counts.done }}/{{ node.counts.total }}]</span>

    <span v-if="node.item.blocked" class="flex shrink-0 items-center gap-1 rounded-[5px] bg-severity-error-tint px-1.5 py-0.5 text-[10px] font-medium text-severity-error" data-testid="task-tree-blocked">
      <IconBan class="size-2.5" aria-hidden="true" />Blocked
    </span>

    <span
      v-if="node.item.sessionId"
      class="flex shrink-0 items-center justify-center rounded-[5px] bg-chip px-1 py-0.5 text-text-3"
      :title="`Linked to session ${sessionName || node.item.sessionId}`"
      data-testid="task-tree-session"
    ><IconTerminal class="size-2.5" aria-hidden="true" /></span>

    <span class="shrink-0 rounded-[5px] px-1.5 py-0.5 text-[10px] font-medium" :class="status.classes" data-testid="task-tree-status">{{ status.label }}</span>

    <span class="w-9 shrink-0 text-right font-mono text-[10.5px] text-text-4" :title="new Date(node.item.updatedAt).toLocaleString()" data-testid="task-tree-age">{{ relativeAge(Date.parse(node.item.updatedAt)) }}</span>
  </div>
</template>
