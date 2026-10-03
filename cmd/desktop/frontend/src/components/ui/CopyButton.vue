<script setup lang="ts">
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import { useClipboard } from '../../composables/useClipboard'
import AppTooltip from './AppTooltip.vue'
import BaseButton from './BaseButton.vue'

defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    /** What lands on the clipboard. Empty makes a click do nothing. */
    text: string
    /** The button's label, or its tooltip and accessible name for `icon`. */
    label: string
    variant?: 'button' | 'icon' | 'link'
    resetDelay?: number
  }>(),
  { variant: 'button', resetDelay: undefined },
)

// eslint-disable-next-line vue/no-setup-props-reactivity-loss -- the delay is fixed for the button's life
const { copy, copied } = useClipboard({ resetDelay: props.resetDelay })

function onCopy(): void {
  if (props.text) void copy(props.text)
}
</script>

<template>
  <BaseButton v-if="variant === 'button'" v-bind="$attrs" variant="secondary" size="xs" @click="onCopy">
    <template #icon>
      <IconCheck v-if="copied" class="size-3.5" />
      <IconCopy v-else class="size-3.5" />
    </template>
    {{ copied ? 'Copied' : label }}
  </BaseButton>
  <AppTooltip v-else-if="variant === 'icon'" :text="copied ? 'Copied' : label">
    <button
      type="button"
      v-bind="$attrs"
      class="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-[7px] hover:bg-chip hover:text-text"
      :class="copied ? 'text-severity-success' : 'text-text-4'"
      :aria-label="label"
      @click="onCopy"
    >
      <IconCheck v-if="copied" class="size-3.5" />
      <IconCopy v-else class="size-3.5" />
    </button>
  </AppTooltip>
  <button v-else type="button" v-bind="$attrs" @click="onCopy">{{ copied ? 'Copied' : label }}</button>
</template>
