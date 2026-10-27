<script setup lang="ts">
// Shares the height and rule of the document's own toolbar, so the two read
// as one bar.
import { computed, ref } from 'vue'
import IconTrash from '~icons/lucide/trash-2'
import IconX from '~icons/lucide/x'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import { useToasts } from '../../composables/useToasts'
import { useDriveReview } from '../../stores/useDriveReview'

const props = defineProps<{
  space: string
  path: string
  /** Shows a close button, for comments that overlay the document. */
  closable?: boolean
  /** Derives `-review-header`, `-review-count`, `-review-clear`, `-review-clear-confirm`, and `-review-close`. */
  testid: string
}>()
const emit = defineEmits<{ close: [] }>()

const store = useDriveReview()
const { showToast } = useToasts()
const count = computed(() => store.review(props.space, props.path).data.value.comments?.length ?? 0)
const confirming = ref(false)

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
    <IconButton
      v-else
      label="Clear All Comments"
      :icon="IconTrash"
      tone="danger"
      :data-testid="`${testid}-review-clear`"
      @click="confirming = true"
    />
    <IconButton
      v-if="closable"
      label="Hide Comments"
      :icon="IconX"
      :data-testid="`${testid}-review-close`"
      @click="emit('close')"
    />
  </div>
</template>
