<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { useOpenModalCount } from '@hive/ui/composables/useOpenModalCount'
import { appletTabId, isAppletTabId } from '../../lib/appletTabs'
import type { GlobalTabId } from '../../lib/appTabs'
import { useApplets } from '../../stores/useApplets'
import AppletFrame from './AppletFrame.vue'

// The applets open as title-bar tabs. A frame mounts the first time its tab comes forward
// and is only hidden after that, because unmounting an iframe throws its page away. A tab
// restored from the last launch does not run until the user opens it.
const props = defineProps<{
  tabs: readonly GlobalTabId[]
  /** The applet whose tab is in front, or empty when another tab is. */
  activeId: string
  /** Nothing covers the frame, so the page in front is on screen. */
  onScreen: boolean
}>()
const emit = defineEmits<{ close: [id: GlobalTabId] }>()

const applets = useApplets()
const openModals = useOpenModalCount()
const started = ref(new Set<string>())

watch(
  () => props.activeId,
  (id) => {
    if (id && !started.value.has(id)) started.value = new Set([...started.value, id])
  },
  { immediate: true },
)

const pages = computed(() =>
  props.tabs
    .filter(isAppletTabId)
    .map((tab) => tab.slice('applet:'.length))
    .filter((id) => started.value.has(id))
    .map((id) => ({ id, applet: applets.byID(id) })),
)

function opens(id: string): boolean {
  const applet = applets.byID(id)
  return !!applet?.runnable && applet.enabled && applet.surfaces.page
}
</script>

<template>
  <div
    v-for="page in pages"
    v-show="page.id === activeId"
    :key="page.id"
    class="flex min-h-0 min-w-0 flex-1 flex-col bg-app"
    :data-testid="`applet-page-tab-${page.id}`"
  >
    <AppletFrame
      v-if="page.applet && opens(page.id)"
      :applet="page.applet"
      session-slug=""
      surface="page"
      :wants-focus="onScreen && page.id === activeId && openModals === 0 && !applets.open.value"
      :on-screen="onScreen && page.id === activeId && !applets.open.value"
      @close="emit('close', appletTabId(page.id))"
    />
    <div v-else class="flex flex-1 items-center justify-center p-6" data-testid="applet-page-tab-unavailable">
      <InlineError v-if="page.applet && !page.applet.runnable" :message="page.applet.error" />
      <p v-else-if="page.applet && !page.applet.enabled" class="text-small text-text-3">
        {{ page.applet.title }} is turned off.
      </p>
      <p v-else-if="page.applet" class="text-small text-text-3">{{ page.applet.title }} no longer opens as a page.</p>
      <p v-else-if="applets.loaded.value" class="text-small text-text-3">
        There is no applet named <span class="font-mono">{{ page.id }}</span> any more.
      </p>
    </div>
  </div>
</template>
