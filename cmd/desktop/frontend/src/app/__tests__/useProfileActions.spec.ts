import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'
import { useConfirmation } from '../../composables/useConfirmation'
import type { AppNavigation, FeedState } from '../useAppNavigation'
import { useProfileActions } from '../useProfileActions'

function fakeFeed() {
  return {
    activeProfileId: ref('work'),
    activeProfile: ref<{ id: string; name: string } | null>({ id: 'work', name: 'Work' }),
    createProfileError: ref<string | null>(null),
    createProfile: vi.fn().mockResolvedValue(undefined),
    renameProfile: vi.fn().mockResolvedValue(undefined),
    setProfileEnabled: vi.fn().mockResolvedValue(undefined),
    setProfileImage: vi.fn().mockResolvedValue(undefined),
    clearProfileImage: vi.fn().mockResolvedValue(undefined),
    deleteProfile: vi.fn().mockResolvedValue(true),
    loadProfiles: vi.fn().mockResolvedValue(undefined),
  }
}

let feed: ReturnType<typeof fakeFeed>
const nav = { openFeed: vi.fn() }
const github = { connected: ref(false), status: ref<{ login: string } | null>(null) }
let confirmation: ReturnType<typeof useConfirmation>
let actions: ReturnType<typeof useProfileActions>

beforeEach(() => {
  vi.clearAllMocks()
  feed = fakeFeed()
  github.connected.value = false
  github.status.value = null
  confirmation = useConfirmation()
  actions = useProfileActions(
    feed as unknown as FeedState,
    nav as unknown as AppNavigation,
    confirmation,
    github as unknown as Parameters<typeof useProfileActions>[3],
  )
})

describe('useProfileActions', () => {
  it('clears a stale create error on reopen and closes only on success', async () => {
    feed.createProfileError.value = 'taken'
    actions.openNewProfile()
    expect(feed.createProfileError.value).toBeNull()
    expect(actions.newProfileOpen.value).toBe(true)

    feed.createProfile.mockImplementationOnce(() => {
      feed.createProfileError.value = 'taken'
    })
    await actions.submitNewProfile('Home')
    expect(actions.newProfileOpen.value).toBe(true)

    feed.createProfileError.value = null
    feed.activeProfileId.value = 'home'
    await actions.submitNewProfile('Home')
    expect(actions.newProfileOpen.value).toBe(false)
    expect(nav.openFeed).toHaveBeenCalledWith('home')
  })

  it('applies edits to the active profile and skips them without one', async () => {
    await actions.renameProfile('Office')
    await actions.setProfileEnabled(false)
    expect(feed.renameProfile).toHaveBeenCalledWith('work', 'Office')
    expect(feed.setProfileEnabled).toHaveBeenCalledWith('work', false)

    feed.activeProfileId.value = ''
    await actions.clearProfileImage()
    expect(feed.clearProfileImage).not.toHaveBeenCalled()
  })

  it('deletes behind a confirmation and keeps the dialog up when it fails', async () => {
    actions.openDeleteProfile()
    expect(confirmation.options.value?.testid).toBe('delete-profile-modal')

    feed.deleteProfile.mockResolvedValueOnce(false)
    expect(await confirmation.confirm()).toBe(false)
    expect(confirmation.error.value).toBe('Could not delete the profile.')

    expect(await confirmation.confirm()).toBe(true)
    expect(feed.deleteProfile).toHaveBeenLastCalledWith('work')
    expect(nav.openFeed).toHaveBeenCalledWith()
  })

  it('reloads profiles when the connected account changes', async () => {
    github.status.value = { login: 'octocat' }
    github.connected.value = true
    await nextTick()
    expect(feed.loadProfiles).toHaveBeenCalledOnce()

    github.connected.value = false
    await nextTick()
    expect(feed.loadProfiles).toHaveBeenCalledOnce()
  })
})
