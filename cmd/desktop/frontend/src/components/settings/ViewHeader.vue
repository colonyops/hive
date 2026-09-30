<script setup lang="ts">
// The title strip for a full-frame view (settings, activity, dev tools).
//
// There is deliberately no close button: the title bar already owns going
// back, and every consumer closes on Escape. A second "Back to feed" control
// inside the view duplicated that and competed with the page title for the
// only two anchors in the strip.
</script>

<template>
  <header class="flex h-11 shrink-0 items-center gap-2.5 border-b border-row bg-canvas-toolbar px-4">
    <slot name="title" />
  </header>
</template>
