<script setup lang="ts">
import PanelResizeHandle from '@hive/ui/components/ui/PanelResizeHandle.vue'
import { useResizablePanel } from '@hive/ui/composables/useResizablePanel'
import { TASK_DETAIL_PANEL } from '../../lib/tasksPresentation'
import TaskDetail from './TaskDetail.vue'

const {
  size: paneWidth,
  startResize,
  step,
} = useResizablePanel({
  ...TASK_DETAIL_PANEL,
  edge: 'left',
})
</script>

<template>
  <aside
    class="relative flex min-h-0 shrink-0 flex-col bg-pane"
    :style="{ width: paneWidth + 'px' }"
    data-testid="task-detail-pane"
  >
    <PanelResizeHandle edge="left" name="taskdetail" :start="startResize" :step="step" />
    <TaskDetail />
  </aside>
</template>
