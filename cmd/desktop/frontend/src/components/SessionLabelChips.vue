<script setup lang="ts">
import { computed } from 'vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'

const props = withDefaults(defineProps<{ labels: readonly string[] | null; max?: number }>(), { max: 2 })

const shown = computed(() => (props.labels ?? []).slice(0, props.max))
const hidden = computed(() => (props.labels ?? []).slice(props.max))
</script>

<template>
  <span
    v-if="shown.length"
    class="flex min-w-0 max-w-[50%] shrink items-center gap-1"
    :title="(labels ?? []).join(', ')"
    data-testid="session-labels"
  >
    <BaseBadge
      v-for="label in shown"
      :key="label"
      tone="neutral"
      class="min-w-0 shrink px-1.5 py-0.5 text-micro leading-none"
      data-testid="session-label"
    >
      <span class="truncate">{{ label }}</span>
    </BaseBadge>
    <BaseBadge
      v-if="hidden.length"
      tone="muted"
      class="shrink-0 px-1.5 py-0.5 text-micro leading-none"
      :aria-label="`${hidden.length} more ${hidden.length === 1 ? 'label' : 'labels'}`"
      data-testid="session-labels-more"
    >
      +{{ hidden.length }}
    </BaseBadge>
  </span>
</template>
