<script setup lang="ts">
// The last step of a review: a note for the agent and the send.
import { computed, ref } from 'vue'
import SendToAgentDialog from './SendToAgentDialog.vue'
import BaseButton from '../ui/BaseButton.vue'
import Kbd from '../ui/Kbd.vue'
import TextArea from '../ui/TextArea.vue'
import { useDriveReview } from '../../stores/useDriveReview'
import type { AgentTargetRef } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const props = defineProps<{
  space: string
  path: string
  /** The agent Send to Agent offers first. */
  agent?: AgentTargetRef | null
  /** Derives `-review-footer`, `-review-note`, `-review-ready`, `-review-send`, and the dialog's ids. */
  testid: string
}>()

const store = useDriveReview()
const count = computed(() => store.review(props.space, props.path).data.value.comments?.length ?? 0)
const note = computed({
  get: () => store.note(props.space, props.path),
  set: (text: string) => store.setNote(props.space, props.path, text),
})
const ready = computed(() => (count.value === 1 ? '1 comment ready' : `${count.value} comments ready`))
const sending = ref(false)

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
    event.preventDefault()
    sending.value = true
  }
}
</script>

<template>
  <div
    class="flex flex-col gap-2.5 border-t border-row bg-pane px-4 pt-3.5 pb-4"
    :data-testid="`${testid}-review-footer`"
  >
    <TextArea
      v-model="note"
      :rows="2"
      size="sm"
      placeholder="Add a note for the agent…"
      :data-testid="`${testid}-review-note`"
      @keydown="onKeydown"
    />
    <div class="flex items-center justify-between gap-2">
      <span class="text-caption text-text-3" :data-testid="`${testid}-review-ready`">{{ ready }}</span>
      <BaseButton
        variant="secondary"
        size="xs"
        class="!border-accent/40 !bg-accent-tint !text-accent"
        :data-testid="`${testid}-review-send`"
        @click="sending = true"
      >
        Send to agent
        <Kbd class="text-accent/70">⌘↵</Kbd>
      </BaseButton>
    </div>
    <SendToAgentDialog
      v-if="sending"
      :space="space"
      :path="path"
      :agent="agent"
      :testid="testid"
      @close="sending = false"
    />
  </div>
</template>
