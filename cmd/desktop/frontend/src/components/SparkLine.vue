<script setup lang="ts">
// A trend, not a chart: one stroke, no axes, no labels, no tooltip, no fill. It
// answers "is this number climbing" in a band of its own below the number, and
// anything more would compete with the number for the same attention.
//
// Two things keep it still while a poller feeds it. The x axis is fixed to
// `capacity` slots and the samples are right-aligned in them, so a new sample
// scrolls the line left instead of re-spacing every point — without that, each
// tick redraws the whole curve in a new place and the eye cannot follow it.
// And the y band is quantised and sticky: it only moves when the data leaves
// it or shrinks to a fraction of it, so a value wobbling by a megabyte does
// not rescale the whole card twice a second.
//
// The band is never zero-based. A process sitting at 175 MB against a zero
// baseline draws a flat line at the top of the box and reads as a rule.
import { computed, ref, watch } from 'vue'

// The caller owns the height — this sets none, so a bare `class="h-14"` lands
// rather than losing to an `h-full` here.
const props = withDefaults(defineProps<{
  values: number[]
  /** Slots on the x axis. Samples fill it from the right. */
  capacity?: number
}>(), { capacity: 40 })

const WIDTH = 100
const HEIGHT = 32
/** Below this share of the band, the data has shrunk enough to warrant a tighter one. */
const REBAND_BELOW = 0.35

interface Band { lo: number, hi: number }

const band = ref<Band | null>(null)

// Rounded to a power of ten of the series' own span, so successive samples land
// in the same band and the line holds position.
function quantise(min: number, max: number): Band {
  if (max <= min) {
    const pad = Math.abs(max) * 0.1 || 1
    return { lo: min - pad, hi: max + pad }
  }
  const step = 10 ** Math.floor(Math.log10(max - min))
  return { lo: Math.floor(min / step) * step, hi: Math.ceil(max / step) * step }
}

watch(() => props.values, (values) => {
  if (values.length === 0) {
    band.value = null
    return
  }
  const min = Math.min(...values)
  const max = Math.max(...values)
  const current = band.value
  const fits = current && min >= current.lo && max <= current.hi
  const fills = current && (max - min) >= (current.hi - current.lo) * REBAND_BELOW
  if (fits && (fills || max === min)) return
  band.value = quantise(min, max)
}, { immediate: true, deep: true })

const points = computed(() => {
  const values = props.values
  const current = band.value
  if (values.length < 2 || !current) return ''

  const span = current.hi - current.lo || 1
  const step = WIDTH / Math.max(1, props.capacity - 1)
  // Right-aligned: the newest sample is always at the right edge.
  const offset = WIDTH - (values.length - 1) * step
  return values.map((value, index) => {
    const x = Math.max(0, offset + index * step)
    const y = HEIGHT - ((value - current.lo) / span) * HEIGHT
    return `${x.toFixed(1)},${Math.min(HEIGHT, Math.max(0, y)).toFixed(1)}`
  }).join(' ')
})
</script>

<template>
  <svg
    :viewBox="`0 0 ${WIDTH} ${HEIGHT}`"
    preserveAspectRatio="none"
    aria-hidden="true"
    class="block w-full"
  >
    <polyline
      v-if="points"
      :points="points"
      fill="none"
      stroke="currentColor"
      stroke-width="1.6"
      stroke-linecap="round"
      stroke-linejoin="round"
      vector-effect="non-scaling-stroke"
    />
  </svg>
</template>
