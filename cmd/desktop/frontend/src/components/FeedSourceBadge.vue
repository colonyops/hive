<script setup lang="ts">
import type { Component } from 'vue'
import SourceMark from './SourceMark.vue'

// The source tile shared by the feed row (`md`) and the detail header (`sm`).
withDefaults(defineProps<{ icon: Component; image?: string; source?: string; size?: 'sm' | 'md' }>(), {
  image: undefined,
  source: undefined,
  size: 'md',
})
</script>

<template>
  <span
    :data-source="source"
    :class="[
      'inline-flex flex-none items-center justify-center rounded-lg border border-strong bg-chip text-text',
      size === 'md' ? 'size-[30px]' : 'size-[26px]',
    ]"
    ><SourceMark :icon="icon" :image="image" :class="size === 'md' ? 'size-4' : 'size-[15px]'"
  /></span>
</template>
