import { useEventListener } from '@vueuse/core'
import { effectScope, onScopeDispose, toValue, watch, type EffectScope, type MaybeRefOrGetter } from 'vue'

interface Layer {
  order: number
  enabled: () => boolean
  onEscape: () => void
}

// Overlays stack (a confirm inside a drawer inside Settings), and one Escape
// must close only the top one. A plain window listener per caller would fire
// them all on the same keypress.
const layers = new Set<Layer>()
let nextOrder = 0
let listener: EffectScope | null = null

function onKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Escape') return
  let top: Layer | undefined
  for (const layer of layers) {
    if (layer.enabled() && (!top || layer.order > top.order)) top = layer
  }
  top?.onEscape()
}

/**
 * Calls `onEscape` when Escape is pressed and this caller is the topmost
 * enabled layer. A caller is on top from when it registers, and again each
 * time `enabled` turns true, so an always-mounted surface (the palette) that
 * opens over another overlay takes Escape first. A disabled caller is skipped,
 * so a surface that wants to swallow Escape while busy stays enabled and
 * ignores the key in `onEscape`.
 */
export function useEscapeToClose(onEscape: () => void, options: { enabled?: MaybeRefOrGetter<boolean> } = {}): void {
  const enabled = (): boolean => toValue(options.enabled ?? true)
  const layer: Layer = { order: ++nextOrder, enabled, onEscape }
  watch(enabled, (on) => {
    if (on) layer.order = ++nextOrder
  })

  layers.add(layer)
  if (!listener) {
    // Detached so the listener outlives whichever caller happened to start it.
    listener = effectScope(true)
    listener.run(() => useEventListener(window, 'keydown', onKeydown))
  }

  onScopeDispose(() => {
    layers.delete(layer)
    if (layers.size === 0) {
      listener?.stop()
      listener = null
    }
  })
}
