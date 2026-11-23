<script setup lang="ts">
import { computed, onErrorCaptured, ref, watch } from 'vue'
import { useCodeCopy } from '../../composables/useCodeCopy'
import { parseFenceBody } from '../../lib/fenceData'
import { renderFenceCode, type CanvasMarkdownPart } from '../../lib/githubMarkdown'
import { useCodeHighlight } from '../../lib/highlight'
import { useHiveUiPlatform } from '../../platform'
import InlineError from '../ui/InlineError.vue'
import { fenceRenderer, type FenceContext } from './registry'

const props = defineProps<{
  part: Extract<CanvasMarkdownPart, { kind: 'fence' }>
  ctx: FenceContext
  /** The wrapper's test id, `{doc testid}-fence-{index}`. The renderer gets it too. */
  testid: string
}>()

const { highlight } = useCodeHighlight()
const { copyCode } = useCodeCopy()
const platform = useHiveUiPlatform()

type Resolved =
  | { kind: 'render'; renderer: NonNullable<ReturnType<typeof fenceRenderer>>; data: unknown }
  | { kind: 'fallback'; error?: string }

const resolved = computed<Resolved>(() => {
  const renderer = fenceRenderer(props.part.lang)
  if (!renderer) return { kind: 'fallback' }
  let data: unknown = props.part.source
  if (renderer.body === 'yaml') {
    const parsed = parseFenceBody(props.part.source)
    if (!parsed.ok) return { kind: 'fallback', error: parsed.error }
    data = parsed.data
  }
  const checked = renderer.validate?.(data)
  if (checked && !checked.ok) return { kind: 'fallback', error: checked.error }
  return { kind: 'render', renderer, data: checked ? checked.data : data }
})

const crashed = ref(false)
watch(resolved, () => (crashed.value = false))

onErrorCaptured((cause) => {
  crashed.value = true
  platform.log('fences').warn('fence renderer failed', { lang: props.part.lang, error: String(cause) })
  return false
})

const showRenderer = computed(() => resolved.value.kind === 'render' && !crashed.value)
const error = computed(() => (resolved.value.kind === 'fallback' ? resolved.value.error : undefined))
const fallbackHtml = computed(() => renderFenceCode(props.part, highlight))
</script>

<template>
  <div
    class="canvas-fence"
    :data-fence-lang="part.lang"
    :data-source-start="part.lines?.[0]"
    :data-source-end="part.lines?.[1]"
    :data-testid="testid"
  >
    <component
      :is="resolved.renderer.component"
      v-if="showRenderer && resolved.kind === 'render'"
      :data="resolved.data"
      :source="part.source"
      :ctx="ctx"
      :testid="testid"
    />
    <template v-else>
      <InlineError v-if="error" :message="error" class="mb-2" :testid="`${testid}-error`" />
      <!-- eslint-disable-next-line vue/no-v-html -- renderFenceCode escapes the source through markdown-it's fence rule -->
      <div :data-testid="`${testid}-code`" @click="copyCode" v-html="fallbackHtml" />
    </template>
  </div>
</template>

<style scoped>
.canvas-fence {
  margin: 0.75em 0;
}
</style>
