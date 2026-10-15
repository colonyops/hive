<script setup lang="ts">
import { computed, ref } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconSend from '~icons/lucide/send'
import SendToAgentDialog from './SendToAgentDialog.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import Kbd from '@hive/ui/components/ui/Kbd.vue'
import TextArea from '@hive/ui/components/ui/TextArea.vue'
import { useClipboard } from '@hive/ui/composables/useClipboard'
import { useToasts } from '../../composables/useToasts'
import { errorText } from '../../lib/appError'
import { useDriveReview } from '../../stores/useDriveReview'
import type { AgentTargetRef } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const props = defineProps<{
  space: string
  path: string
  /** The agent Send to Agent offers first. */
  agent?: AgentTargetRef | null
  /** Derives `-review-footer`, `-review-note`, `-review-ready`, `-review-copy`, `-review-send`, and the dialog's ids. */
  testid: string
}>()

const store = useDriveReview()
const { showToast } = useToasts()
const count = computed(() => store.review(props.space, props.path).data.value.comments?.length ?? 0)
const note = computed({
  get: () => store.note(props.space, props.path),
  set: (text: string) => store.setNote(props.space, props.path, text),
})
const ready = computed(() => (count.value === 1 ? '1 comment ready' : `${count.value} comments ready`))
const sending = ref(false)

const { copy, setStatus, status: copyStatus } = useClipboard()
async function copyAll(): Promise<void> {
  try {
    await copy(await store.feedback(props.space, props.path))
  } catch (err) {
    setStatus('error')
    showToast(errorText(err, 'Could not build the feedback.'), { severity: 'error' })
    return
  }
  if (copyStatus.value === 'error') showToast('Could not copy to the clipboard', { severity: 'error' })
}

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
    <span class="text-caption text-text-3" :data-testid="`${testid}-review-ready`">{{ ready }}</span>
    <div class="grid grid-cols-2 gap-2">
      <BaseButton variant="secondary" size="xs" :data-testid="`${testid}-review-copy`" @click="copyAll">
        <template #icon>
          <IconCheck v-if="copyStatus === 'success'" class="size-3.5" />
          <IconCopy v-else class="size-3.5" />
        </template>
        {{ copyStatus === 'success' ? 'Copied' : 'Copy All' }}
      </BaseButton>
      <BaseButton
        variant="secondary"
        size="xs"
        class="!border-accent/40 !bg-accent-tint !text-accent"
        :data-testid="`${testid}-review-send`"
        @click="sending = true"
      >
        <template #icon><IconSend class="size-3.5" /></template>
        Send to Agent
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
