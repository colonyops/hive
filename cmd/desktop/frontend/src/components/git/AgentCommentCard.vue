<script setup lang="ts">
import { computed } from 'vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import IconX from '~icons/lucide/x'
import AgentMark from '../AgentMark.vue'
import AgentMarkdown from './AgentMarkdown.vue'
import { agentIconID } from '../../lib/agentIcon'
import type { AgentComment } from '../../lib/agentReview'
import { commentLabel } from '../../lib/diffReview'

const props = defineProps<{
  comment: AgentComment
  /** The rows under it no longer read as the agent quoted them. */
  stale?: boolean
  /** The walkthrough is on this comment. */
  focused?: boolean
}>()
const emit = defineEmits<{ dismiss: []; pick: [] }>()

const agent = computed(() => agentIconID(props.comment.agent || props.comment.author))
const label = computed(() =>
  commentLabel({ ...props.comment, kind: props.comment.kind === 'file' ? 'file' : undefined }),
)
const testid = computed(() => `agent-comment-${props.comment.id}`)
// Inset from the card's edges, so the rounded corners stay whole.
const BARS: Record<string, string> = {
  issue: 'bg-severity-error',
  suggestion: 'bg-severity-info',
  note: 'bg-text-4',
}
const bar = computed(() => BARS[props.comment.severity] ?? BARS.note)
</script>

<template>
  <!-- A pointer press here must not reach CodeView, which would start a line selection under the card. -->
  <div
    class="group relative flex max-w-3xl flex-col gap-1.5 rounded-md border bg-raised py-2 pr-3 pl-5 font-sans text-body"
    :class="focused ? 'border-accent shadow-[0_0_0_1px_var(--color-accent)]' : 'border-card'"
    :data-testid="testid"
    :data-focused="focused || undefined"
    @pointerdown.stop
    @click.stop
  >
    <span
      class="absolute top-2 bottom-2 left-2 w-[3px] rounded-full"
      :class="bar"
      :title="comment.severity"
      :aria-label="comment.severity"
      :data-testid="`${testid}-bar`"
    />
    <div class="flex h-6 min-w-0 items-center gap-2 text-small">
      <span class="flex shrink-0 items-center gap-1.5 font-medium text-text">
        <AgentMark :agent="agent" />
        {{ comment.author }}
      </span>
      <span
        v-if="stale"
        class="size-1.5 shrink-0 rounded-full bg-severity-warning"
        title="The rows under this comment changed after the agent wrote it"
        aria-label="Lines changed"
        :data-testid="`${testid}-stale`"
      />
      <button
        type="button"
        class="min-w-0 cursor-pointer truncate font-mono text-caption text-text-3 hover:text-text"
        :title="`${comment.path} ${label}`"
        :data-testid="`${testid}-pick`"
        @click="emit('pick')"
      >
        {{ label }}
      </button>
      <IconButton
        class="ml-auto"
        :class="focused ? '' : 'opacity-0 group-focus-within:opacity-100 group-hover:opacity-100'"
        :label="focused ? 'Dismiss (d)' : 'Dismiss'"
        :icon="IconX"
        size="md"
        :data-testid="`${testid}-dismiss`"
        @click="emit('dismiss')"
      />
    </div>
    <AgentMarkdown class="leading-relaxed text-text" :source="comment.text" />
  </div>
</template>
