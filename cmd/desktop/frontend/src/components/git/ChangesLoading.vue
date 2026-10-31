<script setup lang="ts">
import IconLoaderCircle from '~icons/lucide/loader-circle'
</script>

<template>
  <div class="flex h-full min-h-0 flex-1 items-center justify-center bg-app" data-testid="changes-loading">
    <!-- Only this part waits: the cover has to hold back the file tree and the diffs from the start, or a quick load would still show them apart. -->
    <span class="changes-loading-label flex flex-col items-center gap-3 text-body text-text-2" role="status">
      <IconLoaderCircle class="size-8 animate-spin text-accent" aria-hidden="true" />
      Loading changes…
    </span>
  </div>
</template>

<style scoped>
.changes-loading-label {
  animation: changes-loading-in 120ms 150ms both;
}
@keyframes changes-loading-in {
  from {
    opacity: 0;
  }
}
</style>
