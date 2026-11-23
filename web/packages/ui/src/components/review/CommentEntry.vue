<script setup lang="ts">
// One comment inside a CommentCard: who and when, the text, and its Edit and
// Delete actions. Editing happens in place; the caller owns which comment is
// being edited and saves the text it emits.
import { ref, watch } from 'vue'
import BaseButton from '../ui/BaseButton.vue'
import TextArea from '../ui/TextArea.vue'

export interface CommentEntryTestids {
  body?: string
  deleted?: string
  edit?: string
  delete?: string
  editText?: string
  editSave?: string
}

const props = defineProps<{
  author: string
  /** Shown as the author's tooltip. */
  authorTitle?: string
  age: string
  /** Right of the header, such as the lines the comment is on. */
  aside?: string
  asideTitle?: string
  text: string
  deleted?: boolean
  editing?: boolean
  canEdit?: boolean
  canDelete?: boolean
  /** Brightens the text, for the active card. */
  active?: boolean
  busy?: boolean
  testids?: CommentEntryTestids
}>()
const emit = defineEmits<{ edit: []; cancel: []; save: [text: string]; delete: [] }>()

const draft = ref('')
watch(
  () => props.editing,
  (on) => {
    if (on) draft.value = props.text
  },
  { immediate: true },
)
</script>

<template>
  <div class="flex flex-col gap-2">
    <div v-if="!deleted" class="flex min-w-0 items-center gap-2">
      <slot name="badge" />
      <span class="truncate text-small font-semibold text-text" :title="authorTitle">{{ author }}</span>
      <slot name="author" />
      <span class="shrink-0 text-caption text-text-4">{{ age }}</span>
      <span v-if="aside" class="ml-auto shrink-0 font-mono text-micro text-text-4" :title="asideTitle">{{
        aside
      }}</span>
    </div>
    <p v-if="deleted" class="text-small text-text-4 italic" :data-testid="testids?.deleted">Comment deleted</p>
    <div v-else-if="editing" class="flex flex-col gap-1.5" @click.stop>
      <TextArea v-model="draft" :rows="2" size="sm" :data-testid="testids?.editText" />
      <div class="flex justify-end gap-2">
        <BaseButton variant="ghost" size="xs" @click="emit('cancel')">Cancel</BaseButton>
        <BaseButton
          size="xs"
          :busy="busy"
          :disabled="!draft.trim()"
          :data-testid="testids?.editSave"
          @click="emit('save', draft)"
        >
          Save
        </BaseButton>
      </div>
    </div>
    <p
      v-else
      class="flex items-start gap-1.5 text-small leading-normal break-words whitespace-pre-wrap"
      :class="active ? 'text-text' : 'text-text-2'"
    >
      <slot name="prefix" />
      <span class="min-w-0" :data-testid="testids?.body">{{ text }}</span>
    </p>
    <div
      v-if="!deleted && !editing && (canEdit || canDelete)"
      class="flex gap-3.5 pt-0.5 text-caption text-text-3"
      @click.stop
    >
      <button
        v-if="canEdit"
        type="button"
        class="-mx-1 cursor-pointer rounded px-1 hover:bg-chip hover:text-text"
        :data-testid="testids?.edit"
        @click="emit('edit')"
      >
        Edit
      </button>
      <button
        v-if="canDelete"
        type="button"
        class="-mx-1 ml-auto cursor-pointer rounded px-1 hover:bg-severity-error-tint hover:text-severity-error"
        :data-testid="testids?.delete"
        @click="emit('delete')"
      >
        Delete
      </button>
    </div>
  </div>
</template>
