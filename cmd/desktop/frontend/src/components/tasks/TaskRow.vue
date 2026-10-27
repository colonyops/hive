<script setup lang="ts">
import { computed } from 'vue'
import IconBan from '~icons/lucide/ban'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconLayers from '~icons/lucide/layers'
import IconTerminal from '~icons/lucide/terminal'
import AppTooltip from '@hive/ui/components/ui/AppTooltip.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import type { TaskItem } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { relativeAge } from '../../lib/age'
import TaskStatusIcon from './TaskStatusIcon.vue'

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
const emit = defineEmits<{ select: [id: string]; toggle: [id: string] }>()

const isEpic = computed(() => props.item.type === 'epic')
// 18px per level reads clearly at up to ~4 levels deep without the row
// running out of room for its trailing chips.
const indent = computed(() => props.depth * 18 + 12)
</script>

<template>
  <div
    class="group flex h-9 min-w-0 cursor-pointer items-center gap-2 border-b border-row pr-4 text-body transition-colors"
    :class="selected ? 'bg-selection text-text' : 'text-text-2 hover:bg-row-hover hover:text-text'"
    :style="{ paddingLeft: indent + 'px' }"
    role="button"
    tabindex="0"
    :aria-selected="selected"
    :data-id="item.id"
    data-testid="task-row"
    @click="emit('select', item.id)"
    @keydown.enter.prevent="emit('select', item.id)"
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

    <TaskStatusIcon :status="item.status" data-testid="task-row-status" />

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

    <span
      class="w-9 shrink-0 text-right font-mono text-caption text-text-4"
      :title="new Date(item.updatedAt).toLocaleString()"
      data-testid="task-row-age"
      >{{ relativeAge(Date.parse(item.updatedAt)) }}</span
    >
  </div>
</template>
