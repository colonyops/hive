<script setup lang="ts">
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import type { TaskItem } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { useDragReorder } from '../../composables/useDragReorder'
import { statusMeta, type TaskBoardColumn, type TaskStatus } from '../../lib/tasksPresentation'
import TaskBoardCard from './TaskBoardCard.vue'
import TaskStatusIcon from './TaskStatusIcon.vue'

defineProps<{
  columns: TaskBoardColumn[]
  selectedId: string | null
  countsById: Map<string, { done: number; total: number }>
  parentTitleById: Map<string, string>
  sessionNameById: Map<string, string>
}>()
const emit = defineEmits<{
  select: [id: string]
  move: [item: TaskItem, status: TaskStatus]
  showMore: [status: TaskStatus]
}>()

const drag = useDragReorder<TaskItem, TaskStatus>({
  mime: 'application/x-hive-task',
  payload: (item) => item.id,
  onDrop: (item, status) => {
    if (item.status !== status) emit('move', item, status)
  },
})
</script>

<template>
  <div class="hive-scroll flex min-h-0 flex-1 gap-3 overflow-x-auto bg-app p-4" data-testid="tasks-board">
    <section
      v-for="column in columns"
      :key="column.status"
      class="flex min-h-0 min-w-[280px] max-w-[400px] flex-1 flex-col rounded-xl border transition-colors"
      :class="
        drag.dragging.value && drag.target.value === column.status
          ? 'border-accent bg-accent-tint'
          : 'border-transparent bg-sidebar'
      "
      :aria-label="statusMeta(column.status).label"
      :data-status="column.status"
      :data-testid="`tasks-board-column-${column.status}`"
      @dragover.prevent="drag.over($event, column.status)"
      @drop.prevent="drag.drop"
    >
      <header class="flex shrink-0 items-center gap-2 px-3 pb-2 pt-3">
        <TaskStatusIcon :status="column.status" />
        <span class="text-body font-medium text-text">{{ statusMeta(column.status).label }}</span>
        <span class="font-mono text-caption text-text-4" data-testid="tasks-board-column-count">{{
          column.total
        }}</span>
      </header>
      <div class="hive-scroll flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-2 pb-2">
        <TaskBoardCard
          v-for="item in column.items"
          :key="item.id"
          :item="item"
          :selected="item.id === selectedId"
          :dragging="drag.dragging.value?.id === item.id"
          :counts="countsById.get(item.id)"
          :parent-title="parentTitleById.get(item.parentId)"
          :session-name="sessionNameById.get(item.sessionId)"
          @select="emit('select', $event)"
          @dragstart="drag.start($event, item)"
          @dragend="drag.end"
        />
        <div
          v-if="!column.total"
          class="rounded-lg border border-dashed border-card px-3 py-6 text-center text-caption text-text-4"
          data-testid="tasks-board-column-empty"
        >
          No {{ statusMeta(column.status).label.toLowerCase() }} tasks
        </div>
        <BaseButton
          v-if="column.items.length < column.total"
          variant="ghost"
          size="xs"
          class="self-center"
          data-testid="tasks-board-show-more"
          @click="emit('showMore', column.status)"
        >
          Show {{ Math.min(column.total - column.items.length, 50) }} more of {{ column.total - column.items.length }}
        </BaseButton>
      </div>
    </section>
  </div>
</template>
