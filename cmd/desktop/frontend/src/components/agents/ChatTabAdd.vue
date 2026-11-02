<script setup lang="ts">
import { computed, ref } from 'vue'
import IconPlus from '~icons/lucide/plus'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import type { MenuEntry } from '@hive/ui/types/menu'
import { useApplets, type AppletMenuRow } from '../../stores/useApplets'
import { appletMenuChoice, appletMenuEntries } from '../applets/appletMenu'
import { sessionView, type SessionViewKind } from '../terminal/panelViews'

const props = defineProps<{
  panel: string
  views: readonly SessionViewKind[]
}>()
const emit = defineEmits<{ view: [kind: SessionViewKind]; applet: [id: string, opensAs: AppletMenuRow['opensAs']] }>()

const applets = useApplets()

const menuOpen = ref(false)
const addButton = ref<HTMLElement | null>(null)

const entries = computed<MenuEntry[]>(() => [
  ...props.views.map((kind): MenuEntry => {
    const { label, icon } = sessionView(kind)
    return { kind: 'action', id: kind, label, icon, testid: `chat-tab-add-${kind}` }
  }),
  ...(applets.sessionMenu.value.length ? [{ kind: 'separator' as const }] : []),
  ...appletMenuEntries(applets.sessionMenu.value),
])

function select(id: string): void {
  menuOpen.value = false
  const choice = appletMenuChoice(id)
  if (choice) emit('applet', choice.applet, choice.opensAs)
  else emit('view', id as SessionViewKind)
}
</script>

<template>
  <div ref="addButton" class="relative flex items-center">
    <IconButton
      label="Open a tab"
      :icon="IconPlus"
      size="sm"
      :active="menuOpen"
      :data-testid="`panel-tab-add-${panel}`"
      @click="menuOpen = !menuOpen"
    />
    <AppMenu
      v-if="menuOpen"
      :entries="entries"
      :ignore="[addButton]"
      :anchor="addButton"
      align="start"
      width="min(200px, 100%)"
      testid="chat-tab-add-menu"
      @select="select"
      @close="menuOpen = false"
    />
  </div>
</template>
