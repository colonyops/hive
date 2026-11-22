<script setup lang="ts">
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import TextArea from '@hive/ui/components/ui/TextArea.vue'
import { onMounted, ref } from 'vue'

const props = withDefaults(
  defineProps<{
    /** Posts the text and answers an error to show, or null. */
    submit: (text: string) => Promise<string | null>
    placeholder?: string
    submitLabel?: string
    initial?: string
    autofocus?: boolean
    cancelable?: boolean
    testid?: string
  }>(),
  {
    placeholder: 'Add a comment',
    submitLabel: 'Comment',
    initial: '',
    autofocus: false,
    cancelable: false,
    testid: 'composer',
  },
)
const emit = defineEmits<{ cancel: []; done: [] }>()

// The draft seeds once from initial and is the writer's from then on.
// eslint-disable-next-line vue/no-setup-props-reactivity-loss -- see above
const text = ref(props.initial)
const error = ref<string | null>(null)
const busy = ref(false)
const field = ref<{ focus: () => void } | null>(null)

onMounted(() => {
  if (props.autofocus) field.value?.focus()
})

async function send(): Promise<void> {
  if (!text.value.trim() || busy.value) return
  busy.value = true
  // The draft stays on a failure, so the writer can retry without retyping.
  error.value = await props.submit(text.value)
  busy.value = false
  if (!error.value) {
    text.value = ''
    emit('done')
  }
}

function onKey(event: KeyboardEvent): void {
  if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
    event.preventDefault()
    void send()
  } else if (event.key === 'Escape' && props.cancelable) emit('cancel')
}
</script>

<template>
  <div class="flex flex-col gap-2" :data-testid="testid">
    <TextArea
      ref="field"
      v-model="text"
      size="sm"
      :rows="2"
      :placeholder="placeholder"
      :data-testid="`${testid}-input`"
      @keydown="onKey"
    />
    <InlineError v-if="error" variant="line" :message="error" :testid="`${testid}-error`" />
    <div class="flex justify-end gap-2">
      <BaseButton v-if="cancelable" variant="ghost" size="xs" :data-testid="`${testid}-cancel`" @click="emit('cancel')">
        Cancel
      </BaseButton>
      <BaseButton size="xs" :busy="busy" :disabled="!text.trim()" :data-testid="`${testid}-submit`" @click="send">
        {{ submitLabel }}
      </BaseButton>
    </div>
  </div>
</template>
