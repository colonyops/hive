<script setup lang="ts">
import IconFileDiff from '~icons/lucide/file-diff'
import IconFileText from '~icons/lucide/file-text'
import IconListTodo from '~icons/lucide/list-todo'
import IconTerminal from '~icons/lucide/terminal'
import claudeCodeMark from '../../assets/agents/claude-code.svg'
import codexMark from '../../assets/agents/codex.svg'
import piMark from '../../assets/agents/pi.svg'
import AgentIcon from '../AgentIcon.vue'
import type { AgentIconID } from '../../lib/agentIcon'
import type { SessionTab } from '../../lib/sessionLayout'

// One mark for a tab and for its tree row, so the two read as the same thing.
defineProps<{ kind: SessionTab['kind']; agent?: AgentIconID }>()

const MARKS: Partial<Record<AgentIconID, string>> = { claude: claudeCodeMark, codex: codexMark, pi: piMark }
const VIEWS = { window: IconTerminal, docs: IconFileText, diff: IconFileDiff, tasks: IconListTodo }
</script>

<template>
  <img
    v-if="agent && MARKS[agent]"
    :src="MARKS[agent]"
    alt=""
    draggable="false"
    class="size-3.5 shrink-0 rounded-sm"
    :data-agent-icon="agent"
  />
  <AgentIcon v-else-if="agent" :id="agent" class="size-3.5 shrink-0" />
  <component :is="VIEWS[kind]" v-else class="size-3.5 shrink-0" aria-hidden="true" />
</template>
