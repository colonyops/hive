<script setup lang="ts">
// A sidebar row is too narrow for tag text next to the name, so the row
// shows that tags exist and the tooltip names them.
import { computed } from 'vue'
import IconTag from '~icons/lucide/tag'
import IconTags from '~icons/lucide/tags'
import AppTooltip from '@hive/ui/components/ui/AppTooltip.vue'

const props = defineProps<{ tags: readonly string[] | null }>()

const text = computed(() => (props.tags ?? []).join(', '))
</script>

<template>
  <AppTooltip v-if="tags?.length" :text="text">
    <span
      class="flex shrink-0 items-center text-text-4"
      role="img"
      :aria-label="`Tags: ${text}`"
      :data-count="tags.length > 1 ? 'many' : 'one'"
      data-testid="session-tags"
    >
      <component :is="tags.length > 1 ? IconTags : IconTag" class="size-3" aria-hidden="true" />
    </span>
  </AppTooltip>
</template>
