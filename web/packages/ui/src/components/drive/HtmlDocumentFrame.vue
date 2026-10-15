<script setup lang="ts">
// An html document runs as written, so it gets an opaque origin: without
// allow-same-origin it cannot read the app's storage or call the host app,
// and the CSP Go puts first in <head> keeps it off localhost. allow-popups
// is left out until it is verified on WKWebView (ADR
// hive-drive-is-one-document-tree-under-the-context-root-and-a-canvas-is-one-document-kind-in-it).
//
// The frame fills the positioned DocumentReader. DocumentReader stretches to
// the available height for HTML because this absolute frame contributes no layout
// height.
defineProps<{
  html: string
  /** A URL the frame loads instead of `html`, for a host that serves the document itself. */
  src?: string
  /** Derives `-frame`. */
  testid: string
}>()
</script>

<template>
  <iframe
    sandbox="allow-scripts"
    :src="src"
    :srcdoc="src ? undefined : html"
    referrerpolicy="no-referrer"
    title="HTML document"
    class="absolute inset-0 z-30 h-full w-full border-0 bg-white"
    :data-testid="`${testid}-frame`"
  />
</template>
