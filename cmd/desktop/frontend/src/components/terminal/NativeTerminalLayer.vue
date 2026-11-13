<script setup lang="ts">
import { computed } from 'vue'
import { nativeTerminalTabsOf } from '../../lib/nativeTerminalTabs'
import { panelById, type PanelLayout } from '../../lib/panelLayout'
import NativeTerminalSurface from './NativeTerminalSurface.vue'

// Native terminal tabs sit in the panel host's floating layer, like applets:
// a tab that moves between panels keeps its surface instead of reopening it.
const props = defineProps<{
  layout: PanelLayout
  region: (panelId: string) => Record<string, string>
  /** Re-keys the surfaces, so a session's tabs exist only while it is on screen. */
  contentKey: string
  /** The user is in this area, so a tab in front is on screen. */
  active: boolean
  /** The host holds the keyboard, so the tab in front of the focused panel can have it. */
  keysActive: boolean
  overlayOpen: boolean
}>()
const emit = defineEmits<{ focusPanel: [panelId: string]; closeTab: [panelId: string, tabId: string] }>()

const tabs = computed(() =>
  nativeTerminalTabsOf(props.layout).map(({ panel, tab, address }) => {
    const owner = panelById(props.layout, panel)
    const shown = !!owner && !owner.picking && owner.activeTab === tab.id
    return { panel, id: tab.id, address, shown, focused: shown && props.layout.focusedPanel === panel }
  }),
)
</script>

<template>
  <div
    v-for="tab in tabs"
    v-show="tab.shown"
    :key="`${contentKey}:${tab.id}`"
    class="absolute flex min-h-0 min-w-0 flex-col"
    :style="region(tab.panel)"
    :data-panel="tab.panel"
    data-testid="panel-view-native-terminal"
    @pointerdown.capture="emit('focusPanel', tab.panel)"
  >
    <NativeTerminalSurface
      :id="`${contentKey}:${tab.id}`"
      :target="{ address: tab.address }"
      :on-screen="active && tab.shown"
      :wants-focus="active && keysActive && tab.focused"
      :overlay-open="overlayOpen"
      @focused="emit('focusPanel', tab.panel)"
      @exited="emit('closeTab', tab.panel, tab.id)"
    />
  </div>
</template>
