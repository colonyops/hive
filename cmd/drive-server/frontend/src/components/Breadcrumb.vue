<script setup lang="ts">
import IconChevronRight from '~icons/lucide/chevron-right'

export interface Crumb {
  label: string
  to: string
}

defineProps<{
  items: Crumb[]
  /** A small, muted trail above a page title. Without it the trail is the page title. */
  compact?: boolean
}>()
</script>

<template>
  <nav aria-label="Breadcrumb" data-testid="breadcrumb">
    <ol class="flex flex-wrap items-center gap-x-1.5 gap-y-0.5" :class="compact ? 'text-small' : 'text-page'">
      <li v-for="(item, index) in items" :key="item.to" class="flex min-w-0 items-center gap-1.5">
        <IconChevronRight
          v-if="index > 0"
          :class="compact ? 'size-4' : 'size-5'"
          class="text-text-4"
          aria-hidden="true"
        />
        <span
          v-if="!compact && index === items.length - 1"
          class="truncate font-semibold text-text"
          aria-current="page"
          >{{ item.label }}</span
        >
        <RouterLink v-else :to="item.to" class="truncate rounded-md px-1.5 text-text-3 hover:bg-chip hover:text-text">{{
          item.label
        }}</RouterLink>
      </li>
    </ol>
  </nav>
</template>
