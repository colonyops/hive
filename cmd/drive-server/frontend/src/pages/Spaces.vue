<script setup lang="ts">
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
</script>

<template>
  <main class="mx-auto max-w-3xl px-4 py-8">
    <h1 class="mb-4 text-heading font-semibold text-text">Spaces</h1>
    <EmptyState variant="boxed" message="No documents yet. Push one with hive drive push." />
  </main>
</template>
