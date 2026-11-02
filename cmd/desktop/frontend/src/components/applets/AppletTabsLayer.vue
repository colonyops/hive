<script setup lang="ts">
import { computed } from 'vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { useOpenModalCount } from '@hive/ui/composables/useOpenModalCount'
import { appletTabsOf } from '../../lib/appletTabs'
import { panelById, type PanelLayout } from '../../lib/panelLayout'
import { useApplets } from '../../stores/useApplets'
import AppletFrame from './AppletFrame.vue'

// An applet tab sits in the panel host's floating layer, because reparenting an iframe
// reloads its page. A tab that moves or a panel that splits must not cost the page its state.
const props = defineProps<{
  layout: PanelLayout
  region: (panelId: string) => Record<string, string>
  sessionSlug: string
  /** Re-keys the frames, so a session's tabs run only while it is on screen. */
  contentKey: string
  /** The user is in this area, so a tab in front is on screen. */
  active: boolean
  /** The host holds the keyboard, so the tab in front of the focused panel can have it. */
  keysActive: boolean
}>()
const emit = defineEmits<{ focusPanel: [panelId: string]; closeTab: [panelId: string, tabId: string] }>()

const applets = useApplets()
const openModals = useOpenModalCount()

const tabs = computed(() =>
  appletTabsOf(props.layout).map(({ panel, tab, applet }) => {
    const owner = panelById(props.layout, panel)
    const shown = !!owner && !owner.picking && owner.activeTab === tab.id
    return {
      panel,
      id: tab.id,
      shown,
      focused: shown && props.layout.focusedPanel === panel,
      appletId: applet,
      applet: applets.byID(applet),
    }
  }),
)
</script>

<template>
  <div
    v-for="tab in tabs"
    v-show="tab.shown"
    :key="`${contentKey}:${tab.id}`"
    class="absolute flex min-h-0 min-w-0 flex-col bg-app"
    :style="region(tab.panel)"
    :data-panel="tab.panel"
    data-testid="panel-view-applet"
    @pointerdown.capture="emit('focusPanel', tab.panel)"
  >
    <AppletFrame
      v-if="tab.applet?.runnable && tab.applet.enabled && tab.applet.surfaces.tab"
      :applet="tab.applet"
      :session-slug="sessionSlug"
      :wants-focus="active && keysActive && openModals === 0 && !applets.open.value && tab.focused"
      :on-screen="active && tab.shown && !applets.open.value"
      surface="tab"
      @close="emit('closeTab', tab.panel, tab.id)"
      @focused="emit('focusPanel', tab.panel)"
    />
    <div v-else class="flex flex-1 items-center justify-center p-6" data-testid="applet-tab-unavailable">
      <InlineError v-if="tab.applet && !tab.applet.runnable" :message="tab.applet.error" />
      <p v-else-if="tab.applet && !tab.applet.enabled" class="text-small text-text-3">
        {{ tab.applet.title }} is turned off.
      </p>
      <p v-else-if="tab.applet" class="text-small text-text-3">{{ tab.applet.title }} no longer opens as a tab.</p>
      <p v-else-if="applets.loaded.value" class="text-small text-text-3">
        There is no applet named <span class="font-mono">{{ tab.appletId }}</span> any more.
      </p>
    </div>
  </div>
</template>
