<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import IconPencil from '~icons/lucide/pencil'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import BaseModal from '@hive/ui/components/ui/BaseModal.vue'
import FormField from '@hive/ui/components/ui/FormField.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import TextInput from '@hive/ui/components/ui/TextInput.vue'
import TokenInput from '@hive/ui/components/ui/TokenInput.vue'
import { seedRef } from '@hive/ui/lib/seedRef'

const props = withDefaults(
  defineProps<{
    name: string
    tags: readonly string[] | null
    /** The field to focus: the one the user asked to change. */
    focus?: 'name' | 'tags'
    suggestions?: string[]
    busy?: boolean
    error?: string | null
  }>(),
  { focus: 'name', suggestions: () => [], busy: false, error: null },
)
const emit = defineEmits<{ close: []; save: [name: string, tags: string[]] }>()

const nameDraft = seedRef(() => props.name)
const tagsDraft = seedRef<string[]>(() => [...(props.tags ?? [])])
const nameInput = ref<{ focus: () => void; select: () => void } | null>(null)
const tagsInput = ref<{ flush: () => void; focus: () => void } | null>(null)
const canSave = computed(() => !!nameDraft.value.trim())

function submit(): void {
  if (props.busy || !canSave.value) return
  // A typed tag that was never committed with Enter is still meant. The
  // flush updates tagsDraft synchronously through v-model.
  tagsInput.value?.flush()
  emit('save', nameDraft.value.trim(), [...tagsDraft.value])
}

onMounted(async () => {
  await nextTick()
  if (props.focus === 'tags') {
    tagsInput.value?.focus()
  } else {
    nameInput.value?.focus()
    nameInput.value?.select()
  }
})
</script>

<template>
  <BaseModal
    title="Edit session"
    :icon="IconPencil"
    :width="440"
    :busy="busy"
    testid="session-edit-dialog"
    @close="emit('close')"
  >
    <div class="flex flex-col gap-4 px-5 py-4">
      <FormField
        v-slot="{ id }"
        label="Name"
        hint="Its terminal session is renamed too, so an open terminal reconnects."
        testid="session-edit-name-field"
      >
        <TextInput
          :id="id"
          ref="nameInput"
          v-model="nameDraft"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
          :disabled="busy"
          data-testid="session-edit-name"
          @keydown.enter="submit"
        />
      </FormField>
      <FormField
        v-slot="{ id }"
        label="Tags"
        hint="Enter or a comma adds a tag. Tags ignore case."
        testid="session-edit-tags-field"
      >
        <TokenInput
          :id="id"
          ref="tagsInput"
          v-model="tagsDraft"
          :suggestions="suggestions"
          placeholder="Add tag…"
          testid="session-edit-tags"
          @keydown.meta.enter="submit"
        />
      </FormField>
      <InlineError v-if="error" :message="error" testid="session-edit-error" />
    </div>
    <template #footer>
      <div class="flex-1" />
      <BaseButton variant="secondary" :disabled="busy" data-testid="session-edit-cancel" @click="emit('close')"
        >Cancel</BaseButton
      >
      <BaseButton :busy="busy" :disabled="!canSave" data-testid="session-edit-save" @click="submit">Save</BaseButton>
    </template>
  </BaseModal>
</template>
