<script setup lang="ts">
// An html document runs as written, so it gets an opaque origin: without
// allow-same-origin it cannot read the app's storage or call the host app,
// and the CSP Go puts first in <head> keeps it off localhost. allow-popups
// is left out until it is verified on WKWebView (ADR
// hive-drive-is-one-document-tree-under-the-context-root-and-a-canvas-is-one-document-kind-in-it).
//
// The frame fills DocumentReader, and for HTML DocumentReader fills the host's
// nearest positioned ancestor, so the document takes the whole view rather
// than the padded reading column.
//
// The wrapper is its own stacking context. The frame must paint over the
// reader's content, but a z-index set on the frame itself competes with menus
// and popovers of the host app in the root context, and the frame wins.
defineProps<{
  html: string
  /** A URL the frame loads instead of `html`, for a host that serves the document itself. */
  src?: string
  /** Derives `-frame`. */
  testid: string
}>()
</script>

<template>
  <div class="absolute inset-0 isolate">
    <iframe
      sandbox="allow-scripts"
      :src="src"
      :srcdoc="src ? undefined : html"
      referrerpolicy="no-referrer"
      title="HTML document"
      class="size-full border-0 bg-white"
      :data-testid="`${testid}-frame`"
    />
  </div>
</template>
