<script setup lang="ts">
import { useTemplateRef } from 'vue'
import SideBar from '../components/SideBar.vue'
import FeedList from '../components/FeedList.vue'
import DetailPane from '../components/DetailPane.vue'
import { useNewSession } from '../composables/useNewSession'
import { useFlowsSession } from '../pipeline/composables/useFlowsSession'
import type { AppNavigation, FeedState } from './useAppNavigation'
import type { useFeedCommands } from './useFeedCommands'
import type { useSelectedItemDetail } from './useSelectedItemDetail'

const props = defineProps<{
  feed: FeedState
  nav: AppNavigation
  feedCommands: ReturnType<typeof useFeedCommands>
  detail: ReturnType<typeof useSelectedItemDetail>
  sidebarCollapsed: boolean
  githubConnected: boolean
}>()
const previewCollapsed = defineModel<boolean>('previewCollapsed', { required: true })

// The composables' result objects keep their identity for App's lifetime.
// eslint-disable-next-line vue/no-setup-props-reactivity-loss
const { feed, nav, feedCommands, detail } = props
const { events: selectedEvents, sessions: itemSessions, chats: itemChats } = detail
const {
  activeProfile,
  profilesError,
  selection,
  title,
  visibleItems,
  selectedId,
  selectedItem,
  unreadOnly,
  unreadCount,
  visibleArchivedItems,
  archivedCount,
  archivedExpanded,
  trashFilter,
  search,
  authors,
  authorFilter,
  feedSort,
  loadError,
  refreshingSources,
  itemSelectionActive,
  selectedItemIDs,
  selectionActions,
  sourceIcons,
  sourceImages,
  actions,
  pendingAction,
  actionRuns,
  openedActionRunID,
} = feed
const flowsSession = useFlowsSession()
const { openFromItem: openNewSessionFromItem } = useNewSession()

// Activating a row (double-click, or Enter/Space) asks to read it, so it opens
// a collapsed pane. A single click only moves the selection: it is also the
// first half of every double-click.
async function activateItemFromRow(id: number): Promise<void> {
  previewCollapsed.value = false
  await feed.selectItem(id)
}

const feedList = useTemplateRef<{ focusSearch: () => void }>('feedList')
defineExpose({ focusSearch: () => feedList.value?.focusSearch() })
</script>

<template>
  <SideBar
    v-if="activeProfile && !sidebarCollapsed"
    :profile="activeProfile"
    :selection="selection"
    :flows-dirty="flowsSession.dirty.value"
    @select="nav.navigateSidebar"
    @open-flows="nav.openFlows()"
    @open-settings="nav.openSettings('profile')"
    @reorder="(t) => activeProfile && feed.reorderFeeds(activeProfile.id, t)"
  />
  <!-- A profile created before an account was connected has no graph, so say
       what is missing rather than landing on an empty Trash view. `tree` tells
       "no feed nodes" apart from "feeds not read yet", so a reload in flight
       does not flash this. -->
  <div
    v-if="activeProfile?.tree && activeProfile.feeds.length === 0"
    class="flex min-w-0 flex-1 flex-col items-center justify-center gap-3 px-10 text-center"
    data-testid="workspace-empty"
  >
    <div class="text-body font-semibold">No sources yet</div>
    <p class="max-w-[400px] text-xs leading-relaxed text-text-3">
      {{
        githubConnected
          ? 'This profile has no feeds. Open the flow editor to wire a source into one.'
          : 'This profile has no feeds, and no account is connected to fetch as. Connect one under Integrations, then wire a source into a feed.'
      }}
    </p>
    <div class="mt-1 flex items-center gap-2">
      <button
        v-if="!githubConnected"
        class="cursor-pointer rounded border border-strong px-3 py-1.5 text-xs text-text-2 hover:text-text"
        data-testid="workspace-empty-integrations"
        @click="nav.selectApplicationSettingsSection('integrations')"
      >
        Open Integrations
      </button>
      <button
        class="cursor-pointer rounded border border-strong px-3 py-1.5 text-xs text-text-2 hover:text-text"
        data-testid="workspace-empty-flows"
        @click="nav.openFlows()"
      >
        Edit flow
      </button>
    </div>
  </div>
  <section v-else-if="activeProfile" class="flex min-w-0 flex-1">
    <FeedList
      ref="feedList"
      :title="title"
      :visible-items="visibleItems"
      :selected-id="selectedId"
      :unread-only="unreadOnly"
      :unread-count="unreadCount"
      :archived-items="visibleArchivedItems"
      :archived-count="archivedCount"
      :archived-expanded="archivedExpanded"
      :trash="selection.type === 'trash'"
      :trash-filter="trashFilter"
      :search="search"
      :authors="authors"
      :author-filter="authorFilter"
      :sort="feedSort"
      :load-error="loadError"
      :refreshing="refreshingSources"
      :selection-mode="itemSelectionActive"
      :selected-item-ids="selectedItemIDs"
      :selection-actions="selectionActions"
      :source-icons="sourceIcons"
      :source-images="sourceImages"
      @select="feed.selectItem"
      @activate="activateItemFromRow"
      @update:search="(value) => (search = value)"
      @update:author-filter="(value) => (authorFilter = value)"
      @set-sort="feed.setFeedSort"
      @set-unread="nav.navigateUnreadFilter"
      @toggle-archived="feed.toggleArchivedSection"
      @set-trash-filter="feed.setTrashFilter"
      @refresh="feed.refreshSources"
      @mark-all-read="feedCommands.markSelectedFeedRead"
      @enter-selection="feed.enterItemSelection"
      @toggle-item-selection="feed.toggleItemSelection"
      @cancel-selection="feed.cancelItemSelection"
      @copy-selection-contents="feed.copySelectedItemContents"
      @run-selection-action="feed.invokeSelectionAction"
      @create-session-from-selection="feedCommands.openSelectedItemsSession"
      @item-set-unread="feed.markItemUnread"
      @item-toggle-archive="feed.toggleArchive"
      @item-toggle-ignored="feed.toggleIgnored"
      @item-open-browser="feed.openItemInBrowser"
      @item-copy-link="feed.copyItemLink"
      @item-copy-contents="feed.copyItemContents"
      @item-create-session="openNewSessionFromItem"
      @item-run-action="feed.runItemAction"
    />
    <DetailPane
      v-if="!previewCollapsed"
      :item="selectedItem"
      :events="selectedEvents"
      :actions="actions"
      :sessions="itemSessions"
      :chats="itemChats"
      :pending-action="pendingAction"
      :action-runs="actionRuns"
      :opened-action-run-id="openedActionRunID"
      :source-icons="sourceIcons"
      :source-images="sourceImages"
      @run-action="feed.invokeAction"
      @open-browser="feed.openSelectedInBrowser"
      @open-url="feed.openUrl"
      @set-unread="(value) => selectedItem && feed.markItemUnread(selectedItem, value)"
      @toggle-archive="selectedItem && feed.toggleArchive(selectedItem)"
      @toggle-ignored="selectedItem && feed.toggleIgnored(selectedItem)"
      @copy-link="selectedItem && feed.copyItemLink(selectedItem)"
      @copy-contents="selectedItem && feed.copyItemContents(selectedItem)"
      @create-session="(target) => selectedItem && openNewSessionFromItem(selectedItem, target)"
      @open-session="nav.openItemSession"
      @open-chat="nav.openItemChat"
      @edit="nav.selectApplicationSettingsSection('actions')"
    />
  </section>
  <div v-else class="flex flex-1 flex-col items-center justify-center gap-3 font-mono text-xs text-text-4">
    <template v-if="profilesError">
      <span data-testid="profiles-error">{{ profilesError }}</span>
      <button
        class="cursor-pointer rounded border border-strong px-3 py-1.5 text-text-2 hover:text-text"
        @click="feed.loadProfiles"
      >
        Retry
      </button>
    </template>
    <span v-else>Loading feed…</span>
  </div>
</template>
