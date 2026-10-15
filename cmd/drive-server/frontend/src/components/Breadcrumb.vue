<script setup lang="ts">
import IconChevronRight from '~icons/lucide/chevron-right'

export interface Crumb {
  label: string
  to: string
}

defineProps<{ items: Crumb[] }>()
</script>

<template>
  <nav aria-label="Breadcrumb" data-testid="breadcrumb">
    <ol class="flex flex-wrap items-center gap-1 text-body">
      <li v-for="(item, index) in items" :key="item.to" class="flex items-center gap-1">
        <IconChevronRight v-if="index > 0" class="size-3 text-text-4" aria-hidden="true" />
        <span v-if="index === items.length - 1" class="font-medium text-text" aria-current="page">{{
          item.label
        }}</span>
        <RouterLink v-else :to="item.to" class="text-text-3 hover:text-text">{{ item.label }}</RouterLink>
      </li>
    </ol>
  </nav>
</template>
