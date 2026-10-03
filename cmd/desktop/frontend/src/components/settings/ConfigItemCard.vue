<script setup lang="ts">
import type { Component } from 'vue'
import IconTrash2 from '~icons/lucide/trash-2'
import IconButton from '../ui/IconButton.vue'
import BaseButton from '../ui/BaseButton.vue'
import BaseCard from '../ui/BaseCard.vue'
import BaseIconBadge from '../ui/BaseIconBadge.vue'

defineProps<{
  title: string
  icon: Component
}>()
const emit = defineEmits<{ edit: [event: MouseEvent]; delete: [] }>()
</script>

<template>
  <BaseCard
    :padded="false"
    class="config-item group/item relative flex-wrap items-start gap-3 rounded-xl border border-card bg-raised px-4 py-3.5 transition-colors hover:border-strong @[600px]/pane:flex-nowrap @[600px]/pane:items-center @[600px]/pane:gap-4 [&.dragging]:opacity-45"
  >
    <template #icon>
      <slot name="leading" />
      <BaseIconBadge :size="38" rounded="rounded-xl" class="border border-accent/35 bg-accent-tint text-accent">
        <component :is="icon" class="size-[17px]" />
      </BaseIconBadge>
    </template>
    <div class="min-w-0 flex-1">
      <div class="truncate text-title font-semibold tracking-[-.01em] text-text">{{ title }}</div>
      <div class="mt-1.5 flex flex-wrap items-center gap-1.5">
        <slot name="badges" />
      </div>
    </div>
    <template #actions>
      <div class="flex w-full items-center justify-end gap-2 @[600px]/pane:w-auto @[600px]/pane:shrink-0">
        <BaseButton variant="secondary" size="xs" @click="emit('edit', $event)">Edit</BaseButton>
        <IconButton
          label="Delete"
          :icon="IconTrash2"
          size="xl"
          variant="outline"
          tone="danger"
          @click="emit('delete')"
        />
      </div>
    </template>
  </BaseCard>
</template>

<style scoped>
/* A host that reorders cards sets `dragging`, `drop-before` and `drop-after`.
   The insertion line floats in the gap between cards rather than lighting up a
   card's own border, which reads as an edit to that card and sits badly against
   the 11px corners. */
.config-item::before,
.config-item::after {
  content: '';
  position: absolute;
  left: 8px;
  right: 8px;
  height: 2px;
  border-radius: var(--radius-xs);
  background: var(--color-accent);
  opacity: 0;
  pointer-events: none;
}
.config-item::before {
  top: -7px;
}
.config-item::after {
  bottom: -7px;
}
.config-item.drop-before::before,
.config-item.drop-after::after {
  opacity: 1;
}
</style>
