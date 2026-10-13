<script setup lang="ts">
// Picks one running agent to receive a document's feedback. Go lists only
// agents that can take text now and resolves the pick again on send.
import { computed, onMounted, ref } from 'vue'
import IconSend from '~icons/lucide/send'
import BaseButton from '../ui/BaseButton.vue'
import BaseModal from '../ui/BaseModal.vue'
import EmptyState from '../ui/EmptyState.vue'
import InlineError from '../ui/InlineError.vue'
import Spinner from '../ui/Spinner.vue'
import { useToasts } from '../../composables/useToasts'
import { activityIndicator } from '../../lib/agentActivity'
import { errorText } from '../../lib/appError'
import { useDriveReview } from '../../stores/useDriveReview'
import { AgentTargets } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reviewservice'
import type {
  AgentTarget,
  AgentTargetRef,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const props = defineProps<{
  space: string
  path: string
  /** The agent of the surface the drive is open beside, listed first. A session ref with no pane matches any of its panes. */
  agent?: AgentTargetRef | null
  /** Derives `-send-dialog`, `-send-target-<index>`, `-send-warning`, `-send-error`, `-send-empty`, and `-send-submit`. */
  testid: string
}>()
const emit = defineEmits<{ close: []; sent: [] }>()

const review = useDriveReview()
const { showToast } = useToasts()

const targets = ref<AgentTarget[]>([])
const loading = ref(true)
const loadError = ref<string | null>(null)
const sending = ref(false)
const sendError = ref<string | null>(null)
const picked = ref<AgentTarget | null>(null)

function preferred(target: AgentTarget): boolean {
  const agent = props.agent
  if (!agent || target.kind !== agent.kind || target.id !== agent.id) return false
  return !agent.pane || target.pane === agent.pane
}

const groups = computed(() => {
  const ordered = [...targets.value].sort((a, b) => Number(preferred(b)) - Number(preferred(a)))
  const code = ordered.filter((target) => target.kind === 'session')
  const chats = ordered.filter((target) => target.kind === 'chat')
  const list = [
    { label: 'Code', targets: code },
    { label: 'Chats', targets: chats },
  ]
  if (props.agent?.kind === 'chat') list.reverse()
  return list.filter((group) => group.targets.length)
})

const rows = computed(() => groups.value.flatMap((group) => group.targets))
const indexOf = (target: AgentTarget) => rows.value.indexOf(target)

onMounted(async () => {
  try {
    targets.value = (await AgentTargets()) ?? []
    picked.value = rows.value.find((target) => preferred(target) && target.status !== 'approval') ?? null
  } catch (err) {
    loadError.value = errorText(err, 'Could not list the running agents.')
  } finally {
    loading.value = false
  }
})

async function send(): Promise<void> {
  const target = picked.value
  if (!target || sending.value) return
  sending.value = true
  sendError.value = null
  const error = await review.send(props.space, props.path, { kind: target.kind, id: target.id, pane: target.pane })
  sending.value = false
  if (error) {
    sendError.value = error
    return
  }
  showToast(`Sent to ${target.label}`)
  emit('sent')
  emit('close')
}
</script>

<template>
  <BaseModal
    title="Send to agent"
    :icon="IconSend"
    :width="460"
    :busy="sending"
    :testid="`${testid}-send-dialog`"
    @close="emit('close')"
  >
    <div class="flex flex-col gap-3">
      <div v-if="loading" class="flex justify-center py-4"><Spinner /></div>
      <InlineError v-else-if="loadError" variant="line" :message="loadError" :testid="`${testid}-send-error`" />
      <EmptyState
        v-else-if="!rows.length"
        variant="inline"
        message="No agents are running."
        :data-testid="`${testid}-send-empty`"
      />
      <template v-else>
        <div v-for="group in groups" :key="group.label" class="flex flex-col gap-1">
          <span class="font-mono text-caption tracking-wider text-text-4 uppercase">{{ group.label }}</span>
          <button
            v-for="target in group.targets"
            :key="`${target.kind}:${target.id}:${target.pane}`"
            type="button"
            role="radio"
            class="flex w-full items-center gap-2 rounded-lg border px-2.5 py-2 text-left text-small"
            :class="[
              picked === target ? 'border-accent bg-accent-tint' : 'border-card hover:bg-chip',
              target.status === 'approval' ? 'cursor-not-allowed opacity-60' : 'cursor-pointer',
            ]"
            :aria-checked="picked === target"
            :disabled="target.status === 'approval'"
            :data-testid="`${testid}-send-target-${indexOf(target)}`"
            @click="picked = target"
          >
            <component
              :is="activityIndicator(target.status).icon"
              class="size-3.5 shrink-0"
              :class="[
                activityIndicator(target.status).color,
                { 'animate-spin': activityIndicator(target.status).animated },
              ]"
              :aria-label="activityIndicator(target.status).label"
            />
            <span class="min-w-0 flex-1 truncate text-text">{{ target.label }}</span>
            <span v-if="target.status === 'approval'" class="shrink-0 text-caption text-text-4"
              >Waiting on a prompt</span
            >
          </button>
        </div>
      </template>
      <p
        v-if="picked?.status === 'active'"
        class="text-small text-severity-warning"
        :data-testid="`${testid}-send-warning`"
      >
        This agent is working. Your feedback will arrive mid-turn.
      </p>
      <InlineError v-if="sendError" variant="line" :message="sendError" :testid="`${testid}-send-error`" />
    </div>
    <template #footer>
      <div class="flex-1" />
      <BaseButton variant="ghost" size="sm" @click="emit('close')">Cancel</BaseButton>
      <BaseButton size="sm" :busy="sending" :disabled="!picked" :data-testid="`${testid}-send-submit`" @click="send">
        Send
      </BaseButton>
    </template>
  </BaseModal>
</template>
