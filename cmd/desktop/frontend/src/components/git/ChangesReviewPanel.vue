<script setup lang="ts">
import { computed } from 'vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import PanelResizeHandle from '@hive/ui/components/ui/PanelResizeHandle.vue'
import { useResizablePanel } from '@hive/ui/composables/useResizablePanel'
import DiffCommentCard from './DiffCommentCard.vue'
import { useChangesReview, type ChangesComment } from '../../stores/useChangesReview'

const props = defineProps<{
  sessionId: string
  isOutdated: (comment: ChangesComment) => boolean
}>()
const emit = defineEmits<{ pick: [comment: ChangesComment] }>()

const {
  size: width,
  startResize,
  step,
} = useResizablePanel({ storageKey: 'hive.panel.changes.comments', defaultSize: 220, min: 170, max: 480, edge: 'left' })

const store = useChangesReview()
const comments = computed(() => store.comments(props.sessionId))
</script>

<template>
  <aside
    class="relative flex shrink-0 flex-col border-l border-border bg-pane"
    :style="{ width: `${width}px` }"
    aria-label="Review comments"
    data-testid="changes-review"
  >
    <PanelResizeHandle edge="left" name="changes-review" :start="startResize" :step="step" />
    <div class="hive-scroll min-h-0 flex-1 overflow-y-auto">
      <DiffCommentCard
        v-for="comment in comments"
        :key="comment.id"
        :comment="comment"
        row
        :outdated="isOutdated(comment)"
        @pick="emit('pick', comment)"
        @update="store.update(sessionId, comment.id, $event)"
        @remove="store.remove(sessionId, comment.id)"
      />
      <EmptyState
        v-if="!comments.length"
        variant="inline"
        class="p-2.5"
        message="Drag across line numbers or select code to comment."
        data-testid="changes-review-empty"
      />
    </div>
  </aside>
</template>
