<script setup lang="ts">
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import { useClipboard } from '../../composables/useClipboard'
import BaseButton from './BaseButton.vue'

const props = withDefaults(
  defineProps<{
    /** What lands on the clipboard. Empty makes a click do nothing. */
    text: string
    label?: string
    size?: 'xs' | 'sm'
  }>(),
  { label: 'Copy', size: 'sm' },
)

const { copy, copied } = useClipboard()

function onCopy(): void {
  if (props.text) void copy(props.text)
}
</script>

<template>
  <BaseButton variant="secondary" :size="size" class="whitespace-nowrap" @click="onCopy">
    <template #icon>
      <IconCheck v-if="copied" class="size-3.5" />
      <IconCopy v-else class="size-3.5" />
    </template>
    {{ copied ? 'Copied' : label }}
  </BaseButton>
</template>
