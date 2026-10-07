import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import type { Router } from 'vue-router'
import type { FeedState } from '../useAppNavigation'

// The factories close over the fakes below; they are read only once a test
// calls setup(), after the module has initialized them.
vi.mock('../../composables/useHiveSetup', () => ({ useHiveSetup: () => hive }))
vi.mock('../../composables/useFirstRun', () => ({ useFirstRun: () => firstRun }))
vi.mock('../../composables/useNotificationSettings', () => ({ useNotificationSettings: () => notifications }))
vi.mock('../../stores/useAgentWorkspaces', () => ({ useAgentWorkspaces: () => ({ startFirstRunChat }) }))
vi.mock('../../stores/useHiveCommand', () => ({ useHiveCommand: () => command }))

import { useOnboarding } from '../useOnboarding'

const hive = {
  setup: ref<object | null>({}),
  unreadable: ref(''),
  loaded: ref(true),
  error: ref(''),
  saving: ref(false),
  load: vi.fn(),
  save: vi.fn(),
}
const command = {
  status: ref<{ asked: boolean; unsupported: string } | null>({ asked: true, unsupported: '' }),
  loaded: ref(true),
  error: ref<string | null>(null),
  saving: ref(false),
  setInstall: vi.fn(),
}
const firstRun = { completed: ref<boolean | null>(false), load: vi.fn(), complete: vi.fn() }
const notifications = {
  permission: ref('not-requested'),
  error: ref(''),
  requestingPermission: ref(false),
  requestPermission: vi.fn(),
}
const github = {
  status: ref<object | null>({ state: 'disconnected' }),
  connected: ref(false),
  card: ref('idle'),
  error: ref<string | null>(null),
  busy: ref(false),
  deviceFlow: ref(null),
  startDeviceFlow: vi.fn(),
  useTokenInstead: vi.fn(),
  backToStart: vi.fn(),
  submitToken: vi.fn(),
}
const feed = {
  profiles: ref([{ id: 'default', nodes: 0 }]),
  activeProfileId: ref('default'),
  seedStarterFlow: vi.fn(),
  showToast: vi.fn(),
}
const router = { push: vi.fn() }
const startFirstRunChat = vi.fn()

function setup() {
  let onboarding!: ReturnType<typeof useOnboarding>
  mount(
    defineComponent({
      setup() {
        onboarding = useOnboarding(
          feed as unknown as FeedState,
          github as unknown as Parameters<typeof useOnboarding>[1],
          router as unknown as Router,
        )
        return () => null
      },
    }),
  )
  return onboarding
}

beforeEach(() => {
  vi.clearAllMocks()
  hive.setup.value = {}
  hive.unreadable.value = ''
  hive.loaded.value = true
  command.status.value = { asked: true, unsupported: '' }
  command.loaded.value = true
  command.setInstall.mockResolvedValue(true)
  firstRun.completed.value = false
  firstRun.complete.mockImplementation(() => {
    firstRun.completed.value = true
  })
  notifications.permission.value = 'not-requested'
  github.status.value = { state: 'disconnected' }
  github.connected.value = false
  github.card.value = 'idle'
  feed.profiles.value = [{ id: 'default', nodes: 0 }]
  startFirstRunChat.mockResolvedValue({ workspace: 'hive', id: 4 })
})

describe('useOnboarding', () => {
  it('walks hive setup, connect, permissions, then the agent hand-off', async () => {
    const onboarding = setup()
    expect(hive.load).toHaveBeenCalledOnce()
    expect(firstRun.load).toHaveBeenCalledOnce()
    expect(onboarding.active.value).toBe(true)
    expect(onboarding.screen.value.card).toBe('hive')

    hive.save.mockResolvedValue(true)
    await onboarding.screenEvents.saveHive()
    expect(onboarding.screen.value.card).toBe('idle')

    onboarding.screenEvents.skipConnect()
    expect(onboarding.screen.value.card).toBe('permissions')

    onboarding.screenEvents.finishPermissions()
    expect(onboarding.screen.value.card).toBe('agent')

    await onboarding.screenEvents.startAgent()
    expect(firstRun.complete).toHaveBeenCalledOnce()
    expect(router.push).toHaveBeenCalledWith({ name: 'agents', params: { workspace: 'hive' }, query: { chat: '4' } })
    expect(onboarding.active.value).toBe(false)
  })

  it('asks about the hive command after hive setup when no choice is recorded', async () => {
    command.status.value = { asked: false, unsupported: '' }
    const onboarding = setup()

    hive.save.mockResolvedValue(true)
    await onboarding.screenEvents.saveHive()
    expect(onboarding.screen.value.card).toBe('command')

    await onboarding.screenEvents.saveCommand(false)
    expect(command.setInstall).toHaveBeenCalledWith(false)
    expect(onboarding.screen.value.card).toBe('idle')
  })

  it('keeps the command step up when the choice cannot be saved', async () => {
    command.status.value = { asked: false, unsupported: '' }
    command.setInstall.mockResolvedValue(false)
    hive.setup.value = null
    const onboarding = setup()
    expect(onboarding.screen.value.card).toBe('command')

    await onboarding.screenEvents.saveCommand(true)
    expect(onboarding.screen.value.card).toBe('command')
  })

  it('skips the command step in a build that cannot install it', () => {
    command.status.value = { asked: false, unsupported: 'This is a development build.' }
    hive.setup.value = null
    expect(setup().screen.value.card).toBe('idle')
  })

  it('skips the steps whose answer is already known', () => {
    hive.setup.value = null
    github.connected.value = true
    notifications.permission.value = 'granted'

    const onboarding = setup()

    expect(onboarding.screen.value.card).toBe('agent')
  })

  it('waits for the GitHub status before putting a step up', async () => {
    hive.setup.value = null
    github.status.value = null
    const onboarding = setup()
    expect(onboarding.active.value).toBe(false)

    github.status.value = { state: 'connected' }
    github.connected.value = true
    await nextTick()
    expect(onboarding.screen.value.card).toBe('permissions')
  })

  it('stays down once first run is complete', () => {
    firstRun.completed.value = true
    expect(setup().active.value).toBe(false)
  })

  it('seeds an empty profile when the account connects, and reports a failed seed', async () => {
    hive.setup.value = null
    feed.seedStarterFlow.mockRejectedValue(new Error('no flows dir'))
    vi.spyOn(console, 'warn').mockImplementation(() => {})
    const onboarding = setup()
    expect(onboarding.screen.value.card).toBe('idle')

    github.connected.value = true
    await flushPromises()

    expect(feed.seedStarterFlow).toHaveBeenCalledWith('default')
    expect(feed.showToast).toHaveBeenCalledWith('Starter feeds were not added', expect.anything())
    expect(onboarding.screen.value.card).toBe('permissions')
  })

  it('keeps the agent step up with an error when the chat cannot start', async () => {
    hive.setup.value = null
    github.connected.value = true
    notifications.permission.value = 'denied'
    startFirstRunChat.mockRejectedValue(new Error('no agent'))
    const onboarding = setup()

    await onboarding.screenEvents.startAgent()

    expect(onboarding.screen.value).toMatchObject({ card: 'agent', error: 'no agent', busy: false })
    expect(firstRun.complete).not.toHaveBeenCalled()
  })
})
