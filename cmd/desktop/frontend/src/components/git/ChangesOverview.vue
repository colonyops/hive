<script setup lang="ts">
import { computed } from 'vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import IconArrowRight from '~icons/lucide/arrow-right'
import IconFileCode from '~icons/lucide/file-code'
import IconGitBranch from '~icons/lucide/git-branch'
import IconX from '~icons/lucide/x'
import AgentMark from '../AgentMark.vue'
import AgentMarkdown from './AgentMarkdown.vue'
import { relativeAgo } from '../../lib/age'
import { agentIconID } from '../../lib/agentIcon'
import { severityCounts, type AgentComment, type AgentSeverity } from '../../lib/agentReview'
import { commentLabel } from '../../lib/diffReview'
import type { DiffReviewWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

// The page a pull request has above its files: what each agent made of the
// changes.
const props = defineProps<{
  reviews: readonly DiffReviewWire[]
  /** Every review's comments, in the order triage walks them. */
  comments: readonly AgentComment[]
  /** Another comparison is on screen, so the agents' rows cannot be shown in place. */
  hidden: boolean
  isStale: (comment: AgentComment) => boolean
  branch: string
  base: string
  files: number
  additions: number
  deletions: number
}>()
const emit = defineEmits<{
  pick: [comment: AgentComment]
  walk: []
  'dismiss-all': [review: DiffReviewWire]
}>()

const SEVERITIES: AgentSeverity[] = ['issue', 'suggestion', 'note']
const PLURALS: Record<AgentSeverity, string> = { issue: 'issues', suggestion: 'suggestions', note: 'notes' }
const CHIPS: Record<AgentSeverity, string> = {
  issue: 'bg-severity-error-tint text-severity-error',
  suggestion: 'bg-severity-info-tint text-severity-info',
  note: 'bg-chip text-text-2',
}
const BARS: Record<string, string> = {
  issue: 'bg-severity-error',
  suggestion: 'bg-severity-info',
  note: 'bg-text-4',
}

const byReview = computed(() => {
  const out = new Map<string, AgentComment[]>()
  for (const comment of props.comments) out.set(comment.reviewId, [...(out.get(comment.reviewId) ?? []), comment])
  return out
})
const totals = computed(() => severityCounts(props.comments))
const tally = (review: DiffReviewWire) => severityCounts(byReview.value.get(review.id) ?? [])
const chipText = (severity: AgentSeverity, count: number) => `${count} ${count === 1 ? severity : PLURALS[severity]}`
const lines = (comment: AgentComment) =>
  commentLabel({ ...comment, kind: comment.kind === 'file' ? 'file' : undefined })
</script>

<template>
  <div class="hive-scroll h-full overflow-y-auto bg-app" data-testid="changes-overview">
    <div class="mx-auto flex max-w-3xl flex-col gap-5 px-6 pt-6 pb-10">
      <header class="flex flex-col gap-3 border-b border-border pb-5">
        <div class="flex items-start gap-3">
          <div class="flex min-w-0 flex-1 flex-col gap-1.5">
            <h2 class="text-heading font-semibold text-text">Overview</h2>
            <p class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-small text-text-3">
              <span class="flex min-w-0 items-center gap-1 font-mono text-text-2">
                <IconGitBranch class="size-3.5 shrink-0 text-text-4" aria-hidden="true" />
                <span class="truncate">{{ branch }}</span>
              </span>
              <span>into</span>
              <span class="font-mono text-text-2">{{ base }}</span>
              <span class="text-text-4">·</span>
              <span>{{ files }} {{ files === 1 ? 'file' : 'files' }}</span>
              <span class="font-mono">
                <span class="text-severity-success">+{{ additions }}</span>
                <span class="ml-1 text-severity-error">-{{ deletions }}</span>
              </span>
            </p>
          </div>
          <BaseButton
            v-if="comments.length && !hidden"
            variant="secondary"
            size="xs"
            title="Step through the agent comments in the diff (w)"
            data-testid="changes-overview-walk"
            @click="emit('walk')"
          >
            Walk through
            <IconArrowRight class="ml-1 size-3.5" aria-hidden="true" />
          </BaseButton>
        </div>
        <div v-if="reviews.length" class="flex flex-wrap items-center gap-1.5 text-caption">
          <span class="mr-1 text-text-3">
            {{ reviews.length }} {{ reviews.length === 1 ? 'review' : 'reviews' }} waiting on you
          </span>
          <template v-for="severity in SEVERITIES" :key="severity">
            <span
              v-if="totals[severity]"
              class="rounded-full px-2 py-0.5 font-medium"
              :class="CHIPS[severity]"
              :data-testid="`changes-overview-total-${severity}`"
              >{{ chipText(severity, totals[severity]) }}</span
            >
          </template>
        </div>
      </header>

      <p
        v-if="hidden && reviews.length"
        class="rounded-md border border-severity-warning-border bg-severity-warning-tint px-3 py-2 text-small text-text-2"
        data-testid="changes-overview-hidden"
      >
        Agents reviewed the branch against its base. Switch to that comparison to see their comments in place.
      </p>

      <div v-if="reviews.length" class="flex flex-col divide-y divide-border">
        <section
          v-for="review in reviews"
          :key="review.id"
          class="flex flex-col gap-3 py-5 first:pt-1"
          :data-testid="`agent-review-${review.id}`"
        >
          <div class="flex min-w-0 items-center gap-2.5">
            <span class="flex size-7 shrink-0 items-center justify-center rounded-md bg-chip">
              <AgentMark class="size-4!" :agent="agentIconID(review.agent || review.author)" />
            </span>
            <div class="flex min-w-0 flex-1 flex-col">
              <span class="truncate text-body font-semibold text-text">{{ review.author }}</span>
              <span class="flex items-center gap-1.5 text-caption text-text-3">
                reviewed {{ relativeAgo(new Date(review.createdAt).getTime()) }}
                <span class="text-text-4">·</span>
                <span class="font-mono" :title="review.head">{{ review.head.slice(0, 7) }}</span>
              </span>
            </div>
            <div class="hidden shrink-0 items-center gap-1 sm:flex">
              <template v-for="severity in SEVERITIES" :key="severity">
                <span
                  v-if="tally(review)[severity]"
                  class="rounded-full px-2 py-0.5 text-caption font-medium"
                  :class="CHIPS[severity]"
                  >{{ chipText(severity, tally(review)[severity]) }}</span
                >
              </template>
            </div>
            <IconButton
              label="Dismiss this review"
              :icon="IconX"
              :data-testid="`agent-review-${review.id}-dismiss-all`"
              @click="emit('dismiss-all', review)"
            />
          </div>

          <AgentMarkdown
            v-if="review.summary"
            class="pl-9.5 text-reading leading-relaxed text-text"
            :source="review.summary"
          />

          <ul v-if="byReview.get(review.id)?.length" class="ml-9.5 flex flex-col gap-2">
            <li v-for="comment in byReview.get(review.id)" :key="comment.id">
              <button
                type="button"
                class="group/row relative flex w-full cursor-pointer flex-col gap-1 rounded-md border border-card bg-raised py-2.5 pr-9 pl-6 text-left transition-colors hover:border-strong hover:bg-chip"
                :data-testid="`agent-row-${comment.id}`"
                @click="emit('pick', comment)"
              >
                <span
                  class="absolute top-2.5 bottom-2.5 left-2.5 w-[3px] rounded-full"
                  :class="BARS[comment.severity] ?? BARS.note"
                  :title="comment.severity"
                  :aria-label="comment.severity"
                />
                <span class="flex min-w-0 items-center gap-1.5 text-caption">
                  <IconFileCode class="size-3.5 shrink-0 text-text-4" aria-hidden="true" />
                  <span class="min-w-0 truncate font-mono text-text-2">{{ comment.path }}</span>
                  <span class="shrink-0 rounded-sm bg-chip px-1.5 font-mono text-micro text-text-3">{{
                    lines(comment)
                  }}</span>
                  <span
                    v-if="isStale(comment)"
                    class="flex shrink-0 items-center gap-1 text-severity-warning"
                    title="The rows under this comment changed after the agent wrote it"
                  >
                    <span class="size-1.5 rounded-full bg-severity-warning" aria-hidden="true" />
                    Lines changed
                  </span>
                </span>
                <AgentMarkdown class="text-body leading-relaxed text-text" :source="comment.text" />
                <IconArrowRight
                  class="absolute top-3 right-3 size-3.5 text-text-4 opacity-0 transition-opacity group-hover/row:opacity-100"
                  aria-hidden="true"
                />
              </button>
            </li>
          </ul>
        </section>
      </div>

      <div
        v-else
        class="flex flex-col items-center gap-1.5 rounded-lg border border-dashed border-card px-6 py-8 text-center"
        data-testid="changes-overview-empty"
      >
        <span class="text-body font-medium text-text-2">No agent reviews yet</span>
        <span class="max-w-md text-small text-text-3">
          Ask an agent to review this session. Its summary shows here and its comments under their lines.
        </span>
      </div>
    </div>
  </div>
</template>
