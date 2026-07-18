<script setup lang="ts">
import type { Component } from 'vue'
import IconDiamond from '~icons/lucide/diamond'
import IconList from '~icons/lucide/list'
import IconPlay from '~icons/lucide/play'
import IconRotateCcw from '~icons/lucide/rotate-ccw'
import IconSearch from '~icons/lucide/search'
import IconSparkles from '~icons/lucide/sparkles'
import IconZap from '~icons/lucide/zap'

defineProps<{ name: string }>()

// Icon names arrive as data (Action.icon from the Go backend). unplugin-icons
// resolves imports at build time, so every name the backend can send must be
// registered here.
const registry: Record<string, Component> = {
  diamond: IconDiamond,
  list: IconList,
  play: IconPlay,
  'rotate-ccw': IconRotateCcw,
  search: IconSearch,
  sparkles: IconSparkles,
}
</script>

<template>
  <component :is="registry[name] ?? IconZap" />
</template>
