<script setup lang="ts">
import { computed } from 'vue'
import { useCodeCopy } from '../composables/useCodeCopy'
import { usePerf } from '../composables/usePerf'
import { renderCanvasMarkdown, type SourceLines } from '../lib/githubMarkdown'
import { useCodeHighlight } from '../stores/useCodeHighlight'
import CanvasMermaid from './CanvasMermaid.vue'

const props = defineProps<{
  source: string
  testid: string
  /** Marks each block with the source lines it came from (renderCanvasMarkdown). */
  sourceLines?: SourceLines
}>()

const emit = defineEmits<{
  follow: [href: string]
}>()

const perf = usePerf('drive')
const { highlight } = useCodeHighlight()
const parts = computed(() => {
  const end = perf.start('markdown:render', { chars: props.source.length })
  const rendered = renderCanvasMarkdown(props.source, highlight, props.sourceLines)
  end({ parts: rendered.length })
  return rendered
})

const { copyCode } = useCodeCopy()

function onClick(event: MouseEvent) {
  if (copyCode(event)) return
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
      <div v-if="part.kind === 'html'" :data-testid="`${testid}-html-${index}`" v-html="part.html" @click="onClick" />
      <!-- eslint-enable vue/no-v-html -->
      <CanvasMermaid
        v-else
        :source="part.source"
        :testid="`${testid}-mermaid-${index}`"
        :data-source-start="part.lines?.[0]"
        :data-source-end="part.lines?.[1]"
      />
    </template>
  </div>
</template>
