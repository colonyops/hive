<script setup lang="ts">
defineProps<{
  title: string
  description?: string
}>()
</script>

<template>
  <section>
    <h2 class="text-[15px] font-semibold text-text">{{ title }}</h2>
    <p v-if="description" class="mt-1 text-xs leading-relaxed text-text-3">{{ description }}</p>
    <slot />
  </section>
</template>
