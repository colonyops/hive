<script setup lang="ts">
import IconUserRound from '~icons/lucide/user-round'
import AgentMark from '../AgentMark.vue'
import type { AgentIconID } from '../../lib/agentIcon'

/** One message in a diff thread. With no agent, the author is the user. */
defineProps<{ agent?: AgentIconID | 'user' }>()
</script>

<template>
  <div class="group/message grid grid-cols-[1.5rem_minmax(0,1fr)] gap-x-2.5 py-2">
    <span class="mt-px flex size-6 items-center justify-center rounded-full bg-chip text-text-2" aria-hidden="true">
      <IconUserRound v-if="agent === 'user'" class="size-3.5" />
      <AgentMark v-else :agent="agent" size="md" />
    </span>
    <div class="flex min-w-0 flex-col gap-0.5">
      <div class="flex h-6 min-w-0 items-center gap-2 text-small">
        <slot name="header" />
      </div>
      <slot />
    </div>
  </div>
</template>
