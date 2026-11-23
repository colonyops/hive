<script setup lang="ts">
import { computed } from 'vue'
import { useCodeCopy } from '../composables/useCodeCopy'
import { useHiveUiPlatform } from '../platform'
import { renderCanvasMarkdown, type SourceLines } from '../lib/githubMarkdown'
import { useCodeHighlight } from '../lib/highlight'
import './fences/builtins'
import FenceHost from './fences/FenceHost.vue'
import { isFence, type FenceContext } from './fences/registry'

const props = defineProps<{
  source: string
  testid: string
  /** Marks each block with the source lines it came from (renderCanvasMarkdown). */
  sourceLines?: SourceLines
  /** Turns [[wikilinks]] on and tells fences which document they are in. */
  links?: { docPath: string; paths: ReadonlySet<string> }
}>()

const emit = defineEmits<{
  follow: [href: string]
}>()

const { perf } = useHiveUiPlatform()
const { highlight } = useCodeHighlight()
const parts = computed(() => {
  const span = perf?.start('markdown:render', { chars: props.source.length })
  const rendered = renderCanvasMarkdown(props.source, {
    highlight,
    sourceLines: props.sourceLines,
    isFence,
    links: props.links,
  })
  if (perf) perf.end(span, { parts: rendered.length })
  return rendered
})

const { copyCode } = useCodeCopy()

const fenceContext = computed<FenceContext>(() => ({
  docPath: props.links?.docPath ?? '',
  paths: props.links?.paths ?? new Set(),
  follow: (href) => emit('follow', href),
}))

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
      <FenceHost v-else :part="part" :ctx="fenceContext" :testid="`${testid}-fence-${index}`" />
    </template>
  </div>
</template>
