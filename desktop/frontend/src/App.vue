<script setup lang="ts">
import TitleBar from './components/TitleBar.vue'
import ProfileRail from './components/ProfileRail.vue'
import SideBar from './components/SideBar.vue'
import FeedList from './components/FeedList.vue'
import DetailPane from './components/DetailPane.vue'
import { useFeedState } from './composables/useFeedState'

const {
  profiles, activeProfile, activeProfileId, selection, items, selectedId, selectedItem,
  actions, unreadOnly, title, countLabel, toast, selectProfile, selectSidebar, selectItem,
  toggleUnread, refresh, notWired, hideWindow,
} = useFeedState()
</script>

<template>
  <main class="h-screen w-screen overflow-hidden border border-card bg-app text-text">
    <div class="flex h-full min-h-0 flex-col overflow-hidden rounded-[10px]">
      <TitleBar :profile-name="activeProfile?.name ?? 'Loading'" @hide="hideWindow" />
      <div class="flex min-h-0 flex-1">
        <ProfileRail :profiles="profiles" :active-profile-id="activeProfileId" @select="selectProfile" />
        <SideBar
          v-if="activeProfile"
          :profile="activeProfile"
          :selection="selection"
          @select="selectSidebar"
        />
        <main v-if="activeProfile" class="flex min-w-0 flex-1">
          <FeedList
            :title="title"
            :items="items"
            :selected-id="selectedId"
            :unread-only="unreadOnly"
            :count-label="countLabel"
            @select="selectItem"
            @toggle-unread="toggleUnread"
            @refresh="refresh"
          />
          <DetailPane :item="selectedItem" :actions="actions" @run-action="notWired" @open-browser="notWired" />
        </main>
        <div v-else class="flex flex-1 items-center justify-center font-mono text-xs text-text-4">Loading feed…</div>
      </div>
    </div>
    <Transition name="toast">
      <div v-if="toast" class="fixed bottom-5 right-5 rounded-lg border border-strong bg-chip px-4 py-2.5 font-mono text-xs text-text shadow-2xl">{{ toast }}</div>
    </Transition>
  </main>
</template>

<style scoped>
.toast-enter-active, .toast-leave-active { transition: opacity .16s ease, transform .16s ease; }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateY(5px); }
</style>
