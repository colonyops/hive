<script setup lang="ts">
// The comments column's title strip: the count and the actions on every
// comment at once. It shares the height and rule of the document's own
// toolbar, so the two read as one bar.
import { computed, ref } from 'vue'
import IconX from '~icons/lucide/x'
import IconButton from '../ui/IconButton.vue'
import { useClipboard } from '../../composables/useClipboard'
import { useToasts } from '../../composables/useToasts'
import { errorText } from '../../lib/appError'
import { useDriveReview } from '../../stores/useDriveReview'

const props = defineProps<{
  space: string
  path: string
  /** Shows a close button, for comments that overlay the document. */
  closable?: boolean
  /** Derives `-review-header`, `-review-count`, `-review-copy`, `-review-clear`, `-review-clear-confirm`, and `-review-close`. */
  testid: string
}>()
const emit = defineEmits<{ close: [] }>()

const store = useDriveReview()
const { showToast } = useToasts()
const count = computed(() => store.review(props.space, props.path).data.value.comments?.length ?? 0)
const confirming = ref(false)

const { copy, setStatus, status } = useClipboard()
async function copyAll(): Promise<void> {
  try {
    await copy(await store.feedback(props.space, props.path))
  } catch (err) {
    setStatus('error')
    showToast(errorText(err, 'Could not build the feedback.'), { severity: 'error' })
    return
  }
  if (status.value === 'error') showToast('Could not copy to the clipboard', { severity: 'error' })
}

async function clear(): Promise<void> {
  const error = await store.clear(props.space, props.path)
  confirming.value = false
  if (error) showToast(error, { severity: 'error' })
}

const action = 'cursor-pointer rounded px-1 text-caption text-text-3 hover:text-text'
</script>

<template>
  <div
    class="flex h-11 shrink-0 items-center gap-2 border-b border-row bg-pane px-4"
    :data-testid="`${testid}-review-header`"
  >
    <span class="text-small font-semibold text-text">Comments</span>
    <span
      class="rounded bg-chip px-1.5 py-0.5 font-mono text-micro leading-none text-text-2"
      :data-testid="`${testid}-review-count`"
      >{{ count }}</span
    >
    <div class="flex-1" />
    <template v-if="confirming">
      <span class="text-caption text-text-2">Clear every comment?</span>
      <button
        type="button"
        :class="action"
        class="!text-severity-error"
        :data-testid="`${testid}-review-clear-confirm`"
        @click="clear"
      >
        Clear
      </button>
      <button type="button" :class="action" @click="confirming = false">Keep</button>
    </template>
    <template v-else>
      <button type="button" :class="action" :data-testid="`${testid}-review-copy`" @click="copyAll">
        {{ status === 'success' ? 'Copied' : 'Copy all' }}
      </button>
      <button type="button" :class="action" :data-testid="`${testid}-review-clear`" @click="confirming = true">
        Clear
      </button>
    </template>
    <IconButton
      v-if="closable"
      label="Hide comments"
      :icon="IconX"
      :data-testid="`${testid}-review-close`"
      @click="emit('close')"
    />
  </div>
</template>
