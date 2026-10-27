<script setup lang="ts">
// Picks one running agent to receive a document's feedback. Go lists only
// agents that can take text now and resolves the pick again on send.
import { computed, onMounted, ref } from 'vue'
import IconSend from '~icons/lucide/send'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import BaseModal from '@hive/ui/components/ui/BaseModal.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import { useToasts } from '../../composables/useToasts'
import { activityIndicator } from '../../lib/agentActivity'
import { errorText } from '../../lib/appError'
import { useDriveReview } from '../../stores/useDriveReview'
import { AgentTargets } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reviewservice'
import type { AgentTarget } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const props = defineProps<{
  space: string
  path: string
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

const groups = computed(() => {
  const code = targets.value.filter((target) => target.kind === 'session')
  const chats = targets.value.filter((target) => target.kind === 'chat')
  return [
    { label: 'Code', targets: code },
    { label: 'Chats', targets: chats },
  ].filter((group) => group.targets.length)
})

const rows = computed(() => groups.value.flatMap((group) => group.targets))

// "repo / session · window" reads as the session, with where it runs muted.
function labelParts(target: AgentTarget): { name: string; detail: string } {
  const [name, detail = ''] = target.label.split(' · ')
  return { name, detail }
}
const indexOf = (target: AgentTarget) => rows.value.indexOf(target)

onMounted(async () => {
  try {
    targets.value = (await AgentTargets()) ?? []
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
    title="Send to Agent"
    :icon="IconSend"
    :width="460"
    :busy="sending"
    :testid="`${testid}-send-dialog`"
    @close="emit('close')"
  >
    <div v-if="loading" class="flex justify-center px-5 py-6"><Spinner /></div>
    <InlineError
      v-else-if="loadError"
      variant="line"
      class="px-5 py-4"
      :message="loadError"
      :testid="`${testid}-send-error`"
    />
    <EmptyState
      v-else-if="!rows.length"
      variant="inline"
      class="px-5 py-4"
      message="No agents are running."
      :data-testid="`${testid}-send-empty`"
    />
    <div v-else role="listbox" aria-label="Running agents" class="hive-scroll max-h-80 overflow-y-auto px-3 py-2">
      <div v-for="group in groups" :key="group.label" role="group" :aria-label="group.label" class="py-1">
        <div class="px-2 pt-1 pb-1.5 font-mono text-micro tracking-[0.12em] text-text-4 uppercase">
          {{ group.label }}
        </div>
        <button
          v-for="target in group.targets"
          :key="`${target.kind}:${target.id}:${target.pane}`"
          type="button"
          role="option"
          class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-small"
          :class="[
            picked === target ? 'bg-selection text-text' : 'text-text-2',
            target.status === 'approval' ? 'cursor-not-allowed opacity-50' : 'cursor-pointer hover:bg-chip',
          ]"
          :aria-selected="picked === target"
          :disabled="target.status === 'approval'"
          :data-testid="`${testid}-send-target-${indexOf(target)}`"
          @click="picked = target"
          @dblclick="send"
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
          <span class="min-w-0 flex-1 truncate" :title="target.label"
            >{{ labelParts(target).name
            }}<span v-if="labelParts(target).detail" class="text-text-4"> · {{ labelParts(target).detail }}</span></span
          >
          <span v-if="target.status === 'approval'" class="shrink-0 text-micro text-text-4">Waiting on a prompt</span>
          <span v-else-if="target.status === 'active'" class="shrink-0 text-micro text-text-4">Working</span>
        </button>
      </div>
    </div>
    <p
      v-if="picked?.status === 'active'"
      class="px-5 pb-3 text-small leading-relaxed text-text-3"
      :data-testid="`${testid}-send-warning`"
    >
      This agent is working. Your feedback will arrive mid-turn.
    </p>
    <InlineError
      v-if="sendError"
      variant="line"
      class="px-5 pb-3 leading-relaxed"
      :message="sendError"
      :testid="`${testid}-send-error`"
    />
    <template #footer>
      <div class="flex-1" />
      <BaseButton variant="secondary" :disabled="sending" @click="emit('close')">Cancel</BaseButton>
      <BaseButton :busy="sending" :disabled="!picked" :data-testid="`${testid}-send-submit`" @click="send">
        Send
      </BaseButton>
    </template>
  </BaseModal>
</template>
