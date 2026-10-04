<script setup lang="ts">
import { onMounted } from 'vue'
import IconLayoutGrid from '~icons/lucide/layout-grid'
import ActionInputsDialog from '../components/ActionInputsDialog.vue'
import ActivityView from '../components/ActivityView.vue'
import AgentCanvasPage from '../components/AgentCanvasPage.vue'
import CommandPalette from '../components/CommandPalette.vue'
import ErrorDialog from '../components/ErrorDialog.vue'
import ReportProblemDialog from '../components/ReportProblemDialog.vue'
import SessionLaunchDialog from '../components/SessionLaunchDialog.vue'
import TasksView from '../components/TasksView.vue'
import ToastStack from '../components/ToastStack.vue'
import UnsavedFlowChangesModal from '../components/UnsavedFlowChangesModal.vue'
import WhatsNewDialog from '../components/WhatsNewDialog.vue'
import ConfirmationHost from '../components/ui/ConfirmationHost.vue'
import HubOverlay from '../components/ui/HubOverlay.vue'
import RenameDialog from '../components/ui/RenameDialog.vue'
import type { Confirmation } from '../composables/useConfirmation'
import { useErrorDialog } from '../composables/useErrorDialog'
import { useNewSession } from '../composables/useNewSession'
import { useReleaseNotes } from '../composables/useReleaseNotes'
import { useReportDialog } from '../composables/useReportDialog'
import { useWailsEvent } from '../composables/useWailsEvent'
import type { ActivityItemLink } from '../lib/activityPresentation'
import { useFlowsSession } from '../pipeline/composables/useFlowsSession'
import type { AppNavigation, FeedState } from './useAppNavigation'
import type { useFeedCommands } from './useFeedCommands'
import type { useHubOverlays } from './useHubOverlays'
import type { useProfileActions } from './useProfileActions'

const props = defineProps<{
  feed: FeedState
  nav: AppNavigation
  confirmation: Confirmation
  overlays: ReturnType<typeof useHubOverlays>
  profileActions: ReturnType<typeof useProfileActions>
  feedCommands: ReturnType<typeof useFeedCommands>
  openActivityItem: (link: ActivityItemLink) => Promise<void>
}>()

// The composables' result objects keep their identity for App's lifetime.
// eslint-disable-next-line vue/no-setup-props-reactivity-loss
const { feed, nav, overlays, profileActions, feedCommands } = props
const {
  sessionLaunchAction,
  sessionLaunchOptions,
  sessionLaunchBusy,
  sessionLaunchError,
  actionInputsAction,
  actionInputsBusy,
  actionInputsError,
  actionRerun,
  toasts,
  creatingProfile,
  createProfileError,
} = feed
const { pendingNavigation, unsavedChangesBusy } = nav
const { tasksOpen, activityOpen, canvasOpen, canvasScope } = overlays
const { newProfileOpen } = profileActions
const flowsSession = useFlowsSession()
const { open: reportDialogOpen } = useReportDialog()
const { current: appError, dismissError } = useErrorDialog()

const {
  open: newSessionOpen,
  options: newSessionOptions,
  initial: newSessionInitial,
  initialTarget: newSessionInitialTarget,
  busy: newSessionBusy,
  error: newSessionError,
  failure: newSessionFailure,
  formKey: newSessionFormKey,
  dismissFailure: dismissNewSessionFailure,
  onCreateFailed: onNewSessionFailed,
} = useNewSession()
// The dialog closed on submit, so the failure has to come to the user.
useWailsEvent('sessions:create-failed', () => {
  void onNewSessionFailed()
})

// An update installs by relaunching, so the version bump is observable only on
// the next launch.
const {
  dialogOpen: whatsNewOpen,
  pendingEntries: whatsNewEntries,
  pendingVersion: whatsNewVersion,
  checkOnLaunch: checkReleaseNotes,
  dismiss: dismissWhatsNew,
} = useReleaseNotes()
onMounted(() => {
  void checkReleaseNotes()
})
</script>

<template>
  <SessionLaunchDialog
    v-if="sessionLaunchAction && sessionLaunchOptions"
    :action="sessionLaunchAction"
    :options="sessionLaunchOptions"
    :initial="{ agent: sessionLaunchOptions.defaultAgent }"
    :busy="sessionLaunchBusy"
    :error="sessionLaunchError"
    testid="create-session"
    field-testid="session"
    @close="feed.cancelSessionLaunch"
    @submit="feed.submitSessionLaunch"
  />
  <ActionInputsDialog
    v-if="actionInputsAction"
    :action-label="actionInputsAction.label"
    :inputs="actionInputsAction.inputs ?? []"
    :busy="actionInputsBusy"
    :error="actionInputsError"
    :submit-label="actionInputsAction.type === 'clipboard' ? 'Copy' : 'Run'"
    @close="feed.cancelActionInputs"
    @submit="feed.submitActionInputs"
  />
  <SessionLaunchDialog
    v-if="newSessionOpen && newSessionOptions"
    :key="newSessionFormKey"
    :options="newSessionOptions"
    :initial="newSessionInitial"
    :initial-target="newSessionInitialTarget"
    with-prompt
    :busy="newSessionBusy"
    :error="newSessionError"
    :failure="newSessionFailure"
    @close="feedCommands.cancelNewSession"
    @submit="feedCommands.submitNewSession"
    @dismiss-failure="dismissNewSessionFailure"
  />
  <ConfirmationHost :confirmation="confirmation" />
  <ConfirmationHost :confirmation="actionRerun" />
  <ToastStack :toasts="toasts" @dismiss="feed.dismissToast" @clear-all="feed.clearToasts" />
  <CommandPalette />
  <ReportProblemDialog v-if="reportDialogOpen" @close="reportDialogOpen = false" />
  <ErrorDialog v-if="appError" :error="appError" @close="dismissError" />
  <WhatsNewDialog v-if="whatsNewOpen" :version="whatsNewVersion" :entries="whatsNewEntries" @close="dismissWhatsNew" />
  <RenameDialog
    v-if="newProfileOpen"
    title="New profile"
    :icon="IconLayoutGrid"
    label="Profile name"
    hint="Saved as a flow in flows/ with the default feeds: your open PRs, the notifications inbox, and cross-repo assignments."
    confirm-label="Create profile"
    :busy="creatingProfile"
    :error="createProfileError"
    testid="new-profile"
    :testids="{ modal: 'new-profile-modal', save: 'new-profile-submit' }"
    @close="newProfileOpen = false"
    @save="profileActions.submitNewProfile"
  />
  <HubOverlay v-if="tasksOpen" label="Tasks" testid="tasks-overlay" @close="tasksOpen = false">
    <TasksView @close="tasksOpen = false" />
  </HubOverlay>
  <HubOverlay v-if="canvasOpen" label="Canvases" testid="canvas-overlay" @close="canvasOpen = false">
    <AgentCanvasPage v-model:scope="canvasScope" @close="canvasOpen = false" @open-url="feed.openUrl" />
  </HubOverlay>
  <HubOverlay v-if="activityOpen" label="Activity" testid="activity-overlay" @close="activityOpen = false">
    <ActivityView
      @close="activityOpen = false"
      @open-url="feed.openUrl"
      @open-item="
        (item) => {
          openActivityItem(item)
          activityOpen = false
        }
      "
    />
  </HubOverlay>
  <!-- Deploying from this modal can raise the error dialog. BaseModal closes on
       any Escape, so stacked, both would take one keypress and drop the guard
       with the error; dismissing the error brings the guard back. -->
  <UnsavedFlowChangesModal
    v-if="pendingNavigation && !appError"
    :busy="unsavedChangesBusy"
    :error="flowsSession.error.value"
    @close="nav.cancelPendingNavigation"
    @deploy="nav.deployPendingNavigation"
    @discard="nav.discardPendingNavigation"
  />
</template>
