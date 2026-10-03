import { ref, watch } from 'vue'
import type { Confirmation } from '../composables/useConfirmation'
import type { useGitHubConnection } from '../composables/useGitHubConnection'
import type { AppNavigation, FeedState } from './useAppNavigation'

/**
 * Profile create, edit, and delete, as the rail and the profile settings page
 * request them.
 */
export function useProfileActions(
  feed: FeedState,
  nav: AppNavigation,
  confirmation: Confirmation,
  github: ReturnType<typeof useGitHubConnection>,
) {
  const { activeProfileId, activeProfile, createProfileError } = feed
  const newProfileOpen = ref(false)

  function openNewProfile(): void {
    createProfileError.value = null // a stale failure must not greet the reopen
    newProfileOpen.value = true
  }

  async function submitNewProfile(name: string): Promise<void> {
    await feed.createProfile(name)
    if (!createProfileError.value) {
      newProfileOpen.value = false
      nav.openFeed(activeProfileId.value)
    }
  }

  function forActiveProfile<A extends unknown[]>(update: (id: string, ...args: A) => Promise<unknown>) {
    return async (...args: A): Promise<void> => {
      if (activeProfileId.value) await update(activeProfileId.value, ...args)
    }
  }

  function openDeleteProfile(): void {
    const profile = activeProfile.value
    if (!profile) return
    confirmation.request({
      title: 'Delete profile',
      description: `Delete ${profile.name}? Its flow file and committed feed rows are removed; other profiles keep their own source nodes.`,
      confirmLabel: 'Delete profile',
      testid: 'delete-profile-modal',
      confirmTestid: 'delete-profile-confirm',
      onConfirm: async () => {
        if (!(await feed.deleteProfile(profile.id))) throw new Error('Could not delete the profile.')
        nav.openFeed()
      },
    })
  }

  // Profiles load at startup whether or not GitHub is connected. This reload
  // is about identity: a different account must never see the previous
  // account's data.
  watch(
    () => (github.connected.value ? (github.status.value?.login ?? '') : null),
    (key) => {
      if (key !== null) void feed.loadProfiles()
    },
  )

  return {
    newProfileOpen,
    openNewProfile,
    submitNewProfile,
    renameProfile: forActiveProfile(feed.renameProfile),
    setProfileEnabled: forActiveProfile(feed.setProfileEnabled),
    setProfileImage: forActiveProfile(feed.setProfileImage),
    clearProfileImage: forActiveProfile(feed.clearProfileImage),
    openDeleteProfile,
  }
}
