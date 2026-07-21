<script setup lang="ts">
import { computed, type Component } from 'vue'

const props = withDefaults(defineProps<{
  active: boolean
  icon?: Component
  label: string
  tone?: 'default' | 'danger'
  testid?: string
}>(), {
  tone: 'default',
})
const emit = defineEmits<{ select: [] }>()

const stateClasses = computed(() => {
  if (!props.active) return 'text-text-2 hover:bg-chip hover:text-text'
  return props.tone === 'danger'
    ? 'bg-hover font-medium text-severity-error'
    : 'bg-hover font-medium text-accent'
})
</script>

<template>
  <button
    type="button"
    class="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-2 text-left text-[13px]"
    :class="stateClasses"
    :aria-current="props.active ? 'true' : undefined"
    :data-testid="props.testid"
    @click="emit('select')"
  ><component :is="props.icon" v-if="props.icon" class="size-3.5 shrink-0" />{{ props.label }}</button>
</template>
