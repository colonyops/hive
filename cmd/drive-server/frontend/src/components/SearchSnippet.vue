<script setup lang="ts">
import { computed } from 'vue'
import { snippetParts } from '../lib/snippet'

const props = defineProps<{ snippet: string }>()
const parts = computed(() => snippetParts(props.snippet))
</script>

<template>
  <span class="snippet"
    ><template v-for="(part, index) in parts" :key="index"
      ><mark v-if="part.match" class="rounded-sm bg-accent-tint text-accent">{{ part.text }}</mark
      ><template v-else>{{ part.text }}</template></template
    ></span
  >
</template>
