<script setup lang="ts">
import IconArrowLeft from '~icons/lucide/arrow-left'

const props = withDefaults(defineProps<{
  closeTestid?: string
  closeLabel?: string
}>(), {
  closeLabel: 'Back to feed',
})

const emit = defineEmits<{ close: [] }>()
</script>

<template>
  <header class="flex h-11 shrink-0 items-center gap-2.5 border-b border-row bg-canvas-toolbar px-4">
    <slot name="title" />
    <div class="flex-1" />
    <button
      type="button"
      class="flex cursor-pointer items-center gap-1.5 rounded-md px-2.5 py-1.5 text-[12.5px] text-text-2 hover:bg-chip hover:text-text"
      :data-testid="props.closeTestid"
      @click="emit('close')"
    ><IconArrowLeft class="size-3.5" />{{ props.closeLabel }}</button>
  </header>
</template>
