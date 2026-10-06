<script setup lang="ts">
import { useResizeObserver } from '@vueuse/core'
import { computed, nextTick, ref, watch } from 'vue'
import IconMaximize from '~icons/lucide/maximize'
import IconMinus from '~icons/lucide/minus'
import IconPlus from '~icons/lucide/plus'
import InlineError from './ui/InlineError.vue'
import IconButton from './ui/IconButton.vue'
import { useTheme, type Theme } from '../composables/useTheme'
import { renderMermaid, type MermaidPalette } from '../lib/mermaid'

const props = defineProps<{
  source: string
  testid: string
}>()

type Bounds = { x: number; y: number; width: number; height: number }
type Point = { x: number; y: number }

const minZoom = 0.2
const maxZoom = 8
const svg = ref('')
const error = ref('')
const root = ref<HTMLElement | null>(null)
const viewport = ref<HTMLElement | null>(null)
const svgHost = ref<HTMLElement | null>(null)
const viewportHeight = ref(220)
const contentBounds = ref<Bounds | null>(null)
const fittedBounds = ref<Bounds | null>(null)
const viewBounds = ref<Bounds | null>(null)
const zoom = ref(1)
const autoFit = ref(true)
const dragging = ref(false)
const dragStart = ref<{ pointer: Point; view: Bounds } | null>(null)
const { theme } = useTheme()
let revision = 0

const lightThemes = new Set<Theme>([
  'light',
  'slate-light',
  'one-light',
  'tokyo-night-day',
  'catppuccin-latte',
  'nord-light',
])

const viewBox = computed(() => {
  const bounds = viewBounds.value
  return bounds ? `${bounds.x} ${bounds.y} ${bounds.width} ${bounds.height}` : ''
})
const zoomLabel = computed(() => `${Math.round(zoom.value * 100)}%`)

function cssValue(styles: CSSStyleDeclaration, name: string, fallback: string): string {
  return styles.getPropertyValue(name).trim() || fallback
}

function currentPalette(): MermaidPalette {
  const styles = getComputedStyle(document.documentElement)
  return {
    background: cssValue(styles, '--hv-pane', '#151922'),
    surface: cssValue(styles, '--hv-raised', '#1b2029'),
    surfaceAlt: cssValue(styles, '--hv-chip', '#232a36'),
    border: cssValue(styles, '--hv-strong', '#414d5e'),
    text: cssValue(styles, '--hv-text', '#e9edf4'),
    mutedText: cssValue(styles, '--hv-text-2', '#a6b0c0'),
    accent: cssValue(styles, '--hv-accent', '#f5b23f'),
    darkMode: !lightThemes.has(theme.value),
    fontFamily: cssValue(styles, '--font-sans', 'Inter, system-ui, sans-serif'),
  }
}

function errorMessage(value: unknown): string {
  if (!(value instanceof Error) || !value.message.trim()) return 'Could not render Mermaid diagram.'
  const detail = value.message.trim().split('\n', 1)[0]
  return `Could not render Mermaid diagram: ${detail}`
}

function parseViewBox(element: SVGSVGElement): Bounds | null {
  const values = element
    .getAttribute('viewBox')
    ?.trim()
    .split(/[\s,]+/)
    .map(Number)
  if (!values || values.length !== 4 || values.some((value) => !Number.isFinite(value))) return null
  const [x, y, width, height] = values
  return width > 0 && height > 0 ? { x, y, width, height } : null
}

function fitBounds(content: Bounds, width: number, height: number): Bounds {
  const viewportRatio = width / height
  const contentRatio = content.width / content.height
  if (viewportRatio > contentRatio) {
    const fittedWidth = content.height * viewportRatio
    return {
      x: content.x - (fittedWidth - content.width) / 2,
      y: content.y,
      width: fittedWidth,
      height: content.height,
    }
  }
  const fittedHeight = content.width / viewportRatio
  return {
    x: content.x,
    y: content.y - (fittedHeight - content.height) / 2,
    width: content.width,
    height: fittedHeight,
  }
}

function layout(): void {
  const element = svgHost.value?.querySelector('svg')
  if (!(element instanceof SVGSVGElement)) return
  const content = contentBounds.value ?? parseViewBox(element)
  if (!content) return

  contentBounds.value = content
  const width = root.value?.clientWidth || content.width
  const naturalScale = Math.min(1, width / content.width)
  const maxHeight = Math.min(720, Math.max(360, window.innerHeight * 0.65))
  viewportHeight.value = Math.min(maxHeight, Math.max(220, content.height * naturalScale))
  const fitted = fitBounds(content, width, viewportHeight.value)
  fittedBounds.value = fitted
  if (autoFit.value || !viewBounds.value) {
    viewBounds.value = fitted
    zoom.value = 1
  }
}

function fit(): void {
  autoFit.value = true
  layout()
}

function setZoom(next: number, anchor: Point = { x: 0.5, y: 0.5 }): void {
  const fitted = fittedBounds.value
  const current = viewBounds.value
  if (!fitted || !current) return
  const bounded = Math.min(maxZoom, Math.max(minZoom, next))
  const width = fitted.width / bounded
  const height = fitted.height / bounded
  const point = {
    x: current.x + current.width * anchor.x,
    y: current.y + current.height * anchor.y,
  }
  viewBounds.value = {
    x: point.x - width * anchor.x,
    y: point.y - height * anchor.y,
    width,
    height,
  }
  zoom.value = bounded
  autoFit.value = false
}

function zoomBy(factor: number): void {
  setZoom(zoom.value * factor)
}

function onWheel(event: WheelEvent): void {
  if (!event.ctrlKey && !event.metaKey) return
  event.preventDefault()
  const rect = viewport.value?.getBoundingClientRect()
  if (!rect?.width || !rect.height) return
  setZoom(zoom.value * Math.exp(-event.deltaY * 0.002), {
    x: (event.clientX - rect.left) / rect.width,
    y: (event.clientY - rect.top) / rect.height,
  })
}

function onPointerDown(event: PointerEvent): void {
  if (event.button !== 0 || !viewBounds.value) return
  viewport.value?.setPointerCapture?.(event.pointerId)
  dragging.value = true
  autoFit.value = false
  dragStart.value = {
    pointer: { x: event.clientX, y: event.clientY },
    view: { ...viewBounds.value },
  }
}

function onPointerMove(event: PointerEvent): void {
  const start = dragStart.value
  const rect = viewport.value?.getBoundingClientRect()
  if (!start || !rect?.width || !rect.height) return
  viewBounds.value = {
    ...start.view,
    x: start.view.x - ((event.clientX - start.pointer.x) * start.view.width) / rect.width,
    y: start.view.y - ((event.clientY - start.pointer.y) * start.view.height) / rect.height,
  }
}

function onPointerUp(event: PointerEvent): void {
  if (!dragStart.value) return
  viewport.value?.releasePointerCapture?.(event.pointerId)
  dragging.value = false
  dragStart.value = null
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === '+' || event.key === '=') zoomBy(1.25)
  else if (event.key === '-') zoomBy(0.8)
  else if (event.key === '0') fit()
  else return
  event.preventDefault()
}

async function render(): Promise<void> {
  const requestedRevision = ++revision
  svg.value = ''
  error.value = ''
  contentBounds.value = null
  fittedBounds.value = null
  viewBounds.value = null
  autoFit.value = true
  try {
    const rendered = await renderMermaid(props.source, currentPalette())
    if (requestedRevision !== revision) return
    svg.value = rendered
    await nextTick()
    if (requestedRevision === revision) layout()
  } catch (cause: unknown) {
    if (requestedRevision === revision) error.value = errorMessage(cause)
  }
}

useResizeObserver(root, () => {
  if (svg.value && autoFit.value) layout()
})
watch(viewBox, (value) => {
  const element = svgHost.value?.querySelector('svg')
  if (element && value) element.setAttribute('viewBox', value)
})
watch([() => props.source, theme], () => void render(), { immediate: true, flush: 'post' })
</script>

<template>
  <div ref="root" class="canvas-mermaid" :data-testid="testid">
    <template v-if="svg">
      <div
        ref="viewport"
        class="canvas-mermaid-viewport"
        :class="{ 'is-dragging': dragging }"
        :style="{ height: `${viewportHeight}px` }"
        :data-testid="`${testid}-viewport`"
        role="region"
        aria-label="Interactive Mermaid diagram. Drag to pan, use Control or Command with scroll to zoom, and press 0 to fit."
        tabindex="0"
        @dblclick="fit"
        @keydown="onKeydown"
        @pointerdown="onPointerDown"
        @pointermove="onPointerMove"
        @pointerup="onPointerUp"
        @pointercancel="onPointerUp"
        @wheel="onWheel"
      >
        <!-- eslint-disable-next-line vue/no-v-html -- Mermaid sanitizes generated SVG with securityLevel=strict and disables links. -->
        <div ref="svgHost" class="canvas-mermaid-svg" v-html="svg" />
        <div class="canvas-mermaid-controls" :data-testid="`${testid}-controls`" @pointerdown.stop>
          <IconButton
            label="Zoom out"
            :icon="IconMinus"
            size="sm"
            :disabled="zoom <= minZoom"
            :data-testid="`${testid}-zoom-out`"
            @click="zoomBy(0.8)"
          />
          <output class="canvas-mermaid-zoom" :data-testid="`${testid}-zoom`">{{ zoomLabel }}</output>
          <IconButton
            label="Zoom in"
            :icon="IconPlus"
            size="sm"
            :disabled="zoom >= maxZoom"
            :data-testid="`${testid}-zoom-in`"
            @click="zoomBy(1.25)"
          />
          <IconButton label="Fit diagram" :icon="IconMaximize" size="sm" :data-testid="`${testid}-fit`" @click="fit" />
        </div>
      </div>
    </template>
    <InlineError v-else-if="error" :message="error" variant="banner" :testid="`${testid}-error`" />
  </div>
</template>

<style scoped>
.canvas-mermaid {
  width: 100%;
  margin: 0.75em 0;
}
.canvas-mermaid-viewport {
  position: relative;
  width: 100%;
  min-height: 220px;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  background-color: var(--hv-pane);
  background-image: radial-gradient(circle, color-mix(in srgb, var(--hv-text-3) 24%, transparent) 1px, transparent 1px);
  background-size: 18px 18px;
  cursor: grab;
  touch-action: none;
  user-select: none;
}
.canvas-mermaid-viewport:focus-visible {
  outline: 2px solid var(--color-accent);
  outline-offset: 2px;
}
.canvas-mermaid-viewport.is-dragging {
  cursor: grabbing;
}
.canvas-mermaid-svg {
  position: absolute;
  inset: 0;
}
.canvas-mermaid-svg :deep(svg) {
  display: block;
  width: 100% !important;
  max-width: none !important;
  height: 100% !important;
}
.canvas-mermaid-controls {
  position: absolute;
  top: 8px;
  right: 8px;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 3px;
  border: 1px solid var(--color-border);
  border-radius: 7px;
  background: color-mix(in srgb, var(--hv-raised) 92%, transparent);
  box-shadow: 0 4px 14px rgb(0 0 0 / 18%);
}
.canvas-mermaid-zoom {
  min-width: 40px;
  color: var(--hv-text-2);
  font-family: var(--font-mono);
  font-size: 10px;
  line-height: 1;
  text-align: center;
}
</style>
