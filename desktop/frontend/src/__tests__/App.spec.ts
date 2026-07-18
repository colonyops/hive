import { describe, expect, it, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import App from '../App.vue'
import { useCommandPalette } from '../composables/useCommands'
import type { Profile } from '../types/feed'

const mocks = vi.hoisted(() => ({
  // feedservice
  ActionsFor: vi.fn(),
  Config: vi.fn(),
  ConfigPrompt: vi.fn(),
  CreateFeed: vi.fn(),
  CreateProfile: vi.fn(),
  CreateSource: vi.fn(),
  DeleteFeed: vi.fn(),
  DeleteProfile: vi.fn(),
  FeedDefFor: vi.fn(),
  Items: vi.fn(),
  MarkRead: vi.fn(),
  Profiles: vi.fn(),
  Refresh: vi.fn(),
  Sources: vi.fn(),
  UpdateFeed: vi.fn(),
  // auth service
  Status: vi.fn(),
  StartDeviceFlow: vi.fn(),
  CancelDeviceFlow: vi.fn(),
  SetToken: vi.fn(),
  SignOut: vi.fn(),
  // runtime
  On: vi.fn(),
  Hide: vi.fn(),
}))

vi.mock('../../bindings/github.com/colonyops/hive/desktop/feedservice', () => ({
  ActionsFor: mocks.ActionsFor,
  Config: mocks.Config,
  ConfigPrompt: mocks.ConfigPrompt,
  CreateFeed: mocks.CreateFeed,
  CreateProfile: mocks.CreateProfile,
  CreateSource: mocks.CreateSource,
  DeleteFeed: mocks.DeleteFeed,
  DeleteProfile: mocks.DeleteProfile,
  FeedDefFor: mocks.FeedDefFor,
  Items: mocks.Items,
  MarkRead: mocks.MarkRead,
  Profiles: mocks.Profiles,
  Refresh: mocks.Refresh,
  Sources: mocks.Sources,
  UpdateFeed: mocks.UpdateFeed,
}))

vi.mock('../../bindings/github.com/colonyops/hive/internal/desktop/auth/service', () => ({
  Status: mocks.Status,
  StartDeviceFlow: mocks.StartDeviceFlow,
  CancelDeviceFlow: mocks.CancelDeviceFlow,
  SetToken: mocks.SetToken,
  SignOut: mocks.SignOut,
}))

vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
  Window: { Hide: mocks.Hide },
}))

const profiles: Profile[] = [
  {
    id: 'personal',
    letter: 'P',
    name: 'Personal',
    sourceSummary: '2 sources',
    totalCount: 1,
    unreadCount: 0,
    feeds: [{ id: 'desktop', name: 'Desktop UI', count: 1, newCount: 0 }],
  },
]

async function mountApp() {
  const wrapper = mount(App)
  await flushPromises()
  return wrapper
}

describe('App feed editor wiring', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.Status.mockResolvedValue({ state: 'authenticated', login: 'hay', name: 'Hay', avatarUrl: '', message: '' })
    mocks.Profiles.mockResolvedValue(profiles)
    mocks.Items.mockResolvedValue([])
    mocks.ActionsFor.mockResolvedValue([])
    mocks.Config.mockResolvedValue({ path: '/cfg/profiles.yaml', exists: true, yaml: 'profiles:\n', valid: true, error: '' })
    mocks.Sources.mockResolvedValue([
      { id: 'my-prs', kind: 'search', query: 'is:open is:pr author:@me' },
      { id: 'inbox', kind: 'notifications' },
    ])
    mocks.FeedDefFor.mockResolvedValue({ id: 'desktop', name: 'Desktop UI', sources: ['my-prs'], filters: {} })
    mocks.DeleteFeed.mockResolvedValue(undefined)
    mocks.DeleteProfile.mockResolvedValue(undefined)
    mocks.On.mockReturnValue(() => {})
  })

  it('registers New feed and per-feed Edit feed palette commands', async () => {
    const wrapper = await mountApp()
    const { results, query } = useCommandPalette()
    query.value = ''

    const ids = results.value.map((cmd) => cmd.id)
    expect(ids).toContain('feed:new')
    expect(ids).toContain('feed:edit:desktop')
    expect(results.value.find((cmd) => cmd.id === 'feed:new')?.title).toBe('New feed…')
    expect(results.value.find((cmd) => cmd.id === 'feed:edit:desktop')?.title).toBe('Edit feed: Desktop UI')

    wrapper.unmount()
  })

  it('opens the editor in create mode from the New feed command', async () => {
    const wrapper = await mountApp()
    const { results } = useCommandPalette()

    await results.value.find((cmd) => cmd.id === 'feed:new')?.run()
    await flushPromises()

    expect(document.querySelector('[data-testid="feed-editor-title"]')?.textContent).toBe('New feed')
    // Canned sources are listed for picking.
    expect(document.querySelector('[data-testid="feed-editor-source-my-prs"]')).not.toBeNull()
    expect(mocks.FeedDefFor).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('opens the editor prefilled from the Edit feed command', async () => {
    const wrapper = await mountApp()
    const { results } = useCommandPalette()

    await results.value.find((cmd) => cmd.id === 'feed:edit:desktop')?.run()
    await flushPromises()

    expect(document.querySelector('[data-testid="feed-editor-title"]')?.textContent).toBe('Edit feed')
    expect(mocks.FeedDefFor).toHaveBeenCalledWith('personal', 'desktop')
    expect(document.querySelector<HTMLInputElement>('[data-testid="feed-editor-name"]')?.value).toBe('Desktop UI')

    wrapper.unmount()
  })

  it('opens the editor from the sidebar pencil and saves through UpdateFeed', async () => {
    mocks.UpdateFeed.mockResolvedValue(undefined)
    const wrapper = await mountApp()

    await wrapper.find('[data-testid="sidebar-feed-edit-desktop"]').trigger('click')
    await flushPromises()

    expect(document.querySelector('[data-testid="feed-editor"]')).not.toBeNull()

    document.querySelector<HTMLButtonElement>('[data-testid="feed-editor-save"]')?.click()
    await flushPromises()

    expect(mocks.UpdateFeed).toHaveBeenCalledWith('personal', 'desktop', {
      id: 'desktop',
      name: 'Desktop UI',
      sources: ['my-prs'],
      filters: {},
    })
    expect(document.querySelector('[data-testid="feed-editor"]')).toBeNull() // closed on success

    wrapper.unmount()
  })

  it('deletes a feed from the sidebar editor and closes the drawer', async () => {
    const wrapper = await mountApp()

    await wrapper.find('[data-testid="sidebar-feed-edit-desktop"]').trigger('click')
    await flushPromises()

    document.querySelector<HTMLButtonElement>('[data-testid="feed-editor-delete"]')?.click()
    await flushPromises()
    document.querySelector<HTMLButtonElement>('[data-testid="feed-editor-delete-confirm"]')?.click()
    await flushPromises()

    expect(mocks.DeleteFeed).toHaveBeenCalledWith('personal', 'desktop')
    expect(document.querySelector('[data-testid="feed-editor"]')).toBeNull()
    // ToastStack isn't teleported, unlike the drawer/modals above — assert
    // through the wrapper, not document.querySelector.
    expect(wrapper.find('[data-testid="toast-title"]').text()).toBe('Feed deleted')

    wrapper.unmount()
  })

  it('deletes the active profile through the sidebar trash icon and confirm modal, then falls back to onboarding when none remain', async () => {
    const wrapper = await mountApp()

    await wrapper.find('[data-testid="sidebar-delete-profile"]').trigger('click')
    await flushPromises()

    expect(document.querySelector('[data-testid="delete-profile-modal"]')).not.toBeNull()

    mocks.Profiles.mockResolvedValue([])
    document.querySelector<HTMLButtonElement>('[data-testid="delete-profile-confirm"]')?.click()
    await flushPromises()

    expect(mocks.DeleteProfile).toHaveBeenCalledWith('personal')
    expect(document.querySelector('[data-testid="delete-profile-modal"]')).toBeNull()
    // No profiles left: the same "create your first workspace" step a fresh
    // install starts from, not a bespoke empty state. OnboardingScreen isn't
    // teleported, so assert through the wrapper.
    expect(wrapper.find('[data-testid="onboarding"]').exists()).toBe(true)

    wrapper.unmount()
  })
})
