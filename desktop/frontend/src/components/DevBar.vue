<script setup lang="ts">
import { computed } from 'vue'
import { colorForBranch, readableTextColor } from '../lib/devBarColor'

// Dev-only status strip pinned to the bottom of the window. Rendered only when
// Vite serves the app in dev mode (import.meta.env.DEV). Its single link keeps
// development tools out of the persistent application chrome while retaining
// enough instance identity to distinguish concurrent worktrees.
const branch = import.meta.env.VITE_HIVE_DEV_BRANCH || 'unknown'
const port = window.location.port

const barColor = computed(() => colorForBranch(branch))
const textColor = computed(() => readableTextColor(barColor.value))
const label = computed(() => `Dev tools · ${branch}${port ? ` :${port}` : ''}`)
</script>

<template>
  <footer
    class="flex shrink-0 select-none items-center border-t border-black/20 px-3 py-1.5 font-mono text-xs leading-none"
    :style="{ background: barColor, color: textColor }"
  >
    <RouterLink
      :to="{ name: 'dev' }"
      :title="label"
      data-testid="devbar-link"
      class="cursor-pointer rounded px-2 py-1 text-[11px] font-bold hover:bg-black/15"
    >{{ label }}</RouterLink>
  </footer>
</template>
