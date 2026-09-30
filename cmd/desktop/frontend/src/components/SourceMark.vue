<script setup lang="ts">
import { ref, watch } from 'vue'
import type { Component } from 'vue'

// Renders an inbox item's source badge: the `image` data URL when set, else the
// `icon` glyph (also the fallback when the image fails to load).
const props = defineProps<{ icon: Component; image?: string }>()

const failed = ref(false)
watch(() => props.image, () => { failed.value = false })
</script>

<template>
  <!-- The image carries no size of its own: `size-full` here would outrank the
       caller's `size-4` in the utility layer, so an image mark filled the badge
       edge to edge while a glyph mark sat inset, and a failed image snapped
       between the two. Both branches take the caller's size. -->
  <img v-if="image && !failed" :src="image" alt="" class="object-contain" @error="failed = true">
  <component :is="icon" v-else aria-hidden="true" />
</template>
