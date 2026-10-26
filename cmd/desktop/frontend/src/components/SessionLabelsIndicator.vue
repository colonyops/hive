<script setup lang="ts">
// A sidebar row is too narrow for label text next to the name, so the row
// shows that labels exist and the tooltip names them.
import { computed } from 'vue'
import IconTag from '~icons/lucide/tag'
import AppTooltip from '@hive/ui/components/ui/AppTooltip.vue'

const props = defineProps<{ labels: readonly string[] | null }>()

const text = computed(() => (props.labels ?? []).join(', '))
</script>

<template>
  <AppTooltip v-if="labels?.length" :text="text">
    <span
      class="flex shrink-0 items-center gap-0.5 font-mono text-micro leading-none text-text-4"
      role="img"
      :aria-label="`Labels: ${text}`"
      data-testid="session-labels"
    >
      <IconTag class="size-3" aria-hidden="true" />
      <span v-if="labels.length > 1">{{ labels.length }}</span>
    </span>
  </AppTooltip>
</template>
