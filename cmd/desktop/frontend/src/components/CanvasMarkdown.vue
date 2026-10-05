<script setup lang="ts">
import { computed } from 'vue'
import { renderCanvasMarkdown } from '../lib/githubMarkdown'
import CanvasMermaid from './CanvasMermaid.vue'

const props = defineProps<{
  source: string
  testid: string
}>()

const emit = defineEmits<{
  follow: [href: string]
}>()

const parts = computed(() => renderCanvasMarkdown(props.source))

function followLink(event: MouseEvent) {
  const target = event.target
  if (!(target instanceof Element)) return
  const anchor = target.closest('a')
  if (!anchor) return
  event.preventDefault()
  const href = anchor.getAttribute('href')
  if (href) emit('follow', href)
}
</script>

<template>
  <div>
    <template v-for="(part, index) in parts" :key="index">
      <!-- eslint-disable vue/no-v-html -- renderCanvasMarkdown escapes raw HTML and restricts links (githubMarkdown.spec.ts) -->
      <div
        v-if="part.kind === 'html'"
        :data-testid="`${testid}-html-${index}`"
        v-html="part.html"
        @click="followLink"
      />
      <!-- eslint-enable vue/no-v-html -->
      <CanvasMermaid v-else :source="part.source" :testid="`${testid}-mermaid-${index}`" />
    </template>
  </div>
</template>
