<script setup lang="ts">
// Token input for the feed-item types an action applies to. Suggests the
// kinds the system actually knows about (live feed-item kinds unioned with
// types already configured elsewhere) and flags anything that won't match a
// known kind.
import { computed, ref } from 'vue'
import TokenInput from '@hive/ui/components/ui/TokenInput.vue'

const props = defineProps<{ modelValue: string[] | null; knownTypes: string[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()

const field = ref<{ flush: () => void } | null>(null)

const isKnown = (tag: string): boolean =>
  !props.knownTypes.length || props.knownTypes.some((type) => type.toLowerCase() === tag.toLowerCase())
const flag = (tag: string): string => (isKnown(tag) ? '' : "Not a known feed-item type — it won't match any feed item")
const hasUnknown = computed(() => props.knownTypes.length > 0 && (props.modelValue ?? []).some((tag) => !isKnown(tag)))

// The drawer commits the pending draft when Save is pressed.
defineExpose({ flush: () => field.value?.flush() })
</script>

<template>
  <div>
    <div class="text-xs text-text-2">Applies to <span class="text-text-4">(feed-item types)</span></div>
    <TokenInput
      ref="field"
      class="mt-1"
      :model-value="modelValue"
      :suggestions="knownTypes"
      :flag="flag"
      monospace
      placeholder="Add type…"
      aria-label="Applies to"
      testid="action-applies-to"
      @update:model-value="emit('update:modelValue', $event)"
    />

    <p v-if="hasUnknown" class="mt-1.5 text-caption leading-relaxed text-accent">
      Highlighted types don't match any known feed item yet.
    </p>
  </div>
</template>
