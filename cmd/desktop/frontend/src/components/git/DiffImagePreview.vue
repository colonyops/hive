<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useStorage } from '@vueuse/core'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import SegmentedControl, { type SegmentedControlOption } from '@hive/ui/components/ui/SegmentedControl.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import { errorText } from '../../lib/appError'
import { formatBytes } from '../../lib/bytes'
import { useHiveUiPlatform } from '@hive/ui/platform'
import { lightThemes } from '@hive/ui/lib/themes'
import { backdropForPixels, imageSrc, type ImageBackdrop } from '../../lib/diffImages'
import type {
  ChangedFile,
  SessionFileImage,
  SessionImageVersion,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  file: ChangedFile
  /** Changes when the file does, so a held preview is read again. */
  version: string
  load: () => Promise<SessionFileImage>
}>()

type CompareMode = 'two-up' | 'swipe'
type Zoom = 'fit' | 'actual'
type Side = 'before' | 'after'

const compareOptions: SegmentedControlOption<CompareMode>[] = [
  { value: 'two-up', label: '2-up', title: 'Side by side' },
  { value: 'swipe', label: 'Swipe', title: 'Drag to reveal the new version over the old' },
]
const backdropOptions: SegmentedControlOption<ImageBackdrop>[] = [
  { value: 'light', label: 'Light', title: 'Light transparency grid' },
  { value: 'dark', label: 'Dark', title: 'Dark transparency grid' },
]
const zoomOptions: SegmentedControlOption<Zoom>[] = [
  { value: 'fit', label: 'Fit', title: 'Fit the frame' },
  { value: 'actual', label: '1:1', title: 'Actual size' },
]

// Shared by every preview: a reviewer comparing icons wants the same mode on the next one.
const storedCompare = useStorage<string>('hive.changes.image.compare', 'two-up')
const compare = computed<CompareMode>({
  get: () => (storedCompare.value === 'swipe' ? 'swipe' : 'two-up'),
  set: (mode) => (storedCompare.value = mode),
})
const zoom = useStorage<Zoom>('hive.changes.image.zoom', 'fit')
const swipe = ref(50)

// The backdrop suits each image, so a pick holds for this preview only.
const platform = useHiveUiPlatform()
const pickedBackdrop = ref<ImageBackdrop | null>(null)
const backdrop = computed<ImageBackdrop>({
  get: () =>
    pickedBackdrop.value ??
    after.value?.backdrop ??
    before.value?.backdrop ??
    (lightThemes.has(platform.theme.value) ? 'light' : 'dark'),
  set: (tone) => (pickedBackdrop.value = tone),
})

// The frame is a fixed height, so the item's height does not jump as images decode.
const FRAME_PX = 288
const PAD_PX = 12
// An icon this small is scaled up in whole steps with hard pixel edges. Anything
// larger is only ever scaled down, so a screenshot never turns soft.
const ICON_PX = 64

interface Dims {
  width: number
  height: number
}
interface Decoded {
  src: string
  dims: Dims
  version: SessionImageVersion
  /** The backdrop this version's pixels ask for, or null when none shows through. */
  backdrop: ImageBackdrop | null
}

// Enough pixels to judge an image's tone, few enough to read on every mount.
const SAMPLE_PX = 48

function sampleBackdrop(img: HTMLImageElement): ImageBackdrop | null {
  try {
    const canvas = document.createElement('canvas')
    canvas.width = canvas.height = SAMPLE_PX
    const context = canvas.getContext('2d', { willReadFrequently: true })
    if (!context) return null
    context.drawImage(img, 0, 0, SAMPLE_PX, SAMPLE_PX)
    return backdropForPixels(context.getImageData(0, 0, SAMPLE_PX, SAMPLE_PX).data)
  } catch {
    // A canvas the WebView refuses to read only loses the automatic pick.
    return null
  }
}

const image = ref<SessionFileImage | null>(null)
const decoded = ref<Partial<Record<Side, Decoded | null>>>({})
const failure = ref<string | null>(null)
const loading = ref(false)

async function decode(version: SessionImageVersion | null): Promise<Decoded | null> {
  const src = version ? imageSrc(version) : null
  if (!version || !src) return null
  const img = new Image()
  img.src = src
  try {
    await img.decode()
  } catch {
    return null
  }
  return { src, dims: { width: img.naturalWidth, height: img.naturalHeight }, version, backdrop: sampleBackdrop(img) }
}

// A version change starts a new read while the last may be in flight; only the
// newest one may write, or a slow old read would put back the old pixels.
let latestRead = 0

async function read(): Promise<void> {
  const mine = ++latestRead
  loading.value = true
  failure.value = null
  try {
    const next = await props.load()
    const [before, after] = await Promise.all([decode(next.before), decode(next.after)])
    if (mine !== latestRead) return
    image.value = next
    decoded.value = { before, after }
  } catch (err) {
    if (mine === latestRead) failure.value = errorText(err, 'Could not read this image.')
  } finally {
    if (mine === latestRead) loading.value = false
  }
}
onMounted(read)
watch(() => props.version, read)

const before = computed(() => decoded.value.before ?? null)
const after = computed(() => decoded.value.after ?? null)
const both = computed(() => !!before.value && !!after.value)
const identical = computed(() => both.value && before.value!.version.data === after.value!.version.data)

const STATE_NOTES: Record<string, string> = {
  too_large: 'Too large to preview',
  unavailable: 'Not a regular file',
  gone: 'No longer changed',
}
const note = computed(() => {
  const read = image.value
  if (!read) return null
  if (read.state === 'error') return read.error || 'Could not read this image.'
  if (read.state !== 'ready') return STATE_NOTES[read.state] ?? `Cannot preview: ${read.state}`
  const unreadable = (['before', 'after'] as const).filter((side) => read[side] && !decoded.value[side])
  if (unreadable.length === 2) return 'Neither version is an image this view can draw'
  if (unreadable.length === 1) return `The ${unreadable[0]} version is not an image this view can draw`
  return null
})

const showsTwoUp = computed(() => both.value && (compare.value === 'two-up' || identical.value))
const largest = computed<Dims>(() => ({
  width: Math.max(before.value?.dims.width ?? 0, after.value?.dims.width ?? 0),
  height: Math.max(before.value?.dims.height ?? 0, after.value?.dims.height ?? 0),
}))

// The frame's width follows the code column, which follows its content, so
// sizing must not measure it: an icon scales by the frame's fixed height, and
// anything else scales down in CSS, each side as a share of the larger one so
// a resized image reads as resized.
const iconScale = computed(() => {
  const { width, height } = largest.value
  if (zoom.value === 'actual' || !height || Math.max(width, height) > ICON_PX) return 1
  return Math.max(1, Math.floor((FRAME_PX - 2 * PAD_PX) / height))
})
const scalesInCss = computed(() => zoom.value === 'fit' && iconScale.value === 1)

function px(d: Dims): { width: string; height: string } {
  return { width: `${d.width * iconScale.value}px`, height: `${d.height * iconScale.value}px` }
}

/** A version on its own in a frame. */
function shown(d: Dims): Record<string, string> {
  if (!scalesInCss.value) return px(d)
  const { width, height } = largest.value
  return { maxWidth: `${(d.width / width) * 100}%`, maxHeight: `${(d.height / height) * 100}%` }
}

/** A version inside the overlay stage, which is the larger version's box. */
function layered(d: Dims): Record<string, string> {
  if (!scalesInCss.value) return px(d)
  const { width, height } = largest.value
  return { width: `${(d.width / width) * 100}%`, height: `${(d.height / height) * 100}%` }
}

const stage = computed(() => {
  if (!scalesInCss.value) return px(largest.value)
  const { width, height } = largest.value
  return { aspectRatio: `${width} / ${height}`, height: `min(100%, ${height}px)`, maxWidth: '100%' }
})
const pixelClass = computed(() => (iconScale.value > 1 ? '[image-rendering:pixelated]' : ''))

function dims(d: Dims | undefined): string {
  return d ? `${d.width} × ${d.height}` : '?'
}

const delta = computed(() => {
  if (!before.value || !after.value) return null
  const change = after.value.version.size - before.value.version.size
  if (change === 0) return null
  return {
    text: `${change > 0 ? '+' : '-'}${formatBytes(Math.abs(change))}`,
    tone: change > 0 ? 'text-severity-warning' : 'text-severity-success',
  }
})
const resized = computed(
  () =>
    both.value &&
    (before.value!.dims.width !== after.value!.dims.width || before.value!.dims.height !== after.value!.dims.height),
)
const single = computed(() => after.value ?? before.value)
// From what git read rather than what decoded, so a modified file whose new
// version is unreadable is not called deleted.
const sideLabel = computed(() => {
  const read = image.value
  if (read?.state !== 'ready' || (read.before && read.after)) return null
  return read.after ? 'Added' : 'Deleted'
})

const backdropClass = computed(() => `image-checker image-checker-${backdrop.value}`)
const prefix = computed(() => `changes-image-${props.file.path}`)
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-border bg-pane" :data-testid="prefix">
    <div class="flex flex-wrap items-center gap-2 border-b border-border px-2.5 py-1.5">
      <SegmentedControl
        v-if="both && !identical"
        v-model="compare"
        variant="compact"
        size="sm"
        :options="compareOptions"
        aria-label="Compare mode"
        :testid="`${prefix}-compare`"
      />
      <span v-else-if="identical" class="text-caption text-text-3">Same pixels</span>
      <span
        v-else-if="sideLabel"
        class="text-caption font-semibold"
        :class="sideLabel === 'Added' ? 'text-severity-success' : 'text-severity-error'"
        >{{ sideLabel }}</span
      >
      <span class="min-w-0 flex-1 truncate font-mono text-caption text-text-3" :data-testid="`${prefix}-meta`">
        <template v-if="both && !identical">
          {{ dims(before?.dims) }} → {{ dims(after?.dims) }} · {{ formatBytes(before!.version.size) }} →
          {{ formatBytes(after!.version.size) }}
          <span v-if="delta" :class="delta.tone">{{ delta.text }}</span>
        </template>
        <template v-else-if="single && !note">
          {{ dims(single.dims) }} · {{ formatBytes(single.version.size) }} · {{ single.version.mime }}
        </template>
      </span>
      <span
        v-if="resized"
        class="shrink-0 rounded bg-severity-warning-tint px-1.5 text-caption text-severity-warning"
        :data-testid="`${prefix}-resized`"
        >Resized</span
      >
      <SegmentedControl
        v-model="backdrop"
        variant="compact"
        size="sm"
        :options="backdropOptions"
        aria-label="Background"
        :testid="`${prefix}-backdrop`"
      />
      <SegmentedControl
        v-model="zoom"
        variant="compact"
        size="sm"
        :options="zoomOptions"
        aria-label="Zoom"
        :testid="`${prefix}-zoom`"
      />
    </div>

    <div>
      <div v-if="loading && !image" class="flex h-72 items-center justify-center text-text-3">
        <Spinner />
      </div>
      <div v-else-if="failure" class="px-3 py-3"><InlineError :message="failure" /></div>
      <div v-else-if="note" class="px-3 py-3 text-caption text-text-3" :data-testid="`${prefix}-note`">
        {{ note }}
      </div>

      <div
        v-else-if="!both && single"
        class="flex h-72 items-center justify-center overflow-auto p-3"
        :class="[backdropClass, after ? 'image-added' : 'image-deleted']"
      >
        <img :src="single.src" alt="" :style="shown(single.dims)" :class="pixelClass" />
      </div>

      <div v-else-if="showsTwoUp" class="grid h-72 grid-cols-2 divide-x divide-border">
        <div v-for="side in ['before', 'after'] as const" :key="side" class="relative min-h-0 min-w-0">
          <span
            class="absolute top-1.5 left-1.5 z-10 rounded bg-pane/90 px-1.5 py-0.5 text-micro font-semibold tracking-wider uppercase"
            :class="side === 'before' ? 'text-severity-error' : 'text-severity-success'"
            >{{ side }}</span
          >
          <div class="flex size-full items-center justify-center overflow-auto p-3" :class="backdropClass">
            <img
              :src="decoded[side]!.src"
              alt=""
              :style="shown(decoded[side]!.dims)"
              :class="pixelClass"
              :data-testid="`${prefix}-${side}`"
            />
          </div>
        </div>
      </div>

      <template v-else-if="both">
        <div class="flex h-72 items-center justify-center overflow-auto p-3" :class="backdropClass">
          <div class="relative shrink-0" :style="stage">
            <img
              :src="before!.src"
              alt=""
              class="absolute top-1/2 left-1/2 max-w-none -translate-1/2"
              :class="pixelClass"
              :style="layered(before!.dims)"
              :data-testid="`${prefix}-before`"
            />
            <div class="absolute inset-0" :style="{ clipPath: `inset(0 0 0 ${swipe}%)` }">
              <img
                :src="after!.src"
                alt=""
                class="absolute top-1/2 left-1/2 max-w-none -translate-1/2"
                :class="pixelClass"
                :style="layered(after!.dims)"
                :data-testid="`${prefix}-after`"
              />
            </div>
            <div
              class="pointer-events-none absolute inset-y-0 w-0.5 -translate-x-1/2 bg-accent"
              :style="{ left: `${swipe}%` }"
            />
            <input
              v-model.number="swipe"
              type="range"
              min="0"
              max="100"
              aria-label="Swipe position"
              class="absolute inset-0 size-full cursor-ew-resize opacity-0"
              :data-testid="`${prefix}-swipe`"
            />
          </div>
        </div>
        <div class="border-t border-border px-3 py-1.5 text-caption text-text-3">
          Drag across the image: before on the left, after on the right.
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.image-checker {
  background-color: var(--checker-a);
  background-image: conic-gradient(var(--checker-b) 25%, transparent 0 50%, var(--checker-b) 0 75%, transparent 0);
  background-size: 16px 16px;
}
.image-checker-light {
  --checker-a: #ffffff;
  --checker-b: #e5e5e5;
}
.image-checker-dark {
  --checker-a: #1c1c1e;
  --checker-b: #2c2c2e;
}
.image-added {
  box-shadow: inset 0 0 0 2px color-mix(in srgb, var(--color-severity-success) 45%, transparent);
}
.image-deleted {
  box-shadow: inset 0 0 0 2px color-mix(in srgb, var(--color-severity-error) 45%, transparent);
}
</style>
