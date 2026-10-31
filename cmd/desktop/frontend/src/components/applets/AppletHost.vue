<script setup lang="ts">
import { computed } from 'vue'
import BaseModal from '@hive/ui/components/ui/BaseModal.vue'
import CopyButton from '@hive/ui/components/ui/CopyButton.vue'
import HubOverlay from '@hive/ui/components/ui/HubOverlay.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { launcherIconComponent } from '../../lib/launcherIcons'
import { useApplets } from '../../stores/useApplets'
import AppletFrame from './AppletFrame.vue'
import AppletView from './AppletView.vue'

const applets = useApplets()
const applet = applets.open
const icon = computed(() => launcherIconComponent(applet.value?.icon))

// A panel can ask to be any size, and the window's edges still win.
const pageSize = computed(() => ({
  ...(applet.value?.width ? { width: `min(${applet.value.width}px, 96vw)` } : {}),
  ...(applet.value?.height ? { height: `min(${applet.value.height}px, 88vh)` } : {}),
}))

const PICKER_WIDTH = 720
</script>

<template>
  <template v-if="applet">
    <BaseModal
      v-if="!applet.runnable"
      :title="`${applet.title} is not valid`"
      tone="danger"
      :width="PICKER_WIDTH"
      testid="applet-invalid"
      @close="applets.close"
    >
      <div class="space-y-3 p-5">
        <InlineError :message="applet.error" testid="applet-invalid-error" />
        <div class="flex items-center gap-2 text-meta text-text-3">
          <span class="min-w-0 truncate font-mono">{{ applet.path }}</span>
          <CopyButton :text="applet.path" size="xs" />
        </div>
      </div>
    </BaseModal>
    <BaseModal
      v-else-if="applet.view === 'picker'"
      :key="applet.id"
      :title="applet.title"
      :icon="icon"
      :width="PICKER_WIDTH"
      testid="applet-picker"
      @close="applets.close"
    >
      <AppletView :applet="applet" :session-slug="applets.sessionSlug.value" @close="applets.close" />
    </BaseModal>
    <HubOverlay
      v-else-if="applet.view === 'html'"
      :key="`page-${applet.id}`"
      :label="applet.title"
      :panel-style="pageSize"
      testid="applet-page-overlay"
      @close="applets.close"
    >
      <AppletFrame :applet="applet" :session-slug="applets.sessionSlug.value" @close="applets.close" />
    </HubOverlay>
    <HubOverlay v-else :key="applet.id" :label="applet.title" testid="applet-overlay" @close="applets.close">
      <AppletView :applet="applet" :session-slug="applets.sessionSlug.value" @close="applets.close" />
    </HubOverlay>
  </template>
</template>
