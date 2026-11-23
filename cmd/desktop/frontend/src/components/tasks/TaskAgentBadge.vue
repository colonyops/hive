<script setup lang="ts">
import { computed } from 'vue'
import IconCircleAlert from '~icons/lucide/circle-alert'
import IconCircleCheck from '~icons/lucide/circle-check'
import IconLoaderCircle from '~icons/lucide/loader-circle'
import IconTerminal from '~icons/lucide/terminal'
import AppTooltip from '@hive/ui/components/ui/AppTooltip.vue'
import { agentLabel, type TaskAgent } from '../../lib/taskAgent'

const props = withDefaults(
  defineProps<{
    agent: TaskAgent
    /**
     * False for a closed task: its session may still run, but on other work,
     * so the badge shows only who held the task.
     */
    live?: boolean
    testid?: string
  }>(),
  { live: true, testid: 'task-agent' },
)

const activity = computed(() => (props.live ? props.agent.activity : 'unknown'))

const look = computed(() => {
  switch (activity.value) {
    case 'active':
      return { icon: IconLoaderCircle, spin: true, classes: 'bg-severity-success-tint text-severity-success' }
    case 'approval':
      return { icon: IconCircleAlert, spin: false, classes: 'bg-severity-warning-tint text-severity-warning' }
    case 'ready':
      return { icon: IconCircleCheck, spin: false, classes: 'border border-card text-text-2' }
    case 'stopped':
      return { icon: IconTerminal, spin: false, classes: 'border border-dashed border-card text-text-4' }
    default:
      return { icon: IconTerminal, spin: false, classes: 'border border-card text-text-3' }
  }
})

const label = computed(() => agentLabel({ ...props.agent, activity: activity.value }))
</script>

<template>
  <AppTooltip :text="label">
    <span
      class="inline-flex min-w-0 max-w-40 shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-caption"
      :class="look.classes"
      :aria-label="label"
      :data-activity="activity"
      :data-testid="testid"
    >
      <component :is="look.icon" class="size-3 shrink-0" :class="{ 'animate-spin': look.spin }" aria-hidden="true" />
      <span class="truncate">{{ agent.name }}</span>
    </span>
  </AppTooltip>
</template>
