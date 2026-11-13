<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useElementBounding } from '@vueuse/core'
import { useOpenModalCount } from '@hive/ui/composables/useOpenModalCount'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import {
  Close,
  Focus,
  Open,
  OpenPane,
  SetFrame,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/nativeterminalservice'
import { errorText } from '../../lib/appError'
import { useWailsEvent } from '../../composables/useWailsEvent'
import { useCommandPalette } from '../../composables/useCommands'

// A box in the page that a native libghostty surface covers. The webview draws
// nothing here: this element only reports where the surface goes, and stands in
// for it as the keymap's terminal target.
const props = defineProps<{
  /** Names the surface; unique across every open native tab. */
  id: string
  /** What the surface draws: a herdr-core terminal, or a pane of a tmux session. */
  target: { address: string } | { slug: string; paneId: string }
  onScreen: boolean
  wantsFocus: boolean
  /** Something floats over the page (the activity panel, an applet pop-up). */
  overlayOpen: boolean
}>()
const emit = defineEmits<{ focused: []; exited: [] }>()

/** The `nativeterm:event` payload (wailsui.NativeTerminalEvent). */
interface NativeTerminalEvent {
  id: string
  kind: 'closed' | 'title' | 'focus' | 'key' | 'open-url'
  data: string
}

const box = ref<HTMLElement | null>(null)
const bounds = useElementBounding(box)
const openModals = useOpenModalCount()
const palette = useCommandPalette()
// The surface is a native view above the whole page, so anything the page
// floats over it would be hidden behind it.
const covered = computed(() => openModals.value > 0 || palette.open.value || props.overlayOpen)
const opened = ref(false)
const error = ref<string | null>(null)

function place(): void {
  if (!opened.value) return
  const visible = props.onScreen && !covered.value && bounds.width.value > 0 && bounds.height.value > 0
  void SetFrame(props.id, bounds.left.value, bounds.top.value, bounds.width.value, bounds.height.value, visible)
}

let frame = 0
function schedulePlace(): void {
  if (frame) return
  frame = requestAnimationFrame(() => {
    frame = 0
    place()
  })
}

watch([bounds.left, bounds.top, bounds.width, bounds.height, () => props.onScreen, covered], schedulePlace)

watch(
  () => props.wantsFocus && props.onScreen && opened.value && !covered.value,
  (focus) => {
    if (focus) void Focus(props.id)
  },
  { immediate: true },
)

onMounted(async () => {
  try {
    if ('address' in props.target) await Open(props.id, props.target.address)
    else await OpenPane(props.id, props.target.slug, props.target.paneId)
    opened.value = true
    place()
  } catch (err) {
    error.value = errorText(err, 'Could not open the terminal.')
  }
})

onBeforeUnmount(() => {
  if (frame) cancelAnimationFrame(frame)
  void Close(props.id)
})

// A ⌘ chord the surface does not bind is replayed here, so the app keymap sees
// it as a key from a terminal pane.
function replayKey(data: string): void {
  const chord = JSON.parse(data) as { key: string; meta: boolean; ctrl: boolean; alt: boolean; shift: boolean }
  box.value?.dispatchEvent(
    new KeyboardEvent('keydown', {
      key: chord.key,
      metaKey: chord.meta,
      ctrlKey: chord.ctrl,
      altKey: chord.alt,
      shiftKey: chord.shift,
      bubbles: true,
      cancelable: true,
    }),
  )
}

useWailsEvent('nativeterm:event', (event) => {
  const payload = (Array.isArray(event.data) ? event.data[0] : event.data) as NativeTerminalEvent | undefined
  if (payload?.id !== props.id) return
  if (payload.kind === 'key') replayKey(payload.data)
  else if (payload.kind === 'focus' && payload.data === '1') emit('focused')
  else if (payload.kind === 'closed') emit('exited')
})
</script>

<template>
  <div
    ref="box"
    class="relative min-h-0 min-w-0 flex-1 bg-app"
    tabindex="-1"
    data-terminal-input-scope
    data-code-pane
    data-testid="native-terminal"
    :data-address="'address' in target ? target.address : target.paneId"
  >
    <div v-if="error" class="flex h-full items-center justify-center p-6">
      <InlineError :message="error" />
    </div>
  </div>
</template>
