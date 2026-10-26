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
    labels: readonly string[] | null
    suggestions?: string[]
    busy?: boolean
    error?: string | null
  }>(),
  { suggestions: () => [], busy: false, error: null },
)
const emit = defineEmits<{ close: []; save: [labels: string[]] }>()

const draft = seedRef<string[]>(() => [...(props.labels ?? [])])
const field = ref<{ flush: () => void; focus: () => void } | null>(null)

function submit(): void {
  if (props.busy) return
  // A typed label that was never committed with Enter is still meant. The
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
    :title="`Labels for ${name}`"
    :icon="IconTag"
    :width="440"
    :busy="busy"
    testid="session-labels-dialog"
    @close="emit('close')"
  >
    <FormField
      v-slot="{ id }"
      label="Labels"
      hint="Enter or a comma adds a label. Labels ignore case."
      :error="error"
      testid="session-labels"
      class="px-5 py-4"
    >
      <TokenInput
        :id="id"
        ref="field"
        v-model="draft"
        :suggestions="suggestions"
        placeholder="Add label…"
        testid="session-labels-input"
        @keydown.meta.enter="submit"
      />
    </FormField>
    <template #footer>
      <div class="flex-1" />
      <BaseButton variant="secondary" :disabled="busy" data-testid="session-labels-cancel" @click="emit('close')"
        >Cancel</BaseButton
      >
      <BaseButton :busy="busy" data-testid="session-labels-save" @click="submit">Save</BaseButton>
    </template>
  </BaseModal>
</template>
