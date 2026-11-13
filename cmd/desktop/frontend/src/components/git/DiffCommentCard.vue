<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import TextArea from '@hive/ui/components/ui/TextArea.vue'
import IconPencil from '~icons/lucide/pencil'
import IconTrash from '~icons/lucide/trash-2'
import IconUserRound from '~icons/lucide/user-round'
import { commentLabel } from '../../lib/diffReview'
import { gitFileName } from '../../lib/gitTree'
import { labelOf } from '../../lib/reviewLabels'
import { useReviewLabels } from '../../stores/useReviewLabels'
import type { ChangesComment } from '../../stores/useChangesReview'

const props = defineProps<{
  comment: ChangesComment
  /** A compact row in the panel, away from the diff, so it names its file. */
  row?: boolean
  /** The file changed since this comment was written. */
  outdated?: boolean
}>()
const emit = defineEmits<{ update: [text: string]; remove: []; pick: [] }>()

const { labels } = useReviewLabels()
const label = computed(() => labelOf(labels.value, props.comment.text))
const testid = computed(() => `changes-comment-${props.comment.id}`)

const editing = ref(false)
const draft = ref('')
const field = ref<{ focus: () => void } | null>(null)

// The pencil that was clicked unmounts with this render, so without a focus
// here the keys typed next reach the view's bare-key bindings instead.
async function startEdit(): Promise<void> {
  draft.value = props.comment.text
  editing.value = true
  await nextTick()
  field.value?.focus()
}

function save(): void {
  if (!draft.value.trim()) return
  emit('update', draft.value.trim())
  editing.value = false
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
    event.preventDefault()
    event.stopPropagation()
    save()
  } else if (event.key === 'Escape') {
    event.stopPropagation()
    editing.value = false
  }
}
</script>

<template>
  <!-- A pointer press here must not reach CodeView, which would start a line selection under the card. -->
  <div
    class="group font-sans"
    :class="
      row
        ? 'border-b border-row px-2.5 py-1.5 text-caption hover:bg-chip'
        : 'grid max-w-3xl grid-cols-[1.5rem_minmax(0,1fr)] gap-x-2.5 rounded-md border border-card bg-raised px-3 py-2 text-body'
    "
    :data-testid="testid"
    @pointerdown.stop
    @click.stop
  >
    <span
      v-if="!row"
      class="mt-px flex size-6 items-center justify-center rounded-full bg-chip text-text-2"
      aria-hidden="true"
    >
      <IconUserRound class="size-3.5" />
    </span>
    <div class="flex min-w-0 flex-col gap-0.5">
      <div class="flex min-w-0 items-center" :class="row ? 'h-4.5 gap-1.5' : 'h-6 gap-2 text-small'">
        <span v-if="!row" class="shrink-0 font-medium text-text">You</span>
        <button
          type="button"
          class="flex min-w-0 cursor-pointer gap-1.5 text-left font-mono text-text-3 hover:text-text"
          :class="row ? 'text-micro' : 'text-caption'"
          :title="`${comment.path} ${commentLabel(comment)}`.trim()"
          :data-testid="`${testid}-pick`"
          @click="emit('pick')"
        >
          <span v-if="row && comment.path" class="min-w-0 truncate text-text-2">{{ gitFileName(comment.path) }}</span>
          <span class="shrink-0">{{ commentLabel(comment) }}</span>
        </button>
        <span
          v-if="outdated"
          class="size-1.5 shrink-0 rounded-full bg-severity-warning"
          title="The agent changed this file after you wrote the comment"
          aria-label="File changed"
          :data-testid="`${testid}-outdated`"
        />
        <span
          v-if="!editing"
          class="ml-auto flex shrink-0 items-center gap-0.5 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100"
        >
          <IconButton
            label="Edit comment"
            :icon="IconPencil"
            :size="row ? 'sm' : 'md'"
            :data-testid="`${testid}-edit`"
            @click="startEdit"
          />
          <IconButton
            label="Delete comment"
            :icon="IconTrash"
            :size="row ? 'sm' : 'md'"
            :data-testid="`${testid}-delete`"
            @click="emit('remove')"
          />
        </span>
      </div>
      <div v-if="editing" class="flex flex-col gap-1 pb-0.5">
        <TextArea
          ref="field"
          v-model="draft"
          :rows="2"
          size="sm"
          :data-testid="`${testid}-edit-text`"
          @keydown="onKeydown"
        />
        <div class="flex justify-end gap-1.5">
          <BaseButton variant="ghost" size="xs" @click="editing = false">Cancel</BaseButton>
          <BaseButton size="xs" :disabled="!draft.trim()" :data-testid="`${testid}-edit-save`" @click="save">
            Save
          </BaseButton>
        </div>
      </div>
      <p
        v-else
        class="flex items-start gap-1 break-words text-text"
        :class="row ? 'line-clamp-2 leading-snug' : 'leading-relaxed whitespace-pre-wrap'"
        :title="row ? comment.text : undefined"
      >
        <component :is="label.icon" v-if="label" class="mt-px size-3 shrink-0 text-accent" aria-hidden="true" />
        <span class="min-w-0">{{ comment.text }}</span>
      </p>
    </div>
  </div>
</template>
