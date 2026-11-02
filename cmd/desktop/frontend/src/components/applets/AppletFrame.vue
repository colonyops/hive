<script setup lang="ts">
import { useEventListener } from '@vueuse/core'
import { computed, ref, watch } from 'vue'
import IconX from '~icons/lucide/x'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import { useEscapeToClose } from '@hive/ui/composables/useEscapeToClose'
import type { Applet } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { isEditableTarget } from '../../lib/isEditableTarget'
import { usePopupTerminal } from '../../stores/usePopupTerminal'
import { SURFACES, type AppletSurface } from '../../stores/appletSurfaces'
import { useAppletBridge } from './useAppletBridge'

// Hosts one of an applet's pages. The frame has an opaque origin, so a page can do what
// it likes to itself without reaching Hive's DOM or storage, and what it asks of the
// machine arrives through useAppletBridge
// (ADR applets-are-web-pages-with-full-access-to-the-machine).

const props = withDefaults(
  defineProps<{ applet: Applet; sessionSlug: string; surface?: AppletSurface; entry?: string; wantsFocus?: boolean }>(),
  { surface: 'page', entry: undefined, wantsFocus: false },
)
const emit = defineEmits<{ close: []; resize: [size: { width?: number; height?: number }]; focused: [] }>()

const rules = computed(() => SURFACES[props.surface])
const isPopup = computed(() => props.surface === 'page')
const popupTerminal = usePopupTerminal()
const frame = ref<HTMLIFrameElement | null>(null)

const src = computed(() => {
  const path = (props.entry ?? props.applet.entry).split('/').map(encodeURIComponent).join('/')
  const surface = isPopup.value ? '' : `&surface=${props.surface}`
  return `/__applet/${encodeURIComponent(props.applet.id)}/${path}?r=${props.applet.revision}${surface}`
})

useAppletBridge({
  applet: () => props.applet,
  frame,
  surface: () => props.surface,
  sessionSlug: () => props.sessionSlug,
  close: () => emit('close'),
  resize: (size) => emit('resize', size),
})

// A page's Escape arrives through the bridge. This one is for focus outside the frame.
// A surface with nothing to close must not take Escape from whatever has focus.
useEscapeToClose(() => emit('close'), { enabled: () => rules.value.escapeCloses && !popupTerminal.visible.value })

// A page that loads with the app and on every edit must not take focus from what the
// user is typing in.
function focusFrame(): void {
  const wanted = props.wantsFocus && !isEditableTarget(document.activeElement)
  if (rules.value.takesFocus || wanted) frame.value?.focus()
}

watch(
  () => props.wantsFocus,
  (wants) => {
    if (wants) focusFrame()
  },
)

// A press inside the frame never reaches the app, so the app learns of it from focus
// leaving its own document. The element's own event covers a move from another frame,
// where the window has nothing left to blur.
function reportFocus(): void {
  setTimeout(() => {
    if (frame.value && document.activeElement === frame.value) emit('focused')
  })
}
useEventListener(window, 'blur', reportFocus)
useEventListener(frame, 'focus', reportFocus)
</script>

<template>
  <div class="relative min-h-0 flex-1" :data-testid="isPopup ? 'applet-page' : undefined">
    <iframe
      ref="frame"
      :key="applet.revision"
      :src="src"
      :title="applet.title"
      sandbox="allow-scripts allow-forms allow-modals allow-popups allow-popups-to-escape-sandbox allow-downloads allow-pointer-lock"
      allow="autoplay; fullscreen; clipboard-read; clipboard-write; camera; microphone; gamepad"
      class="absolute inset-0 size-full border-0"
      :data-testid="isPopup ? 'applet-frame' : `applet-${surface}-frame`"
      @load="focusFrame"
    />
    <IconButton
      v-if="isPopup"
      class="absolute top-2 right-2 opacity-40 hover:opacity-100"
      label="Close"
      :icon="IconX"
      size="md"
      data-testid="applet-page-close"
      @click="emit('close')"
    />
  </div>
</template>
