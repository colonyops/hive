<script setup lang="ts">
import { computed } from 'vue'
import { colorForBranch, readableTextColor } from '../lib/devBarColor'

// Dev-only status strip pinned to the bottom of the window. Rendered only when
// Vite serves the app in dev mode (import.meta.env.DEV) — i.e. under
// `wails3 dev` — so it can never appear in a production `wails3 build`, the
// headless server build, or e2e. It exists to make concurrently-running dev
// instances distinguishable at a glance: the branch is injected at launch by
// the dev task, and the port is the Vite dev server this window loaded from.
//
// The bar's color is derived from the branch name (see devBarColor.ts), so each
// branch/worktree window gets a stable, distinct accent — no more guessing
// which window is which.
const branch = import.meta.env.VITE_HIVE_DEV_BRANCH || 'unknown'
const port = window.location.port

const barColor = computed(() => colorForBranch(branch))
const textColor = computed(() => readableTextColor(barColor.value))
</script>

<template>
  <footer
    class="flex shrink-0 select-none items-center gap-2.5 border-t border-black/20 px-3 py-1.5 font-mono text-xs leading-none"
    :style="{ background: barColor, color: textColor }"
  >
    <span class="rounded px-2 py-1 text-[11px] font-bold uppercase tracking-wider" :style="{ background: textColor, color: barColor }">Dev</span>
    <span class="font-semibold">{{ branch }}</span>
    <span v-if="port" class="opacity-60">:{{ port }}</span>
    <span class="ml-auto opacity-70">dev instance</span>
  </footer>
</template>
