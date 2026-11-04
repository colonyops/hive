<script setup lang="ts">
import { computed } from 'vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import AgentMark from '../AgentMark.vue'
import AgentSeverity from './AgentSeverity.vue'
import { relativeAgo } from '../../lib/age'
import { agentIconID } from '../../lib/agentIcon'
import { severityCounts, type AgentComment, type AgentSeverity as Severity } from '../../lib/agentReview'
import { commentLabel } from '../../lib/diffReview'
import { gitFileName } from '../../lib/gitTree'
import type { DiffReviewWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  reviews: readonly DiffReviewWire[]
  /** Every review's comments, in the order triage walks them. */
  comments: readonly AgentComment[]
  focused: string | null
  triaging: boolean
  /** Comments that cannot be drawn here: another comparison is on screen. */
  hidden: boolean
  isStale: (comment: AgentComment) => boolean
}>()
const emit = defineEmits<{
  pick: [comment: AgentComment]
  triage: []
  stop: []
  'adopt-all': [review: DiffReviewWire]
  'dismiss-all': [review: DiffReviewWire]
}>()

const SEVERITIES: Severity[] = ['issue', 'suggestion', 'note']

function reviewComments(review: DiffReviewWire): AgentComment[] {
  return props.comments.filter((comment) => comment.reviewId === review.id)
}

const shortHead = (head: string) => head.slice(0, 7)
const label = (comment: AgentComment) =>
  commentLabel({ ...comment, kind: comment.kind === 'file' ? 'file' : undefined })
const marks = computed(
  () => new Map(props.reviews.map((review) => [review.id, agentIconID(review.agent || review.author)])),
)
</script>

<template>
  <section class="border-b border-border pb-1" aria-label="Agent reviews" data-testid="agent-reviews">
    <div class="flex h-8 items-center gap-1.5 px-2.5">
      <span class="font-mono text-micro tracking-[0.12em] text-text-4 uppercase">Agent reviews</span>
      <span class="flex-1" />
      <template v-if="comments.length && !hidden">
        <BaseButton v-if="!triaging" size="xs" data-testid="agent-reviews-triage" @click="emit('triage')">
          Start triage {{ comments.length }}<span class="ml-1 font-mono opacity-60">t</span>
        </BaseButton>
        <BaseButton v-else variant="ghost" size="xs" data-testid="agent-reviews-stop" @click="emit('stop')">
          Stop triage<span class="ml-1 font-mono opacity-60">esc</span>
        </BaseButton>
      </template>
    </div>
    <p v-if="hidden" class="px-2.5 pb-1.5 text-caption text-text-3" data-testid="agent-reviews-hidden">
      Agents reviewed the branch against its base. Switch to that comparison to see their comments in place.
    </p>
    <div
      v-for="review in reviews"
      :key="review.id"
      class="mx-2.5 mb-1.5 rounded-md border border-node-purple/45 bg-node-purple-tint px-2 py-1.5 text-caption"
      :data-testid="`agent-review-${review.id}`"
    >
      <div class="flex min-w-0 items-center gap-1.5 text-micro text-text-3">
        <span class="flex shrink-0 items-center gap-1 text-caption font-semibold text-node-purple">
          <AgentMark :agent="marks.get(review.id)" />
          {{ review.author }}
        </span>
        <span class="truncate">{{ relativeAgo(new Date(review.createdAt).getTime()) }}</span>
        <span class="shrink-0 font-mono" :title="review.head">{{ shortHead(review.head) }}</span>
      </div>
      <p v-if="review.summary" class="mt-0.5 leading-snug break-words whitespace-pre-wrap text-text">
        {{ review.summary }}
      </p>
      <div class="mt-1 flex flex-wrap items-center gap-1">
        <template v-for="severity in SEVERITIES" :key="severity">
          <AgentSeverity
            v-if="severityCounts(reviewComments(review))[severity]"
            :severity="severity"
            :count="severityCounts(reviewComments(review))[severity]"
          />
        </template>
      </div>
      <div class="mt-1.5 flex gap-1.5">
        <BaseButton
          v-if="reviewComments(review).length"
          size="xs"
          :data-testid="`agent-review-${review.id}-adopt-all`"
          @click="emit('adopt-all', review)"
        >
          Adopt all {{ reviewComments(review).length }}
        </BaseButton>
        <BaseButton
          variant="ghost"
          size="xs"
          :data-testid="`agent-review-${review.id}-dismiss-all`"
          @click="emit('dismiss-all', review)"
        >
          Dismiss {{ reviewComments(review).length ? 'all' : 'review' }}
        </BaseButton>
      </div>
    </div>
    <button
      v-for="comment in comments"
      :key="comment.id"
      type="button"
      class="flex w-full cursor-pointer items-start gap-1.5 border-t border-row px-2.5 py-1.5 text-left text-caption hover:bg-chip"
      :class="focused === comment.id ? 'bg-node-purple-tint shadow-[inset_2px_0_0_var(--color-node-purple)]' : ''"
      :data-testid="`agent-row-${comment.id}`"
      @click="emit('pick', comment)"
    >
      <AgentMark class="mt-0.5" :agent="agentIconID(comment.agent || comment.author)" />
      <span class="flex min-w-0 flex-1 flex-col gap-0.5">
        <span class="flex min-w-0 items-center gap-1.5 font-mono text-micro text-text-3">
          <span class="min-w-0 truncate text-text-2">{{ gitFileName(comment.path) }}</span>
          <span class="shrink-0">{{ label(comment) }}</span>
          <AgentSeverity :severity="comment.severity" />
          <span
            v-if="isStale(comment)"
            class="size-1.5 shrink-0 rounded-full bg-severity-warning"
            title="Lines changed"
          />
        </span>
        <span class="line-clamp-2 text-text" :title="comment.text">{{ comment.text }}</span>
      </span>
    </button>
  </section>
</template>
