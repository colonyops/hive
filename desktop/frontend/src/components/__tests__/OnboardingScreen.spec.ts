import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import OnboardingScreen from '../OnboardingScreen.vue'

vi.mock('@wailsio/runtime', () => ({
  Browser: {
    OpenURL: vi.fn().mockResolvedValue(undefined),
  },
}))

function mountScreen(props: Partial<InstanceType<typeof OnboardingScreen>['$props']> = {}) {
  return mount(OnboardingScreen, {
    props: {
      card: 'idle',
      deviceFlow: null,
      error: null,
      busy: false,
      ...props,
    },
  })
}

describe('OnboardingScreen', () => {
  it('renders the three onboarding steps with connect active', () => {
    const wrapper = mountScreen()
    const text = wrapper.text()
    expect(text).toContain('Connect GitHub')
    expect(text).toContain('Create your first workspace')
    expect(text).toContain('Add feeds & tasks')
    expect(text).toContain('Tokens are stored in your OS keychain.')
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

  it('ignores Enter on the workspace card while busy', async () => {
    const wrapper = mountScreen({ card: 'workspace', busy: true })
    await wrapper.get('[data-testid="onboarding-workspace-input"]').setValue('Frontend Triage')
    await wrapper.get('[data-testid="onboarding-workspace-input"]').trigger('keydown.enter')
    expect(wrapper.emitted('createWorkspace')).toBeUndefined()
  })

  it('ignores Enter on the token card while busy', async () => {
    const wrapper = mountScreen({ card: 'token', busy: true })
    await wrapper.get('[data-testid="onboarding-token-input"]').setValue('ghp_abc')
    await wrapper.get('[data-testid="onboarding-token-input"]').trigger('keydown.enter')
    expect(wrapper.emitted('submitToken')).toBeUndefined()
  })

  it('emits on Enter when not busy and text is present', async () => {
    const wrapper = mountScreen({ card: 'token', busy: false })
    await wrapper.get('[data-testid="onboarding-token-input"]').setValue('ghp_abc')
    await wrapper.get('[data-testid="onboarding-token-input"]').trigger('keydown.enter')
    expect(wrapper.emitted('submitToken')).toEqual([['ghp_abc']])
  })
})
