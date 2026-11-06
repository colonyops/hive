<script setup lang="ts">
// The title bar's tab strip. Inbox, Code and Chats are pinned at the left and
// never close; the global tabs after them are pages the user added from the +
// menu (Drive, Tasks, Settings, an applet), which close, reorder by drag, and
// persist. The selected tab takes the content's own `app` background with an
// outline, so it reads as the tab the frame below belongs to and stands apart
// from the `chip` hover fill. It stays neutral: amber is for what asks for
// attention.
import { useElementSize } from '@vueuse/core'
import { computed, ref, watch, type Component } from 'vue'
import IconCode from '~icons/lucide/code'
import IconFolderTree from '~icons/lucide/folder-tree'
import IconInbox from '~icons/lucide/inbox'
import IconListTodo from '~icons/lucide/list-todo'
import IconMessagesSquare from '~icons/lucide/messages-square'
import IconPlus from '~icons/lucide/plus'
import IconSettings from '~icons/lucide/settings'
import IconSettings2 from '~icons/lucide/settings-2'
import IconX from '~icons/lucide/x'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import type { MenuEntry } from '@hive/ui/types/menu'
import { formatCombo, useKeybindings } from '../composables/useKeybindings'
import { appletTabId, isAppletTabId } from '../lib/appletTabs'
import {
  VIEW_TABS,
  tabStripDensity,
  type AppTabId,
  type GlobalTabId,
  type PinnedTabId,
  type ViewTabId,
} from '../lib/appTabs'
import { launcherIconComponent } from '../lib/launcherIcons'
import { useApplets } from '../stores/useApplets'

const props = defineProps<{
  activeTab: AppTabId | null
  tabs: readonly GlobalTabId[]
}>()
const emit = defineEmits<{
  activate: [id: AppTabId]
  close: [id: GlobalTabId]
  move: [id: GlobalTabId, before: GlobalTabId | null]
  'manage-applets': []
}>()

const applets = useApplets()
const { combosFor } = useKeybindings()

interface TabView {
  id: AppTabId
  label: string
  icon: Component
  testid: string
  kbd?: string
}

// The pinned testids keep the mode switch's names, which the e2e suite and
// App.spec drive the areas by.
const PINNED: (TabView & { id: PinnedTabId; command: string })[] = [
  { id: 'inbox', label: 'Inbox', icon: IconInbox, testid: 'titlebar-mode-hub', command: 'view.go-inbox' },
  { id: 'code', label: 'Code', icon: IconCode, testid: 'titlebar-mode-terminal', command: 'view.go-code' },
  { id: 'chats', label: 'Chats', icon: IconMessagesSquare, testid: 'titlebar-mode-agents', command: 'view.go-chats' },
]

const VIEWS: Record<ViewTabId, { label: string; icon: Component; command: string }> = {
  drive: { label: 'Drive', icon: IconFolderTree, command: 'canvas.toggle' },
  tasks: { label: 'Tasks', icon: IconListTodo, command: 'tasks.toggle' },
  settings: { label: 'Settings', icon: IconSettings, command: 'settings.open' },
}

function kbdFor(commandId: string): string | undefined {
  const combo = combosFor(commandId)[0]
  return combo ? formatCombo(combo) : undefined
}

const pinned = computed<TabView[]>(() => PINNED.map((tab) => ({ ...tab, kbd: kbdFor(tab.command) })))

function globalView(id: GlobalTabId): TabView {
  if (isAppletTabId(id)) {
    const appletId = id.slice('applet:'.length)
    const applet = applets.byID(appletId)
    return {
      id,
      label: applet?.title ?? appletId,
      icon: launcherIconComponent(applet?.icon),
      testid: `titlebar-tab-applet-${appletId}`,
    }
  }
  const view = VIEWS[id]
  return { id, label: view.label, icon: view.icon, testid: `titlebar-tab-${id}`, kbd: kbdFor(view.command) }
}

const globals = computed(() => props.tabs.map(globalView))

function tooltip(tab: TabView): string {
  return tab.kbd ? `${tab.label} (${tab.kbd})` : tab.label
}

// ── Add menu ────────────────────────────────────────────────────────────
const addWrap = ref<HTMLElement | null>(null)
const addOpen = ref(false)
const MANAGE_APPLETS = 'manage-applets'

const addEntries = computed<MenuEntry[]>(() => {
  const open = new Set(props.tabs)
  const pages: MenuEntry[] = VIEW_TABS.map((id) => ({
    kind: 'action',
    id,
    label: VIEWS[id].label,
    icon: VIEWS[id].icon,
    kbd: kbdFor(VIEWS[id].command),
    checked: open.has(id),
    testid: `titlebar-add-tab-${id}`,
  }))
  const appletEntries: MenuEntry[] = applets.globalTabApplets.value.map((applet) => ({
    kind: 'action',
    id: appletTabId(applet.id),
    label: applet.title,
    icon: launcherIconComponent(applet.icon),
    checked: open.has(appletTabId(applet.id)),
    testid: `titlebar-add-tab-applet-${applet.id}`,
  }))
  return [
    { kind: 'label', text: 'Pages' },
    ...pages,
    { kind: 'separator' },
    { kind: 'label', text: 'Applets' },
    ...(appletEntries.length
      ? appletEntries
      : [
          {
            kind: 'action' as const,
            id: 'no-applets',
            label: 'No applets yet',
            hint: 'Ask an agent to build one.',
            disabled: true,
          },
        ]),
    { kind: 'separator' },
    {
      kind: 'action',
      id: MANAGE_APPLETS,
      label: 'Manage applets…',
      icon: IconSettings2,
      testid: 'titlebar-add-tab-manage-applets',
    },
  ]
})

function selectAdd(id: string): void {
  addOpen.value = false
  if (id === MANAGE_APPLETS) emit('manage-applets')
  else emit('activate', id as GlobalTabId)
}

// ── Reorder ─────────────────────────────────────────────────────────────
const dragging = ref<GlobalTabId | null>(null)
const dropBefore = ref<GlobalTabId | null | undefined>(undefined)
const TAB_MIME = 'application/x-hive-titlebar-tab'

function onDragStart(event: DragEvent, id: GlobalTabId): void {
  dragging.value = id
  event.dataTransfer?.setData(TAB_MIME, id)
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
}

function onDragOver(event: DragEvent, index: number): void {
  if (!dragging.value) return
  event.preventDefault()
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const after = event.clientX > rect.left + rect.width / 2
  dropBefore.value = after ? (props.tabs[index + 1] ?? null) : props.tabs[index]
}

function onDrop(): void {
  if (dragging.value && dropBefore.value !== undefined) emit('move', dragging.value, dropBefore.value)
  onDragEnd()
}

function onDragEnd(): void {
  dragging.value = null
  dropBefore.value = undefined
}

function dropMarker(index: number): string {
  if (!dragging.value || dropBefore.value === undefined) return ''
  if (dropBefore.value === props.tabs[index])
    return 'before:absolute before:-left-0.5 before:inset-y-1 before:w-0.5 before:rounded-full before:bg-accent'
  if (dropBefore.value === null && index === props.tabs.length - 1)
    return 'after:absolute after:-right-0.5 after:inset-y-1 after:w-0.5 after:rounded-full after:bg-accent'
  return ''
}

// A middle click closes a tab, as it does in a browser.
function onAuxClick(event: MouseEvent, id: GlobalTabId): void {
  if (event.button !== 1) return
  event.preventDefault()
  emit('close', id)
}

// A border, not a ring: a ring draws outside the box, and the global tabs'
// scroller clips it to the sides. Unselected tabs keep a transparent one so
// selecting never shifts the strip.
const SELECTED = 'border-strong bg-app text-text shadow-sm'
const UNSELECTED = 'border-transparent text-text-3 hover:bg-chip hover:text-text-2'

const strip = ref<HTMLElement | null>(null)
const pinnedGroup = ref<HTMLElement | null>(null)
const { width: stripWidth } = useElementSize(strip)
const { width: pinnedWidth } = useElementSize(pinnedGroup)
// Measured only while the labels show, so dropping them cannot feed back into
// the decision to drop them.
const pinnedLabeledWidth = ref(230)

const density = computed(() =>
  tabStripDensity({
    width: stripWidth.value,
    pinnedLabeledWidth: pinnedLabeledWidth.value,
    globalCount: props.tabs.length,
    globalSelected: !!props.activeTab && props.tabs.includes(props.activeTab as GlobalTabId),
  }),
)

watch([pinnedWidth, () => density.value.pinnedLabels], ([width, labeled]) => {
  if (labeled && width > 0) pinnedLabeledWidth.value = width
})

const tabClass = (selected: boolean) => [
  'flex h-7 shrink-0 cursor-pointer items-center gap-1.5 rounded-lg border text-small font-medium transition-colors',
  density.value.pinnedLabels ? 'px-2.5' : 'px-2',
  selected ? SELECTED : UNSELECTED,
]
</script>

<template>
  <!-- The strip fills the cluster so it can measure the room it has; its empty
       end stays a drag surface for the window. -->
  <div
    ref="strip"
    class="flex min-w-0 flex-1 items-center gap-1"
    role="tablist"
    aria-label="Tabs"
    data-testid="titlebar-tabs"
    :data-density="density.globalLabels ? 'full' : density.pinnedLabels ? 'compact' : 'icons'"
  >
    <div
      ref="pinnedGroup"
      class="flex shrink-0 items-center gap-1"
      style="--wails-draggable: no-drag"
      data-testid="titlebar-mode"
    >
      <button
        v-for="tab in pinned"
        :key="tab.id"
        type="button"
        role="tab"
        :class="tabClass(activeTab === tab.id)"
        :aria-selected="activeTab === tab.id"
        :aria-pressed="activeTab === tab.id"
        :aria-label="tab.label"
        :title="tooltip(tab)"
        :data-testid="tab.testid"
        @click="emit('activate', tab.id)"
      >
        <component :is="tab.icon" class="size-3.5 shrink-0" />
        <span v-if="density.pinnedLabels">{{ tab.label }}</span>
      </button>
    </div>

    <span v-if="globals.length" class="mx-1 h-4 w-px shrink-0 bg-strong" aria-hidden="true" />

    <div
      class="-my-1 flex min-w-0 items-center gap-1 overflow-x-auto py-1 [scrollbar-width:none]"
      style="--wails-draggable: no-drag"
      data-testid="titlebar-global-tabs"
    >
      <div
        v-for="(tab, index) in globals"
        :key="tab.id"
        class="group relative flex h-7 items-center rounded-lg border"
        :class="[
          density.globalLabels ? 'min-w-[84px] max-w-44' : 'shrink-0',
          activeTab === tab.id ? SELECTED : UNSELECTED,
          dragging === tab.id ? 'opacity-50' : '',
          dropMarker(index),
        ]"
        draggable="true"
        :data-testid="`${tab.testid}-wrap`"
        @dragstart="onDragStart($event, tab.id as GlobalTabId)"
        @dragover="onDragOver($event, index)"
        @drop="onDrop"
        @dragend="onDragEnd"
        @auxclick="onAuxClick($event, tab.id as GlobalTabId)"
      >
        <button
          type="button"
          role="tab"
          class="flex h-full min-w-0 flex-1 cursor-pointer items-center gap-1.5 rounded-lg text-small font-medium"
          :class="[
            activeTab === tab.id ? 'text-text' : 'text-text-3 group-hover:text-text-2',
            density.globalLabels ? 'pl-2.5 pr-1' : activeTab === tab.id ? 'pl-2 pr-1' : 'px-2',
          ]"
          :aria-label="tab.label"
          :aria-selected="activeTab === tab.id"
          :title="tooltip(tab)"
          :data-testid="tab.testid"
          @click="emit('activate', tab.id)"
        >
          <component :is="tab.icon" class="size-3.5 shrink-0" />
          <span v-if="density.globalLabels" class="min-w-0 truncate">{{ tab.label }}</span>
        </button>
        <!-- As an icon a tab has no room to spare, so only the one in front keeps
             its close; a middle click still closes the others. -->
        <button
          v-if="density.globalLabels || activeTab === tab.id"
          type="button"
          class="mr-1 flex size-4.5 shrink-0 cursor-pointer items-center justify-center rounded text-text-4 hover:bg-chip hover:text-text"
          :class="activeTab === tab.id ? 'opacity-100' : 'opacity-0 group-hover:opacity-100 focus-visible:opacity-100'"
          :aria-label="`Close ${tab.label}`"
          :data-testid="`${tab.testid}-close`"
          @click.stop="emit('close', tab.id as GlobalTabId)"
        >
          <IconX class="size-3" />
        </button>
      </div>
    </div>

    <div ref="addWrap" class="relative ml-0.5 shrink-0" style="--wails-draggable: no-drag">
      <IconButton
        label="Add a tab"
        tooltip="Add a tab"
        :icon="IconPlus"
        size="lg"
        aria-haspopup="menu"
        :aria-expanded="addOpen"
        :active="addOpen"
        data-testid="titlebar-add-tab"
        @click="addOpen = !addOpen"
      />
      <AppMenu
        v-if="addOpen"
        :entries="addEntries"
        :ignore="[addWrap]"
        align="start"
        width="240px"
        testid="titlebar-add-tab-menu"
        @select="selectAdd"
        @close="addOpen = false"
      />
    </div>
  </div>
</template>
