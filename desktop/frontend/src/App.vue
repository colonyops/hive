<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue'
import TitleBar from './components/TitleBar.vue'
import ProfileRail from './components/ProfileRail.vue'
import SideBar from './components/SideBar.vue'
import FeedList from './components/FeedList.vue'
import DetailPane from './components/DetailPane.vue'
import CommandPalette from './components/CommandPalette.vue'
import OnboardingScreen from './components/OnboardingScreen.vue'
import { useAuth } from './composables/useAuth'
import { useFeedState } from './composables/useFeedState'
import { useCommands, useCommandPalette, type Command } from './composables/useCommands'
import { setTheme, themeLabels, themes } from './composables/useTheme'

const {
  status: authStatus, authenticated, deviceFlow, card: authCard, error: authError, busy: authBusy,
  startDeviceFlow, useTokenInstead, backToStart, submitToken,
} = useAuth()

const {
  profiles, profilesLoaded, activeProfile, activeProfileId, selection, items, loadError,
  selectedId, selectedItem, actions, unreadOnly, title, countLabel, toast,
  creatingProfile, createProfileError, loadProfiles, createProfile,
  selectProfile, selectSidebar, selectUnreadView, selectItem,
  toggleUnread, refresh, notWired, hideWindow,
} = useFeedState()

// Booting while signed out leaves profiles unloaded (or the live backend
// erroring); re-load the moment auth lands, including post-onboarding.
watch(authenticated, (isAuthed) => {
  if (isAuthed) void loadProfiles()
})

// Step 2 of onboarding: authenticated but no workspace exists yet.
const needsWorkspace = computed(() => authenticated.value && profilesLoaded.value && profiles.value.length === 0)

// ── Command palette ──────────────────────────────────────────────────────────

const { open: paletteOpen, toggle: togglePalette } = useCommandPalette()

// Seed commands — reactive getter so they update when profiles/feeds load
useCommands(computed(() => {
  const cmds: Command[] = []

  // Profiles
  for (const p of profiles.value) {
    cmds.push({
      id: `profile:${p.id}`,
      title: `Switch to profile: ${p.name}`,
      group: 'Profiles',
      run: () => selectProfile(p.id),
    })
  }

  // Feeds — All items always first, then individual feeds
  cmds.push({
    id: 'feed:all',
    title: 'Select feed: All items',
    group: 'Feeds',
    run: () => selectSidebar({ type: 'all' }),
  })

  for (const f of activeProfile.value?.feeds ?? []) {
    cmds.push({
      id: `feed:${f.id}`,
      title: `Select feed: ${f.name}`,
      group: 'Feeds',
      run: () => selectSidebar({ type: 'feed', feedId: f.id }),
    })
  }

  cmds.push({
    id: 'feed:toggle-unread',
    title: 'Toggle unread filter',
    group: 'Feeds',
    run: toggleUnread,
  })

  // Themes
  for (const t of themes) {
    cmds.push({
      id: `theme:${t}`,
      title: `Theme: ${themeLabels[t]}`,
      group: 'Theme',
      keywords: ['theme', 'appearance', t],
      run: () => setTheme(t),
    })
  }

  // Window
  cmds.push({
    id: 'window:hide',
    title: 'Hide window',
    group: 'Window',
    run: hideWindow,
  })

  return cmds
}))

// ── Global ⌘K / Ctrl+K listener ──────────────────────────────────────────────

function onGlobalKeydown(e: KeyboardEvent): void {
  if (e.key.toLowerCase() !== 'k' || (!e.metaKey && !e.ctrlKey)) return

  // If palette is already open, always close it (even from inside the input)
  if (paletteOpen.value) {
    e.preventDefault()
    togglePalette()
    return
  }

  // Ignore ⌘K while focus is in any editable element other than the palette input
  const target = e.target as HTMLElement
  const isEditable =
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    target.isContentEditable
  if (isEditable) return

  e.preventDefault()
  togglePalette()
}

onMounted(() => window.addEventListener('keydown', onGlobalKeydown))
onUnmounted(() => window.removeEventListener('keydown', onGlobalKeydown))
</script>

<template>
  <main class="h-screen w-screen overflow-hidden bg-app text-text">
    <div class="flex h-full min-h-0 flex-col overflow-hidden">
      <TitleBar
        :profile-name="authenticated && !needsWorkspace ? activeProfile?.name ?? 'Loading' : undefined"
        :unread-count="activeProfile?.unreadCount ?? 0"
      />
      <!-- Hold an empty frame until auth status resolves so an authenticated
           user never sees onboarding flash by. -->
      <div v-if="authStatus === null" class="flex min-h-0 flex-1 items-center justify-center font-mono text-xs text-text-4">Loading…</div>
      <OnboardingScreen
        v-else-if="!authenticated || needsWorkspace"
        :card="needsWorkspace ? 'workspace' : authCard"
        :device-flow="deviceFlow"
        :error="needsWorkspace ? createProfileError : authError"
        :busy="needsWorkspace ? creatingProfile : authBusy"
        @start-device-flow="startDeviceFlow"
        @use-token-instead="useTokenInstead"
        @back-to-start="backToStart"
        @submit-token="submitToken"
        @create-workspace="createProfile"
      />
      <div v-else class="flex min-h-0 flex-1">
        <ProfileRail :profiles="profiles" :active-profile-id="activeProfileId" @select="selectProfile" />
        <SideBar
          v-if="activeProfile"
          :profile="activeProfile"
          :selection="selection"
          :unread-only="unreadOnly"
          @select="selectSidebar"
          @select-unread="selectUnreadView"
        />
        <section v-if="activeProfile" class="flex min-w-0 flex-1">
          <FeedList
            :title="title"
            :items="items"
            :selected-id="selectedId"
            :unread-only="unreadOnly"
            :count-label="countLabel"
            :load-error="loadError"
            @select="selectItem"
            @toggle-unread="toggleUnread"
            @refresh="refresh"
          />
          <DetailPane :item="selectedItem" :actions="actions" @run-action="notWired" @open-browser="notWired" @edit="notWired" />
        </section>
        <div v-else class="flex flex-1 items-center justify-center font-mono text-xs text-text-4">Loading feed…</div>
      </div>
    </div>
    <Transition name="toast">
      <div v-if="toast" class="fixed bottom-5 right-5 rounded-lg border border-strong bg-chip px-4 py-2.5 font-mono text-xs text-text shadow-2xl" data-testid="toast">{{ toast }}</div>
    </Transition>
    <CommandPalette />
  </main>
</template>

<style scoped>
.toast-enter-active, .toast-leave-active { transition: opacity .16s ease, transform .16s ease; }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateY(5px); }
</style>
