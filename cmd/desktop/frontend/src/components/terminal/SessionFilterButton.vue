<script setup lang="ts">
import { computed, proxyRefs, ref, shallowRef } from 'vue'
import IconEyeOff from '~icons/lucide/eye-off'
import IconListFilter from '~icons/lucide/list-filter'
import IconSave from '~icons/lucide/save'
import IconUndo from '~icons/lucide/undo-2'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import type { MenuEntry } from '@hive/ui/types/menu'
import { useCodeViews } from '../../stores/useCodeViews'
import { useTerminalModeContext } from './terminalModeContext'

const { view, viewMenus } = useTerminalModeContext()
const views = proxyRefs(useCodeViews())

const open = ref(false)
const wrap = shallowRef<HTMLElement | null>(null)

const viewSection = computed<MenuEntry[]>(() => {
  const hidden = view.viewHiddenCount
  const entries: MenuEntry[] = []
  if (hidden) {
    entries.push({
      kind: 'action',
      id: 'noop',
      label: `${hidden} ${hidden === 1 ? 'session' : 'sessions'} hidden`,
      icon: IconEyeOff,
      disabled: true,
      testid: 'code-view-hidden',
    })
  }
  if (views.dirty) {
    entries.push(
      views.activeIsBuiltIn
        ? {
            kind: 'action',
            id: 'view-save-new',
            label: 'Save as new view…',
            icon: IconSave,
            testid: 'code-view-save-new',
          }
        : {
            kind: 'action',
            id: 'view-save',
            label: `Save changes to ${views.activeView?.name}`,
            icon: IconSave,
            testid: 'code-view-save',
          },
      { kind: 'action', id: 'view-reset', label: 'Reset to saved view', icon: IconUndo, testid: 'code-view-reset' },
    )
  }
  return entries.length ? [{ kind: 'separator' }, { kind: 'label', text: 'This view' }, ...entries] : []
})

const entries = computed<MenuEntry[]>(() => [...viewMenus.filterEntries, ...viewSection.value])

function onSelect(id: string): void {
  open.value = false
  if (id === 'view-save') views.save()
  else if (id === 'view-save-new') viewMenus.saving = true
  else if (id === 'view-reset') views.reset()
  else viewMenus.onFilterSelect(id)
}
</script>

<template>
  <div ref="wrap" class="relative flex">
    <!-- The bar no longer carries Save, so this dot is what says there is something to save. -->
    <span
      v-if="views.dirty"
      class="pointer-events-none absolute top-1 right-1 size-1.5 rounded-full bg-accent"
      data-testid="code-view-unsaved"
    />
    <IconButton
      label="Filter sessions by…"
      :icon="IconListFilter"
      :active="open"
      data-testid="code-view-add-filter"
      aria-haspopup="menu"
      :aria-expanded="open"
      @click="open = !open"
    />
    <!-- Start-aligned: the sidebar sits at the window's left edge, so an end-aligned panel would leave the screen. -->
    <AppMenu
      v-if="open"
      align="start"
      :entries="entries"
      :ignore="[wrap]"
      width="12rem"
      testid="code-view-add-menu"
      @close="open = false"
      @select="onSelect"
    />
  </div>
</template>
