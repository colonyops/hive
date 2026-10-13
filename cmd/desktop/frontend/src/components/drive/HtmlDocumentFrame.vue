<script setup lang="ts">
// An html document runs as written, so it gets an opaque origin: without
// allow-same-origin it cannot read the app's storage or call its bindings,
// and the CSP Go puts first in <head> keeps it off localhost. allow-popups
// is left out until it is verified on WKWebView (ADR
// hive-drive-is-one-document-tree-under-the-context-root-and-a-canvas-is-one-document-kind-in-it).
import IconShield from '~icons/lucide/shield'

defineProps<{
  html: string
  /** Derives `-frame` and `-frame-bar`. */
  testid: string
}>()
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border">
    <p
      class="flex shrink-0 items-center gap-1.5 border-b border-border bg-pane px-3 py-1.5 text-caption text-text-3"
      :data-testid="`${testid}-frame-bar`"
    >
      <IconShield class="size-3.5 shrink-0 text-text-4" aria-hidden="true" />
      Sandboxed document. Scripts run inside the frame; it cannot call Hive or send requests to localhost.
    </p>
    <iframe
      sandbox="allow-scripts"
      :srcdoc="html"
      referrerpolicy="no-referrer"
      title="HTML document"
      class="min-h-[480px] w-full flex-1 border-0 bg-white"
      :data-testid="`${testid}-frame`"
    />
  </div>
</template>
