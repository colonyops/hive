<script setup lang="ts">
import { computed, defineAsyncComponent, useTemplateRef } from 'vue'
import TitleBar from './components/TitleBar.vue'
import ProfileRail from './components/ProfileRail.vue'
import OnboardingScreen from './components/OnboardingScreen.vue'
import ProfileSettingsView from './components/ProfileSettingsView.vue'
import SettingsView from './components/SettingsView.vue'
import SequenceHint from './components/SequenceHint.vue'
import FlowsView from './pipeline/components/FlowsView.vue'
import FeedHub from './app/FeedHub.vue'
import AppDialogs from './app/AppDialogs.vue'
import { useAppCommands } from './app/useAppCommands'
import { useAppMode } from './app/useAppMode'
import { useAppNavigation } from './app/useAppNavigation'
import { useFeedCommands } from './app/useFeedCommands'
import { useGlobalKeymap } from './app/useGlobalKeymap'
import { useHubOverlays } from './app/useHubOverlays'
import { useOnboarding } from './app/useOnboarding'
import { useProfileActions } from './app/useProfileActions'
import { useReveal } from './app/useReveal'
import { useSelectedItemDetail } from './app/useSelectedItemDetail'
import { useSelfUpdate } from './app/useSelfUpdate'
import { useCommandPalette } from './composables/useCommands'
import { useConfirmation } from './composables/useConfirmation'
import { useFeedState } from './composables/useFeedState'
import { useGitHubConnection } from './composables/useGitHubConnection'
import { useWailsEvent } from './composables/useWailsEvent'
import { useFlowsSession } from './pipeline/composables/useFlowsSession'
import { useActivity } from './stores/useActivity'
import { useJobs } from './stores/useJobs'
import { kind } from './lib/itemPresentation'

// The dev strip is a Vite dev build's own chrome and never ships. The developer
// tools behind it can be turned on in a shipped build (ADR
// developer-tools-are-reachable-in-a-shipped-build-behind-a-setting), so their
// chunk is always defined and fetched only when the pane opens.
const devMode = import.meta.env.DEV
const DevBar = devMode ? defineAsyncComponent(() => import('./components/DevBar.vue')) : null
const DevView = defineAsyncComponent(() => import('./components/DevView.vue'))
// Async so xterm.js stays out of the initial bundle: the hub must not pay for
// a terminal at startup.
const TerminalMode = defineAsyncComponent(() => import('./components/TerminalMode.vue'))
const AgentsMode = defineAsyncComponent(() => import('./components/AgentsMode.vue'))
const PopupTerminal = defineAsyncComponent(() => import('./components/PopupTerminal.vue'))

const feed = useFeedState()
const { profiles, profilesLoaded, profilesError, activeProfile, activeProfileId, items } = feed
// The flows session is app-lived: the Go engine keeps every flow running with
// the canvas closed, and the sidebar reads the flow listing either way.
const session = useFlowsSession()
const github = useGitHubConnection()
const confirmation = useConfirmation()

const nav = useAppNavigation(feed, session)
const { router, flowsActive, devActive, profileSettingsActive } = nav
const profileActions = useProfileActions(feed, nav, confirmation, github)
const onboarding = useOnboarding(feed, github, router)
const { active: onboardingActive, screen: onboardingScreen } = onboarding
const shellLoaded = computed(() => (profilesLoaded.value || !!profilesError.value) && onboarding.loaded.value)
const appMode = useAppMode(feed, nav, onboardingActive, shellLoaded)
const { mode, terminalActive, agentsActive, hubActive, settingsActive, terminalMounted, agentsMounted } = appMode
const { terminalSidebarCollapsed, agentsSidebarCollapsed, feedSidebarCollapsed, previewCollapsed } = appMode
const overlays = useHubOverlays(terminalActive, appMode.viewCanvasScope)
const { tasksOpen, activityOpen, canvasOpen, actionRunsOpen, terminalSessionRepoKey } = overlays
const update = useSelfUpdate(confirmation, feed.showToast)
const reveal = useReveal(nav, session, feed.showToast)
const feedCommands = useFeedCommands(feed, confirmation)
const itemDetail = useSelectedItemDetail(feed)

const feedHub = useTemplateRef<{ focusSearch: () => void }>('feedHub')
const { runCommand, contextActive, popupTerminalMounted } = useAppCommands({
  feed,
  nav,
  appMode,
  overlays,
  feedCommands,
  onboardingActive,
  shellLoaded,
  openNewProfile: profileActions.openNewProfile,
  focusFeedSearch: () => feedHub.value?.focusSearch(),
})
const palette = useCommandPalette()
useGlobalKeymap({
  runCommand,
  contextActive,
  paletteOpen: palette.open,
  activityOpen,
  tasksOpen,
  canvasOpen,
  actionRunsOpen,
})

const { unseenCount: unseenActivity } = useActivity()
const { activeJobs, hasActive: jobsActive } = useJobs()

// What the actions editor autocompletes "applies to" against. kind() never
// returns empty, so untyped items are offered as a target like any other.
const knownFeedTypes = computed(() =>
  [...new Set(items.value.map((item) => kind(item)))].sort((a, b) => a.localeCompare(b)),
)

// The engine commits before it announces, so this is when inbox items are
// readable; log:appended may route nowhere at all.
useWailsEvent('inbox:updated', () => {
  void feed.refresh()
})
// The session keeps an unsaved editor draft private while refreshing the rest.
useWailsEvent('flows:updated', () => {
  void session.reloadFlows()
})
</script>

<template>
  <main class="h-screen w-screen overflow-hidden bg-app text-text">
    <div class="flex h-full min-h-0 flex-col overflow-hidden">
      <TitleBar
        :profile-name="onboardingActive ? undefined : (activeProfile?.name ?? 'Loading')"
        :mode="mode"
        :activity-active="activityOpen"
        :action-runs-active="actionRunsOpen"
        :error-count="reveal.errorCount.value"
        :unseen-activity="unseenActivity"
        :jobs-active="jobsActive"
        :active-jobs="activeJobs"
        :update-available="update.available.value"
        :update-installing="update.installing.value"
        :latest-version="update.latestVersion.value"
        :can-go-back="nav.canGoBack.value"
        :can-go-forward="nav.canGoForward.value"
        :sidebar-collapsed="appMode.sidebarCollapsed.value"
        :can-toggle-sidebar="appMode.canToggleSidebar.value"
        :preview-collapsed="appMode.previewToggleCollapsed.value"
        :can-toggle-preview="appMode.canTogglePreview.value"
        :preview-unseen="appMode.previewUnseen.value"
        @set-mode="appMode.setMode"
        @back="router.back()"
        @forward="router.forward()"
        @open-error-node="reveal.openErrorNode"
        @open-activity="overlays.toggleActivity"
        @open-job-run="nav.openJobRun"
        @open-job-log="overlays.openActionRuns"
        @run-command="runCommand"
        @open-update="update.openUpdate"
        @toggle-sidebar="appMode.toggleSidebar"
        @toggle-preview="appMode.togglePreview"
        @open-palette="palette.toggle()"
        @toggle-maximise="appMode.toggleMaximise"
      />
      <!-- An empty frame until the profiles resolve, so a returning user never
           sees onboarding flash by. A load failure falls through to the shell,
           which renders the error with a retry. -->
      <div v-if="!shellLoaded" class="flex min-h-0 flex-1 items-center justify-center font-mono text-xs text-text-4">
        Loading…
      </div>
      <OnboardingScreen v-else-if="onboardingActive" v-bind="onboardingScreen" v-on="onboarding.screenEvents" />
      <!-- v-show, not a branch of the chain above: leaving a mode hides it and
           never unmounts it (see useAppMode). -->
      <TerminalMode
        v-if="terminalMounted"
        v-show="terminalActive"
        :active="terminalActive"
        :sidebar-collapsed="terminalSidebarCollapsed"
        :canvas="appMode.codeCanvas.value"
        @open-canvas-page="overlays.openCanvas"
        @open-tasks="overlays.toggleTasks"
        @session-repo-key="terminalSessionRepoKey = $event"
      />
      <AgentsMode
        v-if="agentsMounted"
        v-show="agentsActive"
        :active="agentsActive"
        :sidebar-collapsed="agentsSidebarCollapsed"
        @open-canvas-page="overlays.openCanvas"
      />
      <SettingsView
        v-if="settingsActive"
        :active-category="nav.applicationSettingsSection.value"
        :known-feed-types="knownFeedTypes"
        @close="appMode.closeSettings"
        @select-category="nav.selectApplicationSettingsSection"
      />
      <!-- The profile rail stays mounted across the feed, flows, and profile
           settings pages, so the flows canvas never strands the user. -->
      <div v-if="hubActive" class="flex min-h-0 flex-1">
        <ProfileRail
          :profiles="profiles"
          :active-profile-id="activeProfileId"
          @select="nav.requestSelectProfile"
          @add="profileActions.openNewProfile"
          @reorder="feed.reorderProfiles"
        />
        <DevView v-if="devActive" @close="nav.openFeed()" />
        <ProfileSettingsView
          v-else-if="profileSettingsActive && activeProfile"
          :profile="activeProfile"
          :active-section="nav.profileSettingsSection.value"
          :renaming="feed.renamingProfile.value"
          :rename-error="feed.renameProfileError.value"
          :toggling="feed.togglingProfileId.value !== null"
          :toggle-error="feed.toggleProfileError.value"
          :setting-image="feed.settingProfileImage.value"
          :image-error="feed.profileImageError.value"
          @close="nav.openFeed()"
          @rename="profileActions.renameProfile"
          @toggle-enabled="profileActions.setProfileEnabled"
          @set-image="profileActions.setProfileImage"
          @clear-image="profileActions.clearProfileImage"
          @delete="profileActions.openDeleteProfile"
          @select-section="nav.selectProfileSettingsSection"
        />
        <FlowsView v-else-if="flowsActive" />
        <FeedHub
          v-else
          ref="feedHub"
          v-model:preview-collapsed="previewCollapsed"
          :feed="feed"
          :nav="nav"
          :feed-commands="feedCommands"
          :detail="itemDetail"
          :sidebar-collapsed="feedSidebarCollapsed"
          :github-connected="github.connected.value"
          @open-run-log="overlays.openActionRuns"
        />
      </div>
      <DevBar v-if="devMode" />
      <SequenceHint />
    </div>
    <AppDialogs
      :feed="feed"
      :nav="nav"
      :confirmation="confirmation"
      :overlays="overlays"
      :profile-actions="profileActions"
      :feed-commands="feedCommands"
      :open-activity-item="reveal.openActivityItem"
    />
    <PopupTerminal v-if="popupTerminalMounted" />
  </main>
</template>
