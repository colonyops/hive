<script setup lang="ts">
import { computed, ref } from 'vue'
import IconX from '~icons/lucide/x'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import { useEscapeToClose } from '@hive/ui/composables/useEscapeToClose'
import type { Applet } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { usePopupTerminal } from '../../stores/usePopupTerminal'
import { useAppletBridge, type AppletSurface } from './useAppletBridge'

// Hosts one of an applet's pages. The frame has an opaque origin, so a page can do what
// it likes to itself without reaching Hive's DOM or storage, and what it asks of the
// machine arrives through useAppletBridge
// (ADR applets-are-web-pages-with-full-access-to-the-machine).

const props = withDefaults(
  defineProps<{ applet: Applet; sessionSlug: string; surface?: AppletSurface; entry?: string }>(),
  { surface: 'page', entry: undefined },
)
const emit = defineEmits<{ close: []; resize: [size: { width?: number; height?: number }] }>()

const popupTerminal = usePopupTerminal()
const frame = ref<HTMLIFrameElement | null>(null)

const src = computed(() => {
  const path = (props.entry ?? props.applet.entry).split('/').map(encodeURIComponent).join('/')
  const surface = props.surface === 'page' ? '' : `&surface=${props.surface}`
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
// The title bar has nothing to close, and must not take Escape from whatever has focus.
useEscapeToClose(() => emit('close'), { enabled: () => props.surface !== 'titlebar' && !popupTerminal.visible.value })

// A title bar page loads with the app and on every edit, and must not take focus from
// what the user is typing in.
function focusFrame(): void {
  if (props.surface !== 'titlebar') frame.value?.focus()
}
</script>

<template>
  <div class="relative min-h-0 flex-1" :data-testid="surface === 'page' ? 'applet-page' : undefined">
    <iframe
      ref="frame"
      :key="applet.revision"
      :src="src"
      :title="applet.title"
      sandbox="allow-scripts allow-forms allow-modals allow-popups allow-popups-to-escape-sandbox allow-downloads allow-pointer-lock"
      allow="autoplay; fullscreen; clipboard-read; clipboard-write; camera; microphone; gamepad"
      class="absolute inset-0 size-full border-0"
      :data-testid="surface === 'page' ? 'applet-frame' : `applet-${surface}-frame`"
      @load="focusFrame"
    />
    <IconButton
      v-if="surface === 'page'"
      class="absolute top-2 right-2 opacity-40 hover:opacity-100"
      label="Close"
      :icon="IconX"
      size="md"
      data-testid="applet-page-close"
      @click="emit('close')"
    />
  </div>
</template>
