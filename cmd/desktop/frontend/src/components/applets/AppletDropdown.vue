<script setup lang="ts">
import { onClickOutside, useEventListener, useWindowSize } from '@vueuse/core'
import { computed, ref, watch } from 'vue'
import { useApplets } from '../../stores/useApplets'
import AppletFrame from './AppletFrame.vue'

const GAP = 6
const EDGE = 8
const MIN_SIZE = 40

const applets = useApplets()
const dropdown = applets.dropdown
const panel = ref<HTMLElement | null>(null)
const requested = ref<{ width?: number; height?: number }>({})
const { width: windowWidth, height: windowHeight } = useWindowSize()

watch(dropdown, () => {
  requested.value = {}
})

const panelStyle = computed(() => {
  const current = dropdown.value
  if (!current) return {}
  const rect = current.anchor.getBoundingClientRect()
  // Below the bar rather than the page's frame, as the app's own menus sit.
  const top = (current.anchor.closest('header')?.getBoundingClientRect().bottom ?? rect.bottom) + GAP
  const width = Math.max(MIN_SIZE, Math.min(requested.value.width ?? current.width, windowWidth.value - EDGE * 2))
  const height = Math.max(MIN_SIZE, Math.min(requested.value.height ?? current.height, windowHeight.value - top - EDGE))
  return {
    top: `${top}px`,
    left: `${Math.max(EDGE, Math.min(rect.left, windowWidth.value - EDGE - width))}px`,
    width: `${width}px`,
    height: `${height}px`,
  }
})

function onResize(size: { width?: number; height?: number }): void {
  requested.value = {
    width: Number.isFinite(size.width) ? size.width : requested.value.width,
    height: Number.isFinite(size.height) ? size.height : requested.value.height,
  }
}

onClickOutside(panel, () => applets.closeDropdown(), {
  ignore: () => (dropdown.value ? [dropdown.value.anchor] : []),
})

// A click into a frame reaches the app only as the window losing focus. The page that
// opened the dropdown is left out, because its own click decides whether to toggle it.
useEventListener(
  () => (dropdown.value ? window : null),
  'blur',
  () => {
    setTimeout(() => {
      const active = document.activeElement
      if (active?.tagName !== 'IFRAME' || active === dropdown.value?.anchor || panel.value?.contains(active)) return
      applets.closeDropdown()
    })
  },
)
</script>

<template>
  <Teleport to="body">
    <!-- Under the overlays (z-40) and the palette, which open over a dropdown and not
         behind it. -->
    <div
      v-if="dropdown"
      ref="panel"
      class="fixed z-30 flex overflow-hidden rounded-lg border border-border bg-raised shadow-xl"
      :style="panelStyle"
      data-testid="applet-dropdown"
    >
      <AppletFrame
        :key="`${dropdown.applet.id}/${dropdown.entry}`"
        :applet="dropdown.applet"
        :session-slug="applets.liveSessionSlug.value"
        :entry="dropdown.entry"
        surface="dropdown"
        @close="applets.closeDropdown"
        @resize="onResize"
      />
    </div>
  </Teleport>
</template>
