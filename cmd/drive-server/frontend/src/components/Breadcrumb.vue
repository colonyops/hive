<script setup lang="ts">
import IconChevronRight from '~icons/material-symbols/chevron-right'

export interface Crumb {
  label: string
  to: string
}

defineProps<{
  items: Crumb[]
  /** A small, muted trail under a title, flush with the title's left edge. Without it the trail is the page title. */
  compact?: boolean
}>()
</script>

<template>
  <nav aria-label="Breadcrumb" data-testid="breadcrumb">
    <ol
      class="flex items-center gap-x-1"
      :class="[compact ? 'flex-nowrap overflow-hidden text-small leading-tight' : 'flex-wrap gap-y-0.5 text-page']"
    >
      <li v-for="(item, index) in items" :key="item.to" class="flex min-w-0 items-center gap-1.5">
        <IconChevronRight
          v-if="index > 0"
          :class="compact ? 'size-3.5 shrink-0' : 'size-5'"
          class="text-text-4"
          aria-hidden="true"
        />
        <span
          v-if="!compact && index === items.length - 1"
          class="truncate font-semibold text-text"
          aria-current="page"
          >{{ item.label }}</span
        >
        <RouterLink
          v-else
          :to="item.to"
          class="truncate text-text-3 hover:text-text"
          :class="compact ? 'hover:underline' : 'rounded-md px-1.5 hover:bg-chip'"
          >{{ item.label }}</RouterLink
        >
      </li>
    </ol>
  </nav>
</template>
