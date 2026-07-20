<script setup lang="ts">
// Dev-only status strip pinned to the bottom of the window. Rendered only when
// Vite serves the app in dev mode (import.meta.env.DEV) — i.e. under
// `wails3 dev` — so it can never appear in a production `wails3 build`, the
// headless server build, or e2e. It exists to make concurrently-running dev
// instances distinguishable at a glance: the branch is injected at launch by
// the dev task, and the port is the Vite dev server this window loaded from.
const branch = import.meta.env.VITE_HIVE_DEV_BRANCH || 'unknown'
const port = window.location.port
</script>

<template>
  <footer
    class="flex shrink-0 select-none items-center gap-2.5 border-t border-black/20 bg-yellow-400 px-3 py-1.5 font-mono text-xs leading-none text-black"
  >
    <span class="rounded bg-black px-2 py-1 text-[11px] font-bold uppercase tracking-wider text-yellow-400">Dev</span>
    <span class="font-semibold">{{ branch }}</span>
    <span v-if="port" class="text-black/60">:{{ port }}</span>
    <span class="ml-auto text-black/70">dev instance</span>
  </footer>
</template>
