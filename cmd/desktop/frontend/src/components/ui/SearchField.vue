<script setup lang="ts">
import { ref, useAttrs } from 'vue'
import IconSearch from '~icons/lucide/search'

defineOptions({ inheritAttrs: false })

withDefaults(
  defineProps<{
    /** `boxed` is a bordered toolbar field; `bar` sits flush in a toolbar row the host draws. */
    variant?: 'boxed' | 'bar'
    placeholder?: string
    ariaLabel?: string
    testid?: string
  }>(),
  { variant: 'boxed', placeholder: 'Filter…', ariaLabel: undefined, testid: undefined },
)

const model = defineModel<string>({ default: '' })
// Fires when Escape lands on an empty field, so a host can move focus away.
const emit = defineEmits<{ escape: [event: KeyboardEvent] }>()

const attrs = useAttrs()
const input = ref<HTMLInputElement | null>(null)

// A class lays out the field; every other attribute and listener belongs on
// the input.
function inputAttrs(): Record<string, unknown> {
  const { class: _class, ...rest } = attrs
  return rest
}

// Escape with text only clears, and stops there so the overlay under the
// field stays open. Escape on an empty field is left alone and reaches the
// window Escape stack as usual.
function onKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Escape') return
  if (model.value) {
    event.preventDefault()
    event.stopPropagation()
    model.value = ''
    return
  }
  emit('escape', event)
}

function onInput(event: Event): void {
  model.value = (event.target as HTMLInputElement).value
}

defineExpose({
  focus: () => input.value?.focus(),
  select: () => input.value?.select(),
})
</script>

<template>
  <label
    class="flex items-center gap-2"
    :class="[
      variant === 'boxed'
        ? 'rounded-lg border border-strong bg-app px-2.5 py-1.5 focus-within:border-accent'
        : 'min-w-0 flex-1',
      $attrs.class,
    ]"
  >
    <IconSearch
      class="shrink-0"
      :class="[variant === 'boxed' ? 'size-3.5' : 'size-3', model ? 'text-text-3' : 'text-text-4']"
    />
    <input
      ref="input"
      v-bind="inputAttrs()"
      :value="model"
      type="text"
      :placeholder="placeholder"
      :aria-label="ariaLabel"
      autocapitalize="off"
      autocorrect="off"
      spellcheck="false"
      class="min-w-0 flex-1 bg-transparent text-small text-text outline-none placeholder:text-text-4"
      :data-testid="testid"
      @input="onInput"
      @keydown="onKeydown"
    />
  </label>
</template>
