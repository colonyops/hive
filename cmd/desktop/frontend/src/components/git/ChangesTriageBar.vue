<script setup lang="ts">
import { computed } from 'vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import AgentMark from '../AgentMark.vue'
import { agentIconID } from '../../lib/agentIcon'
import type { AgentComment } from '../../lib/agentReview'
import { commentLabel } from '../../lib/diffReview'
import { gitFileName } from '../../lib/gitTree'

const props = defineProps<{ comment: AgentComment; index: number; total: number }>()
const emit = defineEmits<{ prev: []; next: []; adopt: []; edit: []; dismiss: []; stop: [] }>()

const label = computed(() =>
  commentLabel({ ...props.comment, kind: props.comment.kind === 'file' ? 'file' : undefined }),
)
</script>

<template>
  <div
    class="pointer-events-auto flex max-w-[calc(100%-1.5rem)] flex-wrap items-center gap-1.5 rounded-lg border border-node-purple/45 bg-raised px-2.5 py-1.5 text-caption text-text-2 shadow-lg"
    role="toolbar"
    aria-label="Triage the agent review"
    data-testid="changes-triage"
  >
    <span class="font-mono text-micro text-node-purple" data-testid="changes-triage-position"
      >{{ index + 1 }} / {{ total }}</span
    >
    <span class="flex items-center gap-1 font-semibold text-node-purple">
      <AgentMark :agent="agentIconID(comment.agent || comment.author)" />
      {{ comment.author }}
    </span>
    <span class="min-w-0 truncate font-mono text-micro text-text-3" :title="comment.path">
      {{ gitFileName(comment.path) }} {{ label }}
    </span>
    <BaseButton variant="ghost" size="xs" data-testid="changes-triage-prev" @click="emit('prev')">
      Prev<span class="ml-1 font-mono opacity-60">p</span>
    </BaseButton>
    <BaseButton variant="ghost" size="xs" data-testid="changes-triage-next" @click="emit('next')">
      Next<span class="ml-1 font-mono opacity-60">n</span>
    </BaseButton>
    <BaseButton size="xs" data-testid="changes-triage-adopt" @click="emit('adopt')">
      Adopt<span class="ml-1 font-mono opacity-60">a</span>
    </BaseButton>
    <BaseButton variant="ghost" size="xs" data-testid="changes-triage-edit" @click="emit('edit')">
      Edit<span class="ml-1 font-mono opacity-60">e</span>
    </BaseButton>
    <BaseButton variant="ghost" size="xs" data-testid="changes-triage-dismiss" @click="emit('dismiss')">
      Dismiss<span class="ml-1 font-mono opacity-60">d</span>
    </BaseButton>
    <BaseButton variant="ghost" size="xs" data-testid="changes-triage-stop" @click="emit('stop')">
      Stop<span class="ml-1 font-mono opacity-60">esc</span>
    </BaseButton>
  </div>
</template>
