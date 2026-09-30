import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OnboardingScreen from '../OnboardingScreen.vue'
import { useHiveSetup } from '../../composables/useHiveSetup'

vi.mock('@wailsio/runtime', () => ({
  Browser: {
    OpenURL: vi.fn().mockResolvedValue(undefined),
  },
  Clipboard: { SetText: vi.fn().mockResolvedValue(undefined) },
}))

const hiveMocks = vi.hoisted(() => ({
  Setup: vi.fn(),
  Save: vi.fn(),
  InspectWorkspace: vi.fn(),
  ChooseDirectory: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/hiveconfigservice', () => ({
  Setup: hiveMocks.Setup,
  Save: hiveMocks.Save,
  InspectWorkspace: hiveMocks.InspectWorkspace,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/systemservice', () => ({
  ChooseDirectory: hiveMocks.ChooseDirectory,
}))

const AGENTS = [
  { name: 'claude', label: 'Claude Code', skipPermissionFlags: ['--dangerously-skip-permissions'], installed: false },
  { name: 'opencode', label: 'OpenCode', skipPermissionFlags: ['--agent', 'free-permissions-runner'], installed: true },
]

function hiveSetup(over: Record<string, unknown> = {}) {
  return {
    config: {
      path: '/home/u/.config/hive/config.yaml',
      exists: false,
      usable: false,
      unreadable: '',
      defaultAgent: '',
      profiles: [],
      workspaces: [],
      ...over,
    },
    agents: AGENTS,
    defaultAgentOverride: '',
  }
}

async function loadedHive(setup = hiveSetup()) {
  hiveMocks.Setup.mockResolvedValue(setup)
  const hive = useHiveSetup()
  await hive.load()
  await flushPromises()
  return hive
}

function mountScreen(props: Partial<InstanceType<typeof OnboardingScreen>['$props']> = {}) {
  return mount(OnboardingScreen, {
    props: {
      card: 'idle',
      deviceFlow: null,
      error: null,
      busy: false,
      githubConnected: false,
      ...props,
    },
  })
}

describe('OnboardingScreen', () => {
  // Profile naming is absent because a profile exists before the walk starts.
  it('lists exactly the steps onboarding has', () => {
    const wrapper = mountScreen()
    expect(wrapper.findAll('ol li').map((li) => li.text())).toHaveLength(4)
    expect(wrapper.text()).toContain('Set up your agent and code')
    expect(wrapper.text()).toContain('Connect GitHub')
    expect(wrapper.text()).toContain('Turn on notifications')
    expect(wrapper.text()).toContain('Configure Hive with your agent')
    expect(wrapper.text()).not.toContain('Create your first profile')
    expect(wrapper.text()).toContain('Tokens are stored in your OS keychain.')
  })

  // Hive setup comes first because sessions require an agent and repository
  // folder.
  it('orders the steps and marks the ones already past as done', () => {
    const connecting = mountScreen({ card: 'idle' }).findAll('ol li')
    expect(connecting[0].text()).toContain('Set up your agent and code')
    expect(connecting[1].text()).toContain('Connect GitHub')
    expect(connecting[2].text()).toContain('Turn on notifications')
    expect(connecting[0].text()).toContain('complete')
    expect(connecting[1].text()).toContain('2')
    expect(connecting[1].attributes('aria-current')).toBe('step')

    const agent = mountScreen({ card: 'agent' }).findAll('ol li')
    expect(agent[2].text()).not.toContain('3')
    expect(agent[2].attributes('aria-current')).toBeUndefined()
    expect(agent[2].text()).toContain('complete')
    expect(agent[3].text()).toContain('4')
    expect(agent[3].attributes('aria-current')).toBe('step')
  })

  it('emits startDeviceFlow from the idle card', async () => {
    const wrapper = mountScreen()
    await wrapper.get('[data-testid="onboarding-connect"]').trigger('click')
    expect(wrapper.emitted('startDeviceFlow')).toHaveLength(1)
  })

  it('shows the user code and waiting state on the device card', () => {
    const wrapper = mountScreen({
      card: 'device',
      deviceFlow: { userCode: '7B4C-Q22F', verificationUri: 'https://github.com/login/device' },
    })
    expect(wrapper.get('[data-testid="onboarding-user-code"]').text()).toBe('7B4C-Q22F')
    expect(wrapper.text()).toContain('Waiting for authorization…')
    expect(wrapper.get('[data-testid="onboarding-open-verification"]').text()).toContain('github.com/login/device')
  })

  it('submits a trimmed token and disables submit when empty', async () => {
    const wrapper = mountScreen({ card: 'token' })
    const submit = wrapper.get('[data-testid="onboarding-token-submit"]')
    expect(submit.attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="onboarding-token-input"]').setValue('  ghp_abc  ')
    await submit.trigger('click')
    expect(wrapper.emitted('submitToken')).toEqual([['ghp_abc']])
  })

  it('switches cards via the secondary links', async () => {
    const wrapper = mountScreen()
    await wrapper.get('[data-testid="onboarding-use-token"]').trigger('click')
    expect(wrapper.emitted('useTokenInstead')).toHaveLength(1)

    const tokenCard = mountScreen({ card: 'token' })
    await tokenCard.get('[data-testid="onboarding-back"]').trigger('click')
    expect(tokenCard.emitted('backToStart')).toHaveLength(1)
  })

  it('renders errors on the active card', () => {
    const wrapper = mountScreen({ error: 'Could not reach GitHub to start sign-in.' })
    expect(wrapper.get('[data-testid="onboarding-error"]').text()).toContain('Could not reach GitHub')
  })

  it('ignores Enter on the token card while busy', async () => {
    const wrapper = mountScreen({ card: 'token', busy: true })
    await wrapper.get('[data-testid="onboarding-token-input"]').setValue('ghp_abc')
    await wrapper.get('[data-testid="onboarding-token-input"]').trigger('keydown.enter')
    expect(wrapper.emitted('submitToken')).toBeUndefined()
  })

  // Bypassing is possible, but only past the warning — a feed with no sources
  // is not something to discover later.
  it('warns before skipping the connect step, and can back out of the warning', async () => {
    const wrapper = mountScreen({ card: 'idle' })

    expect(wrapper.get('[data-testid="onboarding-skip"]').text()).toBe('Continue without GitHub')
    await wrapper.get('[data-testid="onboarding-skip"]').trigger('click')
    expect(wrapper.text()).toContain('Skip connecting GitHub?')
    expect(wrapper.text()).toContain('Settings ▸ Integrations')
    expect(wrapper.find('[data-testid="onboarding-connect"]').exists()).toBe(false)
    expect(wrapper.emitted('skipConnect')).toBeUndefined()

    await wrapper.get('[data-testid="onboarding-skip-back"]').trigger('click')
    expect(wrapper.get('[data-testid="onboarding-connect"]').isVisible()).toBe(true)

    await wrapper.get('[data-testid="onboarding-skip"]').trigger('click')
    await wrapper.get('[data-testid="onboarding-skip-confirm"]').trigger('click')
    expect(wrapper.emitted('skipConnect')).toHaveLength(1)
  })

  it('offers the GitHub skip on every connect card and nowhere else', () => {
    for (const card of ['idle', 'device', 'token'] as const) {
      expect(mountScreen({ card }).find('[data-testid="onboarding-skip"]').exists()).toBe(true)
    }
    expect(mountScreen({ card: 'permissions', permission: 'not-requested' }).find('[data-testid="onboarding-skip"]').exists()).toBe(false)
    expect(mountScreen({ card: 'agent' }).find('[data-testid="onboarding-skip"]').exists()).toBe(false)
  })

  it('emits on Enter when not busy and text is present', async () => {
    const wrapper = mountScreen({ card: 'token', busy: false })
    await wrapper.get('[data-testid="onboarding-token-input"]').setValue('ghp_abc')
    await wrapper.get('[data-testid="onboarding-token-input"]').trigger('keydown.enter')
    expect(wrapper.emitted('submitToken')).toEqual([['ghp_abc']])
  })

  // Ask for notification permission here instead of interrupting later use.
  it('marks the notifications step active on the permissions card', () => {
    const steps = mountScreen({ card: 'permissions', permission: 'not-requested' }).findAll('ol li').map((li) => li.text())
    expect(steps[0]).not.toContain('1')
    expect(steps[1]).not.toContain('2')
    expect(steps[2]).toContain('Turn on notifications')
    expect(steps[2]).toContain('3')
    expect(steps[3]).toContain('4')
  })

  it('offers the agent hand-off and an honest way past it', async () => {
    const wrapper = mountScreen({ card: 'agent' })
    expect(wrapper.text()).toContain('Configure Hive with your agent')
    expect(wrapper.text()).toContain('short interview')

    await wrapper.get('[data-testid="onboarding-agent-start"]').trigger('click')
    expect(wrapper.emitted('startAgent')).toHaveLength(1)

    expect(wrapper.get('[data-testid="onboarding-agent-skip"]').text()).toBe('Not now')
    await wrapper.get('[data-testid="onboarding-agent-skip"]').trigger('click')
    expect(wrapper.emitted('finishAgent')).toHaveLength(1)
  })

  it('renders the launch error and progress on the agent card', async () => {
    const wrapper = mountScreen({ card: 'agent', error: 'The Agents area is unavailable.' })
    expect(wrapper.get('[data-testid="onboarding-error"]').text()).toContain('unavailable')

    await wrapper.setProps({ busy: true, error: null })
    const start = wrapper.get('[data-testid="onboarding-agent-start"]')
    expect(start.attributes('disabled')).toBeDefined()
    expect(start.text()).toContain('Starting')
  })

  it('requests permission from the not-requested permissions card, and offers an honest skip', async () => {
    const wrapper = mountScreen({ card: 'permissions', permission: 'not-requested' })
    // The GitHub skip belongs to the connect cards; this card has its own.
    expect(wrapper.find('[data-testid="onboarding-skip"]').exists()).toBe(false)

    await wrapper.get('[data-testid="onboarding-permissions-allow"]').trigger('click')
    expect(wrapper.emitted('requestPermission')).toHaveLength(1)

    expect(wrapper.text()).toContain('Activity')
    expect(wrapper.get('[data-testid="onboarding-permissions-skip"]').text()).toBe('Not now')
    await wrapper.get('[data-testid="onboarding-permissions-skip"]').trigger('click')
    expect(wrapper.emitted('finishPermissions')).toHaveLength(1)
  })

  it('disables the allow button and shows progress while requesting', () => {
    const wrapper = mountScreen({ card: 'permissions', permission: 'not-requested', busy: true })
    const allow = wrapper.get('[data-testid="onboarding-permissions-allow"]')
    expect(allow.attributes('disabled')).toBeDefined()
    expect(allow.text()).toContain('Requesting')
  })

  it('confirms the grant and finishes from the granted permissions card', async () => {
    const wrapper = mountScreen({ card: 'permissions', permission: 'granted' })
    expect(wrapper.find('[data-testid="onboarding-permissions-allow"]').exists()).toBe(false)
    await wrapper.get('[data-testid="onboarding-permissions-finish"]').trigger('click')
    expect(wrapper.emitted('finishPermissions')).toHaveLength(1)
  })

  it('shows guidance instead of a dead button when permission is denied', async () => {
    const wrapper = mountScreen({ card: 'permissions', permission: 'denied' })
    expect(wrapper.find('[data-testid="onboarding-permissions-allow"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="onboarding-permissions-denied-guidance"]').text()).toContain('blocked')
    await wrapper.get('[data-testid="onboarding-permissions-finish"]').trigger('click')
    expect(wrapper.emitted('finishPermissions')).toHaveLength(1)
  })
})

// The Hive step is first run's answer to "which agent, and where is my code" —
// the two values the session launcher is built from. Without them the new
// session picker has nothing to offer, which is the state this step exists to
// prevent.
describe('OnboardingScreen — Hive setup', () => {
  it('starts on the agent this machine actually has', async () => {
    const hive = await loadedHive()
    const wrapper = mountScreen({ card: 'hive', hive })

    expect(wrapper.get('[data-testid="hive-agent-opencode"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-testid="hive-agent-claude"]').attributes('aria-pressed')).toBe('false')
    expect(hive.defaultAgent.value).toBe('opencode')
  })

  it('cannot be submitted until a folder is chosen', async () => {
    const hive = await loadedHive()
    hiveMocks.ChooseDirectory.mockResolvedValue('/home/u/code')
    hiveMocks.InspectWorkspace.mockResolvedValue({ path: '/home/u/code', exists: true, repos: 7 })
    const wrapper = mountScreen({ card: 'hive', hive })

    expect(wrapper.get('[data-testid="onboarding-hive-submit"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="hive-add-workspace"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="hive-workspace-list"]').text()).toContain('7 repositories')
    expect(wrapper.get('[data-testid="onboarding-hive-submit"]').attributes('disabled')).toBeUndefined()
    await wrapper.get('[data-testid="onboarding-hive-submit"]').trigger('click')
    expect(wrapper.emitted('saveHive')).toHaveLength(1)
  })

  // Skipping is a real option: the inbox half of the app works without any of
  // this, and a first run that cannot be got past is worse than an empty
  // session picker.
  it('offers a skip that says what skipping costs', async () => {
    const hive = await loadedHive()
    const wrapper = mountScreen({ card: 'hive', hive })

    expect(wrapper.text()).toContain('the inbox works without this')
    await wrapper.get('[data-testid="onboarding-hive-skip"]').trigger('click')
    expect(wrapper.emitted('finishHive')).toHaveLength(1)
  })

  it('shows an existing config to confirm instead of a form to fill in', async () => {
    const hive = await loadedHive(hiveSetup({
      exists: true,
      usable: true,
      defaultAgent: 'opencode',
      profiles: [{ name: 'opencode', command: 'opencode', flags: [] }],
      workspaces: [{ path: '/home/u/code', exists: true, repos: 9 }],
    }))
    const wrapper = mountScreen({ card: 'hive', hive })

    expect(wrapper.text()).toContain('Using your Hive config')
    const existing = wrapper.get('[data-testid="onboarding-hive-existing"]').text()
    expect(existing).toContain('/home/u/.config/hive/config.yaml')
    expect(existing).toContain('opencode')
    expect(existing).toContain('1 folder')
    expect(wrapper.find('[data-testid="hive-add-workspace"]').exists()).toBe(false)

    await wrapper.get('[data-testid="onboarding-hive-continue"]').trigger('click')
    expect(wrapper.emitted('finishHive')).toHaveLength(1)
  })

  it('renders the save error and busy state on the hive card', async () => {
    const hive = await loadedHive()
    hiveMocks.ChooseDirectory.mockResolvedValue('/home/u/code')
    hiveMocks.InspectWorkspace.mockResolvedValue({ path: '/home/u/code', exists: true, repos: 2 })
    const wrapper = mountScreen({ card: 'hive', hive })
    await wrapper.get('[data-testid="hive-add-workspace"]').trigger('click')
    await flushPromises()

    await wrapper.setProps({ error: 'saving the Hive configuration: permission denied' })
    expect(wrapper.get('[data-testid="onboarding-hive-error"]').text()).toContain('permission denied')

    await wrapper.setProps({ busy: true })
    expect(wrapper.get('[data-testid="onboarding-hive-submit"]').text()).toBe('Saving…')
    expect(wrapper.get('[data-testid="onboarding-hive-submit"]').attributes('disabled')).toBeDefined()
  })
})
