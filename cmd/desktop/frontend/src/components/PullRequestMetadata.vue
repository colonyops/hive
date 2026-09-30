<script setup lang="ts">
import { computed } from 'vue'
import { pullRequestMetadata } from '../lib/itemPresentation'
import IconBadgeCheck from '~icons/lucide/badge-check'
import IconCheck from '~icons/lucide/check'
import IconGitPullRequestDraft from '~icons/lucide/git-pull-request-draft'
import IconLoader from '~icons/lucide/loader'
import IconMessageCircleWarning from '~icons/lucide/message-circle-warning'
import IconX from '~icons/lucide/x'
import type { InboxItem } from '../types/feed'

const props = withDefaults(defineProps<{ item: InboxItem; mode?: 'full' | 'status' | 'review' | 'changes' }>(), { mode: 'full' })

const metadata = computed(() => pullRequestMetadata(props.item))
const ciLabel = computed(() => {
  switch (metadata.value?.ci) {
    case 'passing': return 'Checks pass'
    case 'pending': return 'Checks pending'
    case 'failing': return 'Checks fail'
    default: return ''
  }
})
const ciIcon = computed(() => {
  switch (metadata.value?.ci) {
    case 'passing': return IconCheck
    case 'failing': return IconX
    default: return IconLoader
  }
})
const ciIconMotion = computed(() => metadata.value?.ci === 'pending' ? 'motion-safe:animate-spin motion-safe:[animation-duration:1.8s]' : '')
const ciTone = computed(() => {
  switch (metadata.value?.ci) {
    case 'passing': return 'text-severity-success/80'
    case 'failing': return 'text-severity-error/80'
    default: return 'text-severity-warning/80'
  }
})
const reviewLabel = computed(() => {
  switch (metadata.value?.review) {
    case 'approved': return 'Review approved'
    case 'changes_requested': return 'Changes requested'
    case 'draft': return 'Draft pull request'
    case 'review_required': return 'Review needed'
    default: return ''
  }
})
const reviewShortLabel = computed(() => {
  switch (metadata.value?.review) {
    case 'approved': return 'Approved'
    case 'draft': return 'Draft'
    default: return reviewLabel.value
  }
})
const reviewIcon = computed(() => {
  switch (metadata.value?.review) {
    case 'approved': return IconBadgeCheck
    case 'changes_requested': return IconMessageCircleWarning
    default: return IconGitPullRequestDraft
  }
})
const reviewTone = computed(() => {
  switch (metadata.value?.review) {
    case 'approved': return 'text-severity-success/80'
    case 'changes_requested': return 'text-severity-warning/80'
    case 'review_required': return 'text-accent/80'
    case 'draft': return 'text-text-2'
    default: return 'text-text-3'
  }
})
const hasChanges = computed(() => metadata.value?.additions != null || metadata.value?.deletions != null)
const showStatus = computed(() => props.mode !== 'changes' && props.mode !== 'review' && ciLabel.value !== '')
const showReview = computed(() => (props.mode === 'full' || props.mode === 'review') && reviewLabel.value !== '')
const showChanges = computed(() => (props.mode === 'full' || props.mode === 'changes') && hasChanges.value)
const visible = computed(() => showStatus.value || showReview.value || showChanges.value)
</script>

<template>
  <span v-if="metadata && visible" class="inline-flex shrink-0 items-center gap-3 font-mono text-[10.5px]" data-testid="pr-metadata">
    <span v-if="showStatus" class="inline-flex shrink-0 items-center gap-1.5" :class="ciTone" :aria-label="ciLabel" :title="ciLabel" data-testid="pr-ci">
      <component :is="ciIcon" class="size-3.5" :class="ciIconMotion" :stroke-width="2.5" aria-hidden="true" />
      <span v-if="mode === 'full'">{{ ciLabel }}</span>
    </span>
    <span v-if="showReview" class="inline-flex shrink-0 items-center gap-1.5" :class="reviewTone" :aria-label="reviewLabel" :title="reviewLabel" data-testid="pr-review">
      <component :is="reviewIcon" v-if="mode === 'full'" class="size-3.5" aria-hidden="true" />
      <span>{{ mode === 'review' ? reviewShortLabel : reviewLabel }}</span>
    </span>
    <span v-if="showChanges" class="inline-flex shrink-0 items-center gap-1" aria-label="Pull request line changes" data-testid="pr-lines">
      <span class="text-severity-success/70">+{{ metadata.additions ?? 0 }}</span>
      <span class="text-severity-error/70">−{{ metadata.deletions ?? 0 }}</span>
    </span>
  </span>
</template>
