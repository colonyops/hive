<script setup lang="ts">
import { computed, nextTick, proxyRefs, ref, toRef, watch } from 'vue'
import { useNow } from '@vueuse/core'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconChevronsDownUp from '~icons/lucide/chevrons-down-up'
import IconChevronsUpDown from '~icons/lucide/chevrons-up-down'
import IconEllipsisVertical from '~icons/lucide/ellipsis-vertical'
import IconListFilter from '~icons/lucide/list-filter'
import IconMessagesSquare from '~icons/lucide/messages-square'
import IconPinOff from '~icons/lucide/pin-off'
import IconPlus from '~icons/lucide/plus'
import IconTrash from '~icons/lucide/trash-2'
import IconX from '~icons/lucide/x'
import IconButton from '../ui/IconButton.vue'
import AppMenu from '../ui/AppMenu.vue'
import BaseBadge from '../ui/BaseBadge.vue'
import EmptyState from '../ui/EmptyState.vue'
import InlineError from '../ui/InlineError.vue'
import Kbd from '../ui/Kbd.vue'
import PanelResizeHandle from '../ui/PanelResizeHandle.vue'
import TreeRail from '../ui/TreeRail.vue'
import NewWindowMenu from '../NewWindowMenu.vue'
import SessionRowMenu from '../SessionRowMenu.vue'
import SidebarToolbar from '../sidebar/SidebarToolbar.vue'
import { dropClass, dropEdge, useDragReorder } from '../../composables/useDragReorder'
import { formatCombo, useKeybindings } from '../../composables/useKeybindings'
import { useNewSession } from '../../composables/useNewSession'
import { useResizablePanel } from '../../composables/useResizablePanel'
import { useSelectionRail } from '../../composables/useSelectionRail'
import { moveId, type OrderDropTarget } from '../../lib/listOrder'
import { terminalTreeFocused } from '../../lib/terminalTree'
import { sessionAgeDays, useTerminalSessionAge } from '../../stores/useTerminalSessionAge'
import type { TerminalSessionRow } from '../../stores/useTerminalSessions'
import type { MenuEntry } from '../../types/menu'
import { useTerminalModeContext } from './terminalModeContext'
import { useTreeRowMenus, windowMenuKey } from './useTreeRowMenus'

const { pool, tree, view, nav, attach, ops, rename, sessions } = useTerminalModeContext()
const { openBlank: openNewSession } = useNewSession()
const menus = proxyRefs(useTreeRowMenus({ windowMenuAllowed: ops.rowHasWindowActions }))
const sidebarEl = toRef(nav, 'root')
const filterInput = toRef(nav, 'filterInput')
const { enabled: showSessionAge, thresholdDays: sessionAgeThresholdDays } = useTerminalSessionAge()
const now = useNow({ interval: 60_000 })
const sessionAgeBadges = computed<Record<string, { days: number; title: string }>>(() => {
  if (!showSessionAge.value) return {}
  const badges: Record<string, { days: number; title: string }> = {}
  for (const row of tree.activeSessions) {
    const days = sessionAgeDays(row.createdAt, sessionAgeThresholdDays.value, now.value.getTime())
    if (days === null || !row.createdAt) continue
    badges[row.id] = {
      days,
      title: `Started ${new Date(row.createdAt).toLocaleString()}; ${days} ${days === 1 ? 'day' : 'days'} old`,
    }
  }
  return badges
})

const sidebarMenuEntries = computed<MenuEntry[]>(() => [
  {
    kind: 'action',
    id: 'running-only',
    label: 'Only running sessions',
    checked: view.runningOnly,
    testid: 'terminal-sessions-running-only',
  },
  { kind: 'separator' },
  {
    kind: 'action',
    id: 'collapse-all',
    label: 'Collapse all',
    icon: IconChevronsDownUp,
    testid: 'terminal-sessions-collapse-all',
  },
  {
    kind: 'action',
    id: 'expand-all',
    label: 'Expand all',
    icon: IconChevronsUpDown,
    testid: 'terminal-sessions-expand-all',
  },
  { kind: 'separator' },
  {
    kind: 'action',
    id: 'prune',
    label: tree.prunableCount ? `Prune ${tree.prunableCount} recycled…` : 'Nothing to prune',
    icon: IconTrash,
    testid: 'terminal-sessions-prune',
  },
])

function onSidebarMenuSelect(id: string): void {
  if (id === 'running-only') view.runningOnly = !view.runningOnly
  else if (id === 'collapse-all' || id === 'expand-all') view.expansion.setAll(id === 'expand-all')
  else if (id === 'prune' && tree.prunableCount) sessions.requestPrune(tree.prunableCount)
}

// A chat's own lifecycle stays in the Chats area, which owns its record; this
// menu offers the two things only the pin created. Unpinning drops the row from
// `attachable`, which is what detaches it; the agent keeps running (ADR
// agent-workspace-sessions-are-tmux-sessions).
const chatMenuEntries: MenuEntry[] = [
  {
    kind: 'action',
    id: 'open-in-agents',
    label: 'Open in Chats',
    icon: IconMessagesSquare,
    testid: 'terminal-chat-open-in-agents',
  },
  { kind: 'separator' },
  { kind: 'action', id: 'unpin', label: 'Unpin from Code', icon: IconPinOff, testid: 'terminal-chat-unpin' },
]

function onChatMenuSelect(row: TerminalSessionRow, id: string): void {
  menus.openRow = ''
  if (id === 'unpin') tree.unpinSlug(row.slug)
  else if (id === 'open-in-agents') ops.openChatInAgents(row)
}

function runWindowAction(row: TerminalSessionRow, windowId: string, entryID: string): void {
  menus.openWindow = ''
  ops.runWindowAction(row, windowId, entryID)
}

// The chord worth showing is the one that leaves where focus already is.
const { combosFor } = useKeybindings()
const focusHint = computed(() => {
  const inTree = terminalTreeFocused.value
  const combo = combosFor(inTree ? 'terminal.focus-pane' : 'terminal.focus-sidebar')[0]
  if (!combo) return null
  return { keys: formatCombo(combo), label: inTree ? 'terminal' : 'tree' }
})

const {
  size: sidebarWidth,
  startResize,
  step,
} = useResizablePanel({
  storageKey: 'hive.panel.terminal.sidebar',
  defaultSize: 250,
  min: 180,
  max: 400,
  edge: 'right',
})

// The selection markers are two elements that travel: one tracks the attached
// session, one the active window. Measured off the rows, because row heights
// differ by kind and a group's own height animates.
const treeContent = ref<HTMLElement | null>(null)
const treeSettled = ref(false)
const { rails, update: settleRails } = useSelectionRail(
  treeContent,
  [
    '[data-testid="terminal-session-row"][data-attached="true"]',
    '[data-testid="terminal-window-row"][data-active="true"]',
  ],
  { sources: [() => pool.activeSlug, () => pool.current?.activeWindowId.value], settle: () => treeSettled.value },
)

// Motion stays off until the tree has been through a frame: on the first paint
// every row is an enter, so the panel would animate in as one block and the
// rails would force layout through all of it.
watch(
  () => view.ready,
  (ready) => {
    if (!ready) return
    void nextTick(() =>
      requestAnimationFrame(() =>
        requestAnimationFrame(() => {
          treeSettled.value = true
          settleRails()
        }),
      ),
    )
  },
  { immediate: true },
)

// Expand/collapse animates the measured height; the hooks pin the start and end
// values and .tree-expand-* carries the transition. The animated element holds
// nothing but the panel, because a box cannot shrink below its own padding.
function settleHeight(el: Element): void {
  ;(el as HTMLElement).style.height = ''
}
const expandHooks = {
  onEnter(el: Element): void {
    const panel = el as HTMLElement
    panel.style.height = '0'
    panel.getBoundingClientRect() // commit the collapsed height before the target lands
    panel.style.height = `${panel.scrollHeight}px`
  },
  onAfterEnter: settleHeight,
  onEnterCancelled: settleHeight,
  onLeave(el: Element): void {
    const panel = el as HTMLElement
    panel.style.height = `${panel.scrollHeight}px`
    panel.getBoundingClientRect()
    panel.style.height = '0'
  },
}

// A window can only land back in the session it came from, and is movable only
// while that session holds a control client, which is exactly when the tree
// renders its live tabs.
const windowDrag = useDragReorder<{ slug: string; windowId: string }, OrderDropTarget & { windowId: string }>({
  mime: 'application/x-hive-terminal-window',
  payload: (dragged) => dragged.windowId,
  onDrop: (dragged, target) => {
    const session = pool.pool.get(dragged.slug)
    const order = moveId(session?.tabs.value.map((tab) => tab.windowId) ?? [], dragged.windowId, {
      id: target.windowId,
      edge: target.edge,
    })
    if (session && order) void session.moveWindow(dragged.windowId, order.indexOf(dragged.windowId))
  },
})
const { target: windowDropTarget } = windowDrag

// Both edges of the dragged window name the gap it already fills, so neither marks a move.
function onWindowDragOver(event: DragEvent, row: TerminalSessionRow, windowId: string): void {
  const dragged = windowDrag.dragging.value
  const movable = dragged?.slug === row.slug && dragged.windowId !== windowId
  windowDrag.over(event, movable ? { id: windowMenuKey(row, windowId), windowId, edge: dropEdge(event) } : null)
}

function draggingWindowRow(slug: string, windowId: string): boolean {
  const dragged = windowDrag.dragging.value
  return dragged?.slug === slug && dragged.windowId === windowId
}
</script>

<template>
  <aside
    ref="sidebarEl"
    class="relative flex shrink-0 flex-col border-r border-border bg-sidebar"
    :style="{ width: sidebarWidth + 'px' }"
    data-testid="terminal-session-sidebar"
    tabindex="-1"
    @keydown="nav.onKeydown"
    @focusin="terminalTreeFocused = true"
    @focusout="nav.onFocusOut"
  >
    <SidebarToolbar
      ref="filterInput"
      v-model="view.query"
      filter-label="Filter sessions"
      reload-label="Reload sessions"
      new-label="New session"
      menu-label="Session list actions"
      :menu-entries="sidebarMenuEntries"
      :loading="tree.sessionsLoading"
      testid="terminal-sessions"
      reload-testid="terminal-sessions-refresh"
      new-testid="terminal-new-session"
      @escape="nav.focusTree"
      @keydown.down.prevent="nav.focusTree"
      @keydown.enter.prevent="nav.focusTree"
      @reload="tree.reloadSessions"
      @new="openNewSession(tree.sessionRepository(pool.activeSlug))"
      @select="onSidebarMenuSelect"
    />
    <div
      v-if="view.runningNote"
      class="flex h-7 shrink-0 items-center gap-2 border-b border-border bg-chip px-3"
      data-testid="terminal-sessions-running-note"
    >
      <IconListFilter class="size-3 shrink-0 text-accent" />
      <span class="min-w-0 flex-1 truncate text-caption text-text-2">{{ view.runningNote }}</span>
      <button
        type="button"
        class="shrink-0 cursor-pointer text-caption text-text-3 hover:text-text"
        data-testid="terminal-sessions-running-clear"
        @click="view.runningOnly = false"
      >
        Show all
      </button>
    </div>
    <div class="hive-scroll min-h-0 flex-1 overflow-y-auto pb-4">
      <InlineError
        v-if="tree.sessionsError"
        testid="terminal-sessions-error"
        variant="line"
        class="px-3 py-2"
        :message="tree.sessionsError"
      />
      <p
        v-else-if="!view.ready"
        class="px-3 py-2 font-mono text-xs text-text-4"
        data-testid="terminal-sessions-loading"
      >
        Loading…
      </p>
      <!-- The rails' positioning context, and the box whose resize tells them a row moved. -->
      <div ref="treeContent" class="relative" :class="{ 'tree-settling': !treeSettled }">
        <div v-for="group in view.groups" :key="group.key" class="border-t border-border first:border-t-0">
          <!-- Chats take a repository's header: it heads several rows, so its name cannot be one of them. -->
          <div
            v-if="group.kind !== 'scratch'"
            class="repo-group"
            role="button"
            tabindex="0"
            :data-testid="group.kind === 'chats' ? 'terminal-chats-group' : 'terminal-repo-group'"
            :data-repo="group.key"
            :aria-expanded="view.expansion.expanded(group)"
            @click="view.expansion.toggle(group)"
            @keydown.enter.self.prevent="view.expansion.toggle(group)"
            @keydown.space.self.prevent="view.expansion.toggle(group)"
          >
            <span class="min-w-0 truncate text-body text-text">{{ group.name }}</span>
            <div class="group-trailing" @click.stop>
              <span
                class="group-count font-mono text-caption"
                :class="tree.groupAttached(group) ? 'text-accent' : 'text-text-4'"
                >{{ group.sessions.length }}</span
              >
              <IconButton
                v-if="group.kind === 'repo' && group.key"
                :label="`New session in ${group.name}`"
                :icon="IconPlus"
                size="sm"
                class="row-action group-add"
                data-testid="terminal-repo-new-session"
                @click="openNewSession(group.key)"
              />
            </div>
            <component
              :is="view.expansion.expanded(group) ? IconChevronDown : IconChevronRight"
              class="size-3 shrink-0 text-text-4"
            />
          </div>
          <!-- The scratch terminal's heading is its session row, so it carries the row's controls and
               is a div: a button cannot hold one. -->
          <template v-else>
            <div
              v-for="row in group.sessions"
              :key="row.id"
              class="group-row"
              :class="{ 'menu-open': menus.openRow === row.id }"
              role="button"
              tabindex="-1"
              data-testid="terminal-scratch-heading"
              :data-slug="row.slug"
              :aria-expanded="view.expansion.expanded(group)"
              :title="row.slug"
              @click="view.expansion.toggle(group)"
              @keydown.enter.self.prevent="view.expansion.toggle(group)"
              @keydown.space.self.prevent="view.expansion.toggle(group)"
              @contextmenu.prevent="menus.toggleRow(row, $event)"
            >
              <span class="min-w-0 truncate text-body text-text">{{ group.name }}</span>
              <component
                :is="view.expansion.expanded(group) ? IconChevronDown : IconChevronRight"
                class="ml-auto size-3 shrink-0 text-text-4"
              />
              <div class="row-trailing" data-testid="terminal-session-trailing" @click.stop>
                <IconButton
                  label="New tab"
                  :icon="IconPlus"
                  size="sm"
                  class="row-action row-lead"
                  data-testid="terminal-new-window"
                  @click="ops.newWindowIn(row)"
                />
                <span
                  v-if="tree.rowRunning(row)"
                  class="row-status text-severity-success"
                  data-testid="terminal-session-liveness"
                >
                  <span class="size-2.5 rounded-full bg-current" aria-hidden="true" />
                  <span class="sr-only">Terminal running</span>
                </span>
                <IconButton
                  :ref="(el) => menus.setRowToggle(row.id, el)"
                  label="Terminal actions"
                  :icon="IconEllipsisVertical"
                  size="sm"
                  class="row-action row-swap"
                  aria-haspopup="menu"
                  :aria-expanded="menus.openRow === row.id"
                  data-testid="terminal-session-menu-toggle"
                  @click="menus.toggleRow(row)"
                />
                <SessionRowMenu
                  v-if="menus.openRow === row.id"
                  :session="row"
                  scratch
                  :flip="menus.rowFlip"
                  :ignore="[menus.rowToggle(row.id)]"
                  @close="menus.openRow = ''"
                  @start="attach.startSession(row.slug)"
                  @kill="ops.requestKill(row)"
                />
              </div>
            </div>
          </template>
          <Transition name="tree-expand" v-bind="expandHooks">
            <div v-if="view.expansion.expanded(group)">
              <TransitionGroup name="tree" tag="div" class="relative flex flex-col border-t border-border bg-app py-1">
                <div v-for="row in group.sessions" :key="row.id">
                  <!-- Not a <button>: the row's menu toggle is one, and buttons cannot nest. -->
                  <div
                    v-if="group.kind !== 'scratch'"
                    class="session-row"
                    :class="{
                      'session-row-attached': row.slug === pool.activeSlug,
                      'menu-open': menus.openRow === row.id || menus.openNewWindow === row.id,
                    }"
                    role="button"
                    :tabindex="nav.tabStopKey === `s:${row.id}` ? 0 : -1"
                    :data-testid="group.kind === 'chats' ? 'terminal-chat-row' : 'terminal-session-row'"
                    :data-slug="row.slug"
                    :data-tree-key="`s:${row.id}`"
                    :data-attached="row.slug === pool.activeSlug"
                    :title="row.slug"
                    @click="nav.clickSession(row)"
                    @keydown.enter.self.prevent="nav.enterSession(row)"
                    @keydown.space.self.prevent="nav.enterSession(row)"
                    @contextmenu.prevent="menus.toggleRow(row, $event)"
                  >
                    <span class="min-w-0 flex-1 truncate text-body" :class="{ 'text-text-3': tree.rowIdle(row) }">{{
                      row.name
                    }}</span>
                    <BaseBadge
                      v-if="sessionAgeBadges[row.id]"
                      tone="accent"
                      class="shrink-0 px-1.5 py-0.5 font-mono text-micro leading-none"
                      :title="sessionAgeBadges[row.id].title"
                      :aria-label="`${sessionAgeBadges[row.id].days} ${sessionAgeBadges[row.id].days === 1 ? 'day' : 'days'} old`"
                      data-testid="terminal-session-age"
                    >
                      {{ sessionAgeBadges[row.id].days }}d
                    </BaseBadge>
                    <!-- AppMenu anchors to the positioned row so its panel spans it; the grid
                         overlap keeps this slot from becoming a positioning ancestor. -->
                    <div class="row-trailing" data-testid="terminal-session-trailing" @click.stop>
                      <IconButton
                        v-if="group.kind !== 'chats'"
                        label="New window"
                        :icon="IconPlus"
                        size="sm"
                        class="row-action row-lead"
                        aria-haspopup="menu"
                        :aria-expanded="menus.openNewWindow === row.id"
                        :disabled="ops.agentWindowBusy"
                        data-testid="terminal-new-window"
                        @click="menus.toggleNewWindow(row, $event)"
                      />
                      <NewWindowMenu
                        v-if="menus.openNewWindow === row.id"
                        :running="tree.rowRunning(row)"
                        :flip="menus.newWindowFlip"
                        :ignore="[menus.newWindowToggle]"
                        :agents="ops.agentProfiles"
                        :default-agent="ops.defaultAgentProfile"
                        :loading="ops.agentProfilesLoading"
                        :failed="ops.agentProfilesFailed"
                        @close="menus.openNewWindow = ''"
                        @terminal="ops.newWindowIn(row)"
                        @agent="ops.newAgentWindowIn(row, $event)"
                        @retry="ops.loadAgentProfiles"
                      />
                      <span
                        v-if="tree.rowRunning(row)"
                        class="row-status text-severity-success"
                        data-testid="terminal-session-liveness"
                      >
                        <span class="size-2.5 rounded-full bg-current" aria-hidden="true" />
                        <span class="sr-only">{{ group.kind === 'chats' ? 'Agent running' : 'Terminal running' }}</span>
                      </span>
                      <IconButton
                        :ref="(el) => menus.setRowToggle(row.id, el)"
                        :label="group.kind === 'chats' ? 'Chat actions' : 'Session actions'"
                        :icon="IconEllipsisVertical"
                        size="sm"
                        class="row-action row-swap"
                        aria-haspopup="menu"
                        :aria-expanded="menus.openRow === row.id"
                        :data-testid="
                          group.kind === 'chats' ? 'terminal-chat-menu-toggle' : 'terminal-session-menu-toggle'
                        "
                        @click="menus.toggleRow(row)"
                      />
                      <AppMenu
                        v-if="menus.openRow === row.id && group.kind === 'chats'"
                        :entries="chatMenuEntries"
                        :flip="menus.rowFlip"
                        width="min(230px, 100%)"
                        :ignore="[menus.rowToggle(row.id)]"
                        testid="terminal-chat-menu"
                        @select="onChatMenuSelect(row, $event)"
                        @close="menus.openRow = ''"
                      />
                      <SessionRowMenu
                        v-else-if="menus.openRow === row.id"
                        :session="row"
                        :extra="ops.sessionActionEntries"
                        :flip="menus.rowFlip"
                        :ignore="[menus.rowToggle(row.id)]"
                        @extra="ops.runSessionAction(row, $event)"
                        @close="menus.openRow = ''"
                        @start="attach.startSession(row.slug)"
                        @kill="ops.requestKill(row)"
                        @detail="sessions.openDetail(row)"
                        @rename="sessions.requestRename(row)"
                        @recycle="sessions.requestRecycle(row)"
                        @delete="sessions.requestDelete(row)"
                      />
                    </div>
                  </div>
                  <Transition name="tree-expand" v-bind="expandHooks">
                    <div v-if="tree.windowRowsFor(row).length && view.expansion.expanded(group)">
                      <TransitionGroup name="tree" tag="div" class="relative flex flex-col pb-1">
                        <!-- The slot carries the drag and its marker: the row's ::before and
                             ::after draw the tree connector. -->
                        <div
                          v-for="(win, index) in tree.windowRowsFor(row)"
                          :key="win.windowId"
                          class="window-slot"
                          :class="[
                            dropClass(windowDropTarget, windowMenuKey(row, win.windowId)),
                            { 'opacity-40': draggingWindowRow(row.slug, win.windowId) },
                          ]"
                          :draggable="win.live && rename.windowId !== win.windowId"
                          data-testid="terminal-window-slot"
                          @dragstart="windowDrag.start($event, { slug: row.slug, windowId: win.windowId })"
                          @dragover.prevent="onWindowDragOver($event, row, win.windowId)"
                          @drop.prevent="windowDrag.drop"
                          @dragend="windowDrag.end"
                        >
                          <div
                            class="window-row"
                            :class="{
                              'window-row-flush': group.kind === 'scratch',
                              'window-row-last': index === tree.windowRowsFor(row).length - 1,
                              'window-row-active': win.active,
                              'has-swap': win.live,
                              'menu-open': menus.openWindow === windowMenuKey(row, win.windowId),
                            }"
                            role="button"
                            :tabindex="nav.tabStopKey === `w:${row.id}:${win.windowId}` ? 0 : -1"
                            :data-testid="win.live ? 'terminal-window-row' : 'terminal-listed-window-row'"
                            :data-window-id="win.windowId"
                            :data-tree-key="`w:${row.id}:${win.windowId}`"
                            :data-active="win.live ? win.active : undefined"
                            @click="nav.openWindow(row, win)"
                            @keydown.enter.self.prevent="nav.openWindow(row, win)"
                            @keydown.space.self.prevent="nav.openWindow(row, win)"
                            @dblclick="rename.start(win)"
                            @contextmenu.prevent="menus.toggleWindow(row, win.windowId, $event)"
                          >
                            <input
                              v-if="rename.windowId === win.windowId"
                              v-model="rename.draft"
                              class="min-w-0 flex-1 bg-transparent font-mono text-small text-text outline-none"
                              data-testid="terminal-rename-input"
                              autocapitalize="off"
                              autocorrect="off"
                              spellcheck="false"
                              autofocus
                              @click.stop
                              @dblclick.stop
                              @keydown.enter="rename.commit"
                              @keydown.esc="rename.windowId = ''"
                              @blur="rename.commit"
                            />
                            <span v-else class="min-w-0 flex-1 truncate font-mono text-small">{{ win.name }}</span>
                            <div class="window-trailing" data-testid="terminal-window-trailing" @click.stop>
                              <IconButton
                                v-if="win.live"
                                :label="`Close ${win.name}`"
                                :icon="IconX"
                                size="sm"
                                class="row-action row-swap"
                                data-testid="terminal-close-window"
                                @click="ops.requestCloseWindow(row.slug, win.windowId, win.name)"
                              />
                              <span
                                v-if="win.indicator"
                                class="window-status"
                                :class="win.indicator.color"
                                :title="win.indicator.label"
                                data-testid="terminal-window-status"
                                :data-status="tree.windowStatus(row, win.windowId)?.status"
                              >
                                <component
                                  :is="win.indicator.icon"
                                  class="size-3"
                                  :class="{ 'animate-spin': win.indicator.animated }"
                                  aria-hidden="true"
                                />
                                <span class="sr-only">{{ win.indicator.label }}</span>
                              </span>
                              <!-- Only once a configured action targets a window. -->
                              <IconButton
                                v-if="ops.rowHasWindowActions(row)"
                                :ref="(el) => menus.setWindowToggle(windowMenuKey(row, win.windowId), el)"
                                label="Window actions"
                                :icon="IconEllipsisVertical"
                                size="sm"
                                class="row-action row-lead"
                                aria-haspopup="menu"
                                :aria-expanded="menus.openWindow === windowMenuKey(row, win.windowId)"
                                data-testid="terminal-window-menu-toggle"
                                @click="menus.toggleWindow(row, win.windowId)"
                              />
                              <AppMenu
                                v-if="menus.openWindow === windowMenuKey(row, win.windowId)"
                                :entries="ops.windowActionEntries"
                                :flip="menus.windowFlip"
                                width="min(230px, 100%)"
                                :ignore="[menus.windowToggle(windowMenuKey(row, win.windowId))]"
                                testid="terminal-window-menu"
                                @select="runWindowAction(row, win.windowId, $event)"
                                @close="menus.openWindow = ''"
                              />
                            </div>
                          </div>
                        </div>
                      </TransitionGroup>
                    </div>
                    <!-- An empty scratch section still gives the keyboard a row to land on. -->
                    <div v-else-if="group.kind === 'scratch' && view.expansion.expanded(group)" class="pb-1">
                      <div
                        class="window-row window-row-flush window-row-last text-text-4"
                        role="button"
                        :tabindex="nav.tabStopKey === `s:${row.id}` ? 0 : -1"
                        data-testid="terminal-start-scratch"
                        :data-tree-key="`s:${row.id}`"
                        @click="nav.enterSession(row)"
                        @keydown.enter.self.prevent="nav.enterSession(row)"
                        @keydown.space.self.prevent="nav.enterSession(row)"
                      >
                        <span class="min-w-0 flex-1 truncate font-mono text-small">Start a terminal</span>
                      </div>
                    </div>
                  </Transition>
                </div>
              </TransitionGroup>
            </div>
          </Transition>
        </div>
        <!-- Last, so they paint over the rows: every row and panel is
             positioned too, and among positioned boxes DOM order decides. -->
        <TreeRail :rail="rails[0]" data-testid="terminal-session-rail" />
        <TreeRail :rail="rails[1]" data-testid="terminal-window-rail" />
      </div>
      <EmptyState
        v-if="view.note === 'empty'"
        variant="inline"
        class="px-3 py-2"
        message="No active sessions. Start one from the hub and it will appear here."
        data-testid="terminal-sessions-empty"
      />
      <EmptyState
        v-else-if="view.note === 'no-matches'"
        variant="inline"
        class="px-3 py-2"
        :message="view.noMatchesNote"
        data-testid="terminal-sessions-no-matches"
      />
    </div>
    <!-- The tree's keys are not announced anywhere else. The focus chord is read off the
         live keymap because it is rebindable. -->
    <div v-if="tree.attachable.length" class="tree-hints" data-testid="terminal-tree-hints">
      <span><Kbd class="text-text-3">↑↓</Kbd> switch</span>
      <span><Kbd class="text-text-3">↵</Kbd> enter</span>
      <span v-if="focusHint"
        ><Kbd class="text-text-3">{{ focusHint.keys }}</Kbd> {{ focusHint.label }}</span
      >
    </div>
    <PanelResizeHandle edge="right" name="terminal-sidebar" :start="startResize" :step="step" />
  </aside>
</template>

<style scoped>
/* Pinned under the tree: a legend that scrolls away cannot be consulted when it is needed. */
.tree-hints {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
  padding: 8px 12px;
  border-top: 1px solid var(--color-border);
  font-family: var(--font-mono);
  font-size: var(--text-caption);
  color: var(--color-text-4);
  user-select: none;
}

/* The headings hold controls, which a button cannot contain, so every row is a
   div wearing a button's role. */
.repo-group,
.group-row,
.session-row,
.window-row {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  text-align: left;
  cursor: pointer;
}
.group-row,
.session-row,
.window-row {
  position: relative;
}
.repo-group,
.group-row {
  height: 36px;
}
.session-row {
  height: 30px;
  padding-left: 20px;
  color: var(--color-text);
}
.window-row {
  height: 28px;
  padding-left: 40px;
  color: var(--color-text-2);
}
:is(.repo-group, .group-row, .session-row, .window-row):is(:hover, .menu-open) {
  background: var(--color-chip);
}
/* No focus styling of their own: the walk activates the row it lands on, so
   the rail and the accent already follow the cursor. The UA outline has to be
   removed explicitly. */
:is(.repo-group, .group-row, .session-row, .window-row):focus-visible {
  outline: none;
}
/* No fill: the rail and the accent find the attached row, and the surface keeps its hover feedback. */
.session-row-attached,
.window-row-active {
  font-weight: 500;
  color: var(--color-accent);
}

/* One cell, two occupants: the count is what the row says at rest, the add
   button what it offers under the pointer. */
.group-trailing {
  display: grid;
  margin-left: auto;
  min-width: 18px;
  height: 18px;
  flex: none;
  align-self: center;
}
.group-count,
.group-add {
  grid-area: 1 / 1;
}
.group-count {
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: none;
}
.repo-group:hover .group-count,
.group-trailing:focus-within .group-count {
  opacity: 0;
}
.repo-group:hover .group-add,
.group-add:focus-visible {
  opacity: 1;
}

/* The tree connector is drawn, not typed: a box-drawing glyph is only as tall as
   its font size, so stacked rows would show gaps. ::before is the vertical,
   stopped at the elbow on the last row; ::after is the tick into the name. */
.window-row::before {
  content: '';
  position: absolute;
  left: 26px;
  top: 0;
  bottom: 0;
  border-left: 1px solid var(--color-strong);
}
.window-row::after {
  content: '';
  position: absolute;
  left: 26px;
  top: 50%;
  width: 9px;
  border-top: 1px solid var(--color-strong);
}
.window-row-last::before {
  bottom: 50%;
}
/* The scratch section lists its tabs where a repository lists its sessions, so
   they take that row's box and drop the connector. */
.window-row-flush {
  height: 30px;
  padding-left: 20px;
}
.window-row-flush::before,
.window-row-flush::after {
  display: none;
}

/* On the slot because the row's own ::before and ::after are the connector. */
.window-slot {
  position: relative;
}
.window-slot.drop-before {
  box-shadow: inset 0 2px 0 0 var(--color-accent);
}
.window-slot.drop-after {
  box-shadow: inset 0 -2px 0 0 var(--color-accent);
}

/* Fast enough to read as instant. A leaving row drops out of flow so its
   neighbors glide up through .tree-move (FLIP) instead of snapping. */
.tree-enter-active,
.tree-leave-active,
.tree-move {
  transition:
    opacity 0.15s ease,
    transform 0.15s ease;
}
.tree-enter-from,
.tree-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
.tree-leave-active {
  position: absolute;
  left: 0;
  right: 0;
}
/* Decelerating: ease-in-out spends its first frames barely moving and reads as lag. */
.tree-expand-enter-active,
.tree-expand-leave-active {
  overflow: hidden;
  transition: height 0.18s cubic-bezier(0.2, 0, 0, 1);
}

.tree-settling
  :is(.tree-enter-active, .tree-leave-active, .tree-move, .tree-expand-enter-active, .tree-expand-leave-active) {
  transition: none;
}

@media (prefers-reduced-motion: reduce) {
  .tree-enter-active,
  .tree-leave-active,
  .tree-move,
  .tree-expand-enter-active,
  .tree-expand-leave-active {
    transition: none;
  }
}

/* Two fixed columns keep session and window names aligned. The second holds the
   status glyph, and the control that replaces it on hover shares that cell.
   A listed window has no live client to close, so it keeps its status and
   tooltip there; .has-swap says otherwise. */
.row-trailing,
.window-trailing {
  display: grid;
  grid-template-columns: 18px 18px;
  width: 36px;
  height: 18px;
  flex: none;
  align-self: center;
}
.row-lead {
  grid-area: 1 / 1;
}
.row-status,
.window-status,
.row-swap {
  grid-area: 1 / 2;
}
.row-status,
.window-status {
  display: flex;
  width: 18px;
  height: 18px;
  flex: none;
  align-items: center;
  justify-content: center;
}
.row-status,
.window-row.has-swap .window-status {
  pointer-events: none;
}
.row-action {
  opacity: 0;
}
.row-action:hover,
.row-action[aria-expanded='true'] {
  background: var(--color-app);
  color: var(--color-text);
}
:is(.session-row, .group-row, .window-row):is(:hover, .menu-open) .row-action,
.row-action:focus-visible {
  opacity: 1;
}
:is(.session-row, .group-row):is(:hover, .menu-open) .row-status,
.row-trailing:focus-within .row-status {
  opacity: 0;
}
.window-row.has-swap:hover .window-status,
.window-row.has-swap .window-trailing:focus-within .window-status {
  opacity: 0;
}
</style>
