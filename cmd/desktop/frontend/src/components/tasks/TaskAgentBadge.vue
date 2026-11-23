<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
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

const router = useRouter()

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
const openable = computed(() => props.agent.slug !== '')
const tooltip = computed(() => (openable.value ? `${label.value}. Click to open the session.` : label.value))

// A live chip opens the window whose agent it shows; a closed task's chip
// opens the session, since that window may be on other work by now.
function open(): void {
  const query: Record<string, string> = {}
  if (props.live && props.agent.windowId) query.window = props.agent.windowId
  void router.push({ name: 'terminal', params: { slug: props.agent.slug }, query })
}
</script>

<template>
  <AppTooltip :text="tooltip">
    <component
      :is="openable ? 'button' : 'span'"
      :type="openable ? 'button' : undefined"
      class="inline-flex min-w-0 max-w-40 shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-caption"
      :class="[
        look.classes,
        openable
          ? 'group cursor-pointer hover:brightness-110 focus-visible:outline-2 focus-visible:outline-accent'
          : '',
      ]"
      :aria-label="openable ? `${label}. Open the session` : label"
      :data-activity="activity"
      :data-testid="testid"
      @click.stop="openable && open()"
      @keydown.enter.stop
    >
      <component :is="look.icon" class="size-3 shrink-0" :class="{ 'animate-spin': look.spin }" aria-hidden="true" />
      <span class="truncate" :class="{ 'group-hover:underline': openable }">{{ agent.name }}</span>
    </component>
  </AppTooltip>
</template>
