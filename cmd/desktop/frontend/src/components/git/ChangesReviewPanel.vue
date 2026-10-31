<script setup lang="ts">
import { computed } from 'vue'
import IconCopy from '~icons/lucide/copy'
import IconRotateCcw from '~icons/lucide/rotate-ccw'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import PanelResizeHandle from '@hive/ui/components/ui/PanelResizeHandle.vue'
import { useResizablePanel } from '@hive/ui/composables/useResizablePanel'
import DiffCommentCard from './DiffCommentCard.vue'
import { useCopyReview } from '../../composables/useCopyReview'
import { relativeAgo } from '../../lib/age'
import { useChangesReview, type ChangesComment, type SentReview } from '../../stores/useChangesReview'

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
const sent = computed(() => store.sentReviews(props.sessionId))
const copyReview = useCopyReview()

function sentLabel(review: SentReview): string {
  return review.comments.length === 1 ? '1 comment' : `${review.comments.length} comments`
}

function sentFiles(review: SentReview): string {
  return [...new Set(review.comments.map((comment) => comment.path))].join('\n')
}
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
    <div v-if="sent.length" class="shrink-0 border-t border-border py-1" data-testid="changes-sent">
      <div class="px-2.5 py-0.5 font-mono text-micro tracking-[0.12em] text-text-4 uppercase">Sent</div>
      <div
        v-for="(review, index) in sent"
        :key="review.id"
        class="flex h-6 items-center gap-1 px-2.5 text-caption text-text-3"
        :title="sentFiles(review)"
        :data-testid="`changes-sent-${index}`"
      >
        <span class="min-w-0 truncate">{{ sentLabel(review) }}</span>
        <span class="shrink-0 text-text-4">{{ relativeAgo(review.sentAt) }}</span>
        <span class="ml-auto flex shrink-0 items-center gap-0.5">
          <IconButton
            label="Copy these comments"
            :icon="IconCopy"
            size="sm"
            :data-testid="`changes-sent-${index}-copy`"
            @click="copyReview(review.comments)"
          />
          <IconButton
            label="Restore these comments"
            :icon="IconRotateCcw"
            size="sm"
            :data-testid="`changes-sent-${index}-restore`"
            @click="store.restore(sessionId, review.id)"
          />
        </span>
      </div>
    </div>
  </aside>
</template>
