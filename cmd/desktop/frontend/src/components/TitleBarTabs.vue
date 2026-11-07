<script setup lang="ts">
// The title bar's tab strip. Inbox, Code and Chats are pinned at the left and
// never close; the global tabs after them are pages the user added from the +
// menu (Drive, Tasks, Settings, an applet), which close, reorder by drag, and
// persist. The selected tab takes the content's own `app` background with an
// outline, so it reads as the tab the frame below belongs to and stands apart
// from the `chip` hover fill. It stays neutral: amber is for what asks for
// attention.
import { useElementSize, useEventListener } from '@vueuse/core'
import { computed, ref, shallowRef, watch, type Component } from 'vue'
import IconCode from '~icons/lucide/code'
import IconCopyX from '~icons/lucide/copy-x'
import IconFolderTree from '~icons/lucide/folder-tree'
import IconInbox from '~icons/lucide/inbox'
import IconListTodo from '~icons/lucide/list-todo'
import IconMessagesSquare from '~icons/lucide/messages-square'
import IconPlus from '~icons/lucide/plus'
import IconSettings from '~icons/lucide/settings'
import IconSettings2 from '~icons/lucide/settings-2'
import IconSquareTerminal from '~icons/lucide/square-terminal'
import IconX from '~icons/lucide/x'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import type { MenuEntry } from '@hive/ui/types/menu'
import { dropEdge, useDragReorder } from '../composables/useDragReorder'
import { formatCombo, useKeybindings } from '../composables/useKeybindings'
import { appletIdOfTabId, appletTabId, isAppletTabId } from '../lib/appletTabs'
import {
  VIEW_TABS,
  tabStripDensity,
  type AppTabId,
  type GlobalTabId,
  type PinnedTabId,
  type ViewTabId,
} from '../lib/appTabs'
import { launcherIconComponent } from '../lib/launcherIcons'
import { moveId } from '../lib/listOrder'
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
  runs: { label: 'Action runs', icon: IconSquareTerminal, command: 'action-runs.toggle' },
  settings: { label: 'Settings', icon: IconSettings, command: 'settings.open' },
}

function kbdFor(commandId: string): string | undefined {
  const combo = combosFor(commandId)[0]
  return combo ? formatCombo(combo) : undefined
}

const pinned = computed<TabView[]>(() => PINNED.map((tab) => ({ ...tab, kbd: kbdFor(tab.command) })))

function globalView(id: GlobalTabId): TabView {
  if (isAppletTabId(id)) {
    const appletId = appletIdOfTabId(id)
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

const globals = computed(() => (preview.value ?? props.tabs).map(globalView))

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
// The drag target is the whole previewed order, so the tabs slide into place
// while the pointer moves and the drop commits what the user already sees, as
// in ProfileRail. A drop that ends where the strip started emits nothing.
const drag = useDragReorder<GlobalTabId, GlobalTabId[]>({ mime: 'application/x-hive-titlebar-tab', onDrop: commit })
const { dragging, target: preview } = drag

function commit(id: GlobalTabId, order: GlobalTabId[]): void {
  if (order.every((tab, at) => tab === props.tabs[at])) return
  emit('move', id, order[order.indexOf(id) + 1] ?? null)
}

function overTab(event: DragEvent, id: GlobalTabId): void {
  if (dragging.value === null) return
  event.preventDefault()
  const next = moveId([...(preview.value ?? props.tabs)], dragging.value, { id, edge: dropEdge(event, 'x') })
  drag.over(event, (next as GlobalTabId[] | null) ?? preview.value)
}

// ── Context menu ────────────────────────────────────────────────────────
const tabMenu = ref<GlobalTabId | null>(null)
const tabMenuAnchor = shallowRef<HTMLElement | null>(null)

function openTabMenu(event: MouseEvent, id: GlobalTabId): void {
  tabMenuAnchor.value = event.currentTarget as HTMLElement
  tabMenu.value = id
}

// A right-click is not a click, so AppMenu's click-outside misses it and a
// second tab's menu would open beside this one. Capture runs first, so a
// right-click on a tab still reopens the menu through openTabMenu.
useEventListener(
  () => (tabMenu.value ? window : null),
  'contextmenu',
  () => {
    tabMenu.value = null
  },
  { capture: true },
)

const tabMenuEntries = computed<MenuEntry[]>(() => [
  { kind: 'action', id: 'close', label: 'Close tab', icon: IconX, testid: 'titlebar-tab-menu-close' },
  {
    kind: 'action',
    id: 'close-others',
    label: 'Close other tabs',
    icon: IconCopyX,
    disabled: props.tabs.length < 2,
    testid: 'titlebar-tab-menu-close-others',
  },
])

function selectTabMenu(action: string): void {
  const id = tabMenu.value
  tabMenu.value = null
  if (!id) return
  if (action === 'close') emit('close', id)
  else if (action === 'close-others') for (const other of props.tabs) if (other !== id) emit('close', other)
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
// the decision to drop them. The seed is the labeled Inbox, Code and Chats group,
// for the first layout before a measurement.
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

    <!-- The strip takes the drop, not the tabs: a release in the gap between two
         tabs still lands where the preview shows. -->
    <TransitionGroup
      name="titlebar-tab"
      tag="div"
      class="-my-1 flex min-w-0 items-center gap-1 overflow-x-auto py-1 [scrollbar-width:none]"
      style="--wails-draggable: no-drag"
      data-testid="titlebar-global-tabs"
      @dragover="dragging !== null && $event.preventDefault()"
      @drop.prevent="drag.drop"
    >
      <div
        v-for="tab in globals"
        :key="tab.id"
        class="group relative flex h-7 items-center rounded-lg border"
        :class="[
          density.globalLabels ? 'min-w-[84px] max-w-44' : 'shrink-0',
          activeTab === tab.id ? SELECTED : UNSELECTED,
          dragging === tab.id ? 'opacity-40' : '',
        ]"
        draggable="true"
        :data-testid="`${tab.testid}-wrap`"
        @dragstart="drag.start($event, tab.id as GlobalTabId)"
        @dragover="overTab($event, tab.id as GlobalTabId)"
        @dragend="drag.end"
        @auxclick="onAuxClick($event, tab.id as GlobalTabId)"
        @contextmenu.prevent="openTabMenu($event, tab.id as GlobalTabId)"
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
    </TransitionGroup>

    <AppMenu
      v-if="tabMenu"
      :entries="tabMenuEntries"
      :anchor="tabMenuAnchor"
      :ignore="[tabMenuAnchor]"
      align="start"
      width="200px"
      testid="titlebar-tab-menu"
      @select="selectTabMenu"
      @close="tabMenu = null"
    />

    <div ref="addWrap" class="relative shrink-0" style="--wails-draggable: no-drag">
      <IconButton
        label="Add a page"
        tooltip="Add a page"
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

<style scoped>
/* ProfileRail's timing. A closing tab drops out of flow at once so its
   neighbours glide over; absolute would jump it to the start of the flex row. */
.titlebar-tab-move {
  transition: transform 0.18s cubic-bezier(0.2, 0, 0, 1);
}
.titlebar-tab-leave-active {
  display: none;
}

@media (prefers-reduced-motion: reduce) {
  .titlebar-tab-move {
    transition: none;
  }
}
</style>
