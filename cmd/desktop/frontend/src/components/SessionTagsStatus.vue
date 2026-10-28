<script setup lang="ts">
// The status bar has room for the attached session's tags, which the
// sidebar row only counts. Clicking them opens the editor.
import IconTag from '~icons/lucide/tag'
import AppTooltip from '@hive/ui/components/ui/AppTooltip.vue'

defineProps<{ tags: readonly string[] | null }>()
const emit = defineEmits<{ edit: [] }>()
</script>

<template>
  <div v-if="tags?.length" class="flex min-w-0 shrink items-center text-caption">
    <span class="mx-2 h-3.5 w-px shrink-0 bg-border" aria-hidden="true" />
    <AppTooltip text="Edit tags">
      <button
        type="button"
        class="flex h-6 min-w-0 items-center gap-1 rounded px-1.5 text-text-3 hover:bg-hover hover:text-text-2"
        data-testid="session-status-tags"
        @click="emit('edit')"
      >
        <IconTag class="size-3 shrink-0 text-text-4" aria-hidden="true" />
        <span class="truncate">{{ tags.join(', ') }}</span>
      </button>
    </AppTooltip>
  </div>
</template>
