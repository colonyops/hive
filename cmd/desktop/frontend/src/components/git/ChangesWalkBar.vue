<script setup lang="ts">
import { computed } from 'vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronUp from '~icons/lucide/chevron-up'
import IconX from '~icons/lucide/x'
import AgentMark from '../AgentMark.vue'
import { agentIconID } from '../../lib/agentIcon'
import type { AgentComment } from '../../lib/agentReview'
import { commentLabel } from '../../lib/diffReview'
import { gitFileName } from '../../lib/gitTree'

const props = defineProps<{ comment: AgentComment; index: number; total: number }>()
const emit = defineEmits<{ prev: []; next: []; reply: []; dismiss: []; stop: [] }>()

const label = computed(() =>
  commentLabel({ ...props.comment, kind: props.comment.kind === 'file' ? 'file' : undefined }),
)
</script>

<template>
  <div
    class="pointer-events-auto flex max-w-[calc(100%-1.5rem)] items-center gap-2 rounded-lg border border-border bg-raised py-1.5 pr-1.5 pl-3 text-small text-text-2 shadow-lg"
    role="toolbar"
    aria-label="Walk through the agent comments"
    data-testid="changes-walk"
  >
    <span class="shrink-0 text-text-3" data-testid="changes-walk-position">Comment {{ index + 1 }} of {{ total }}</span>
    <span class="flex shrink-0 items-center gap-1 font-medium text-text">
      <AgentMark :agent="agentIconID(comment.agent || comment.author)" />
      {{ comment.author }}
    </span>
    <span class="min-w-0 truncate font-mono text-caption text-text-3" :title="comment.path">
      {{ gitFileName(comment.path) }} {{ label }}
    </span>
    <span class="flex shrink-0 items-center gap-0.5">
      <IconButton
        label="Previous comment (p)"
        :icon="IconChevronUp"
        data-testid="changes-walk-prev"
        @click="emit('prev')"
      />
      <IconButton
        label="Next comment (n)"
        :icon="IconChevronDown"
        data-testid="changes-walk-next"
        @click="emit('next')"
      />
      <BaseButton variant="ghost" size="xs" data-testid="changes-walk-reply" @click="emit('reply')">
        Reply<span class="ml-1 font-mono opacity-60">r</span>
      </BaseButton>
      <BaseButton variant="ghost" size="xs" data-testid="changes-walk-dismiss" @click="emit('dismiss')">
        Dismiss<span class="ml-1 font-mono opacity-60">d</span>
      </BaseButton>
      <IconButton label="Stop (esc)" :icon="IconX" data-testid="changes-walk-stop" @click="emit('stop')" />
    </span>
  </div>
</template>
