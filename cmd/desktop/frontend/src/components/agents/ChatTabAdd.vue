<script setup lang="ts">
import { computed, ref } from 'vue'
import IconPlus from '~icons/lucide/plus'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import type { MenuEntry } from '@hive/ui/types/menu'
import { sessionView, type SessionViewKind } from '../terminal/panelViews'

const props = defineProps<{
  panel: string
  views: readonly SessionViewKind[]
}>()
const emit = defineEmits<{ view: [kind: SessionViewKind] }>()

const menuOpen = ref(false)
const addButton = ref<HTMLElement | null>(null)

const entries = computed<MenuEntry[]>(() =>
  props.views.map((kind) => {
    const { label, icon } = sessionView(kind)
    return { kind: 'action', id: kind, label, icon, testid: `chat-tab-add-${kind}` }
  }),
)

function select(id: string): void {
  menuOpen.value = false
  emit('view', id as SessionViewKind)
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
