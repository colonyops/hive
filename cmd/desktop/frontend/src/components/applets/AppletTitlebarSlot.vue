<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Applet } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { useApplets } from '../../stores/useApplets'
import AppletFrame from './AppletFrame.vue'

const props = defineProps<{ applet: Applet }>()

const applets = useApplets()
const requestedWidth = ref<number | null>(null)
const width = computed(() => Math.max(0, requestedWidth.value ?? props.applet.titlebar?.width ?? 0))

// The bar has nothing of its own to close, so hive.close() and Escape in its page mean
// the dropdown it opened.
function closeOwnDropdown(): void {
  if (applets.dropdown.value?.applet.id === props.applet.id) applets.closeDropdown()
}

function onResize(size: { width?: number }): void {
  if (Number.isFinite(size.width)) requestedWidth.value = size.width as number
}
</script>

<template>
  <!-- Width zero keeps the page running with nothing to show, and the window's edges
       still win over what a page asks for. -->
  <div
    class="flex h-7 max-w-[40vw] shrink-0 overflow-hidden rounded-lg"
    :style="{ width: `${width}px` }"
    style="--wails-draggable: no-drag"
    :data-testid="`titlebar-applet-${applet.id}`"
  >
    <AppletFrame
      :applet="applet"
      :session-slug="applets.liveSessionSlug.value"
      :entry="applet.titlebar?.entry"
      surface="titlebar"
      @close="closeOwnDropdown"
      @resize="onResize"
    />
  </div>
</template>
