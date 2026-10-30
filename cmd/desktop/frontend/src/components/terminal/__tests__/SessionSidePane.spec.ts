import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SessionSidePane from '../SessionSidePane.vue'
import { resetStores } from '../../../stores/defineStore'
import { useRightPaneTab } from '../../../stores/useRightPaneTab'

const drive = { space: 'hay-kot/hive', session: '1' }

function mountPane(props: Record<string, unknown>) {
  return mount(SessionSidePane, {
    props: { drive, gitSession: '1', requested: 'drive', docPath: null, gitPath: null, active: true, ...props },
    global: {
      stubs: {
        DrivePane: { template: '<div data-testid="drive-pane" />' },
        GitPane: { template: '<div data-testid="git-pane" />' },
      },
    },
  })
}

describe('SessionSidePane', () => {
  beforeEach(() => {
    localStorage.clear()
    resetStores()
    vi.clearAllMocks()
  })

  it('offers both tabs and shows the requested one', () => {
    const wrapper = mountPane({ requested: 'git' })
    expect(wrapper.find('[data-testid="session-side-pane-tab-drive"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="git-pane"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="drive-pane"]').exists()).toBe(false)
  })

  it('asks for the other tab rather than switching itself', async () => {
    const wrapper = mountPane({})
    await wrapper.get('[data-testid="session-side-pane-tab-git"]').trigger('click')
    expect(wrapper.emitted('tab')).toEqual([['git']])
    expect(wrapper.find('[data-testid="drive-pane"]').exists()).toBe(true)
  })

  it('shows Git alone for a session with no drive space, even when ?doc asks for Drive', () => {
    const wrapper = mountPane({ drive: null, requested: 'drive' })
    expect(wrapper.find('[data-testid="session-side-pane-tab-drive"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="git-pane"]').exists()).toBe(true)
  })

  it('shows Drive alone for a pinned chat, even when ?git asks for Git', () => {
    const wrapper = mountPane({ gitSession: null, requested: 'git' })
    expect(wrapper.find('[data-testid="session-side-pane-tab-git"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="drive-pane"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="git-pane-layout"]').exists()).toBe(false)
  })

  it('remembers a tab the user could choose', async () => {
    const wrapper = mountPane({ requested: 'git' })
    await flushPromises()
    expect(useRightPaneTab().lastTab.value).toBe('git')

    await wrapper.setProps({ requested: 'drive' })
    expect(useRightPaneTab().lastTab.value).toBe('drive')
  })

  it('does not remember the only tab a session offers', async () => {
    mountPane({ drive: null, requested: 'git' })
    await flushPromises()
    expect(useRightPaneTab().lastTab.value).toBe('drive')
  })

  it('closes through its host', async () => {
    const wrapper = mountPane({})
    await wrapper.get('[data-testid="session-side-pane-close"]').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
