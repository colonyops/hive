<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { CodeView, type CodeViewFileItem, type CodeViewOptions } from '@pierre/diffs'
import { useHiveUiPlatform } from '@hive/ui/platform'
import { diffMetrics } from '../../lib/diffItems'
import { diffThemeName, diffWorkerPool } from '../../lib/diffHighlight'
import { useSurfaceScale } from '../../stores/useSurfaceScale'

const props = defineProps<{
  /** Identifies the file across reloads, so the pool's highlight cache survives one that changed nothing. */
  id: string
  name: string
  text: string
  /** Changes whenever text may have, so a reload of an edited file draws again. */
  version: string
  testid: string
}>()

// Same reason as DiffStream's SCROLLBAR_PX: CodeView lays the file out from
// itemMetrics, and the platform scrollbar would otherwise eat the last rows.
const SCROLLBAR_PX = 8
const PADDING_PX = 12

const platform = useHiveUiPlatform()
const log = platform.log('drive-code')
const pool = diffWorkerPool(platform.theme.value, log)
const { scales } = useSurfaceScale()
const metrics = computed(() => diffMetrics(scales.code.value, scales.interface.value))
const rowStyle = computed(() => ({
  '--diffs-font-size': `${metrics.value.fontPx}px`,
  '--diffs-line-height': `${metrics.value.rowPx}px`,
}))

const root = ref<HTMLElement | null>(null)
let viewer: CodeView | null = null
// CodeView keeps an item whose version it has already seen, contents and all.
let revision = 0

function viewerOptions(): CodeViewOptions<undefined, undefined> {
  return {
    // A wrapped line would break the fixed row height CodeView lays out from.
    overflow: 'scroll',
    disableFileHeader: true,
    stickyHeaders: false,
    lineHoverHighlight: 'number',
    layout: { paddingTop: 0, paddingBottom: 0, gap: 0 },
    itemMetrics: {
      lineHeight: metrics.value.rowPx,
      diffHeaderHeight: 0,
      spacing: 0,
      paddingTop: PADDING_PX,
      paddingBottom: PADDING_PX + SCROLLBAR_PX,
    },
    unsafeCSS: `:host { --diffs-bg: var(--color-app); --diffs-scrollbar-gutter-override: ${SCROLLBAR_PX}px; }`,
  }
}

function item(): CodeViewFileItem<undefined> {
  const cacheKey = `drive:${props.id}:${props.version}`
  return { id: props.id, type: 'file', version: revision, file: { name: props.name, contents: props.text, cacheKey } }
}

onMounted(() => {
  if (!root.value) return
  viewer = new CodeView<undefined>(viewerOptions(), pool)
  viewer.setup(root.value)
  viewer.setItems([item()])
})
onBeforeUnmount(() => {
  viewer?.cleanUp()
  viewer = null
})

watch(
  () => [props.id, props.text, props.version] as const,
  ([id], [previousId]) => {
    revision++
    viewer?.setItems([item()])
    if (id !== previousId) viewer?.scrollTo({ type: 'position', position: 0 })
  },
  { flush: 'post' },
)
watch(metrics, () => viewer?.setOptions(viewerOptions()), { flush: 'post' })
// Immediate for the reason DiffStream gives: the pool outlives every view, and
// the theme can change while none is open to hear of it.
watch(
  platform.theme,
  (theme) => {
    pool.setRenderOptions({ theme: diffThemeName(theme) }).catch((err: unknown) => {
      log.warn('could not switch the code theme', { err })
    })
  },
  { immediate: true },
)
</script>

<template>
  <div
    ref="root"
    class="drive-code hive-scroll absolute inset-0 overflow-y-auto bg-app outline-none"
    :style="rowStyle"
    data-scale-surface="code"
    tabindex="0"
    :aria-label="name"
    :data-testid="testid"
  />
</template>

<style scoped>
.drive-code {
  --diffs-font-family: var(--font-mono);
  --diffs-font-features: 'liga' 0, 'calt' 0;
}
</style>
