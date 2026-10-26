<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import IconTag from '~icons/lucide/tag'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import BaseModal from '@hive/ui/components/ui/BaseModal.vue'
import FormField from '@hive/ui/components/ui/FormField.vue'
import TokenInput from '@hive/ui/components/ui/TokenInput.vue'
import { seedRef } from '@hive/ui/lib/seedRef'

const props = withDefaults(
  defineProps<{
    name: string
    tags: readonly string[] | null
    suggestions?: string[]
    busy?: boolean
    error?: string | null
  }>(),
  { suggestions: () => [], busy: false, error: null },
)
const emit = defineEmits<{ close: []; save: [tags: string[]] }>()

const draft = seedRef<string[]>(() => [...(props.tags ?? [])])
const field = ref<{ flush: () => void; focus: () => void } | null>(null)

function submit(): void {
  if (props.busy) return
  // A typed tag that was never committed with Enter is still meant. The
  // flush updates draft synchronously through v-model.
  field.value?.flush()
  emit('save', [...draft.value])
}

onMounted(async () => {
  await nextTick()
  field.value?.focus()
})
</script>

<template>
  <BaseModal
    title="Edit tags"
    :icon="IconTag"
    :width="440"
    :busy="busy"
    testid="session-tags-dialog"
    @close="emit('close')"
  >
    <div class="flex flex-col gap-3 px-5 py-4">
      <p class="truncate text-caption text-text-3" :title="name" data-testid="session-tags-session">{{ name }}</p>
      <FormField
        v-slot="{ id }"
        label="Tags"
        hint="Enter or a comma adds a tag. Tags ignore case."
        :error="error"
        testid="session-tags"
      >
        <TokenInput
          :id="id"
          ref="field"
          v-model="draft"
          :suggestions="suggestions"
          placeholder="Add tag…"
          testid="session-tags-input"
          @keydown.meta.enter="submit"
        />
      </FormField>
    </div>
    <template #footer>
      <div class="flex-1" />
      <BaseButton variant="secondary" :disabled="busy" data-testid="session-tags-cancel" @click="emit('close')"
        >Cancel</BaseButton
      >
      <BaseButton :busy="busy" data-testid="session-tags-save" @click="submit">Save</BaseButton>
    </template>
  </BaseModal>
</template>
