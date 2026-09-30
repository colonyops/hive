import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import MenuBarSettingsView from '../MenuBarSettingsView.vue'
import AppSelect from '../AppSelect.vue'

const mocks = vi.hoisted(() => ({
  Limits: vi.fn(),
  Pins: vi.fn(),
  SetPins: vi.fn(),
  FeedChoices: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/menubarservice', () => mocks)

const choices = [
  { feed: 'work/reviews', profileName: 'Work', folder: 'Code review', name: 'Reviews' },
  { feed: 'work/mentions', profileName: 'Work', folder: '', name: 'Mentions' },
  { feed: 'oss/issues', profileName: 'OSS', folder: '', name: 'Issues' },
  { feed: 'oss/prs', profileName: 'OSS', folder: 'Deps', name: 'PRs' },
]

beforeEach(() => {
  vi.clearAllMocks()
  mocks.Limits.mockResolvedValue({ maxFeeds: 3, defaultItemLimit: 3, maxItemLimit: 10 })
  mocks.Pins.mockResolvedValue([{ feed: 'work/reviews', limit: 5 }, { feed: 'oss/issues', limit: 3 }])
  mocks.FeedChoices.mockResolvedValue(choices)
  mocks.SetPins.mockResolvedValue(undefined)
})

function selectIn(wrapper: VueWrapper, testid: string) {
  const select = wrapper.findAllComponents(AppSelect).find((component) => component.props('testid') === testid)
  if (!select) throw new Error(`no AppSelect ${testid}`)
  return select
}

describe('MenuBarSettingsView', () => {
  it('lists pins by feed name and offers only unpinned feeds', async () => {
    const wrapper = mount(MenuBarSettingsView)
    await flushPromises()

    expect(wrapper.get('[data-testid="menubar-pin-0"]').text()).toContain('Reviews')
    expect(wrapper.get('[data-testid="menubar-pin-0"]').text()).toContain('Work › Code review')
    expect(wrapper.get('[data-testid="menubar-pin-1"]').text()).toContain('OSS')
    expect(selectIn(wrapper, 'menubar-add').props('options')).toEqual([
      { value: 'work/mentions', label: 'Work › Mentions' },
      { value: 'oss/prs', label: 'OSS › Deps › PRs' },
    ])
  })

  it('pins a feed at the default limit and hides the picker at the cap', async () => {
    const wrapper = mount(MenuBarSettingsView)
    await flushPromises()

    selectIn(wrapper, 'menubar-add').vm.$emit('update:modelValue', 'oss/prs')
    await flushPromises()

    expect(mocks.SetPins).toHaveBeenCalledWith([
      { feed: 'work/reviews', limit: 5 },
      { feed: 'oss/issues', limit: 3 },
      { feed: 'oss/prs', limit: 3 },
    ])
    expect(wrapper.find('[data-testid="menubar-add"]').exists()).toBe(false)
  })

  it('reorders, changes the limit, and unpins', async () => {
    const wrapper = mount(MenuBarSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="menubar-pin-1-up"]').trigger('click')
    await flushPromises()
    expect(mocks.SetPins).toHaveBeenLastCalledWith([{ feed: 'oss/issues', limit: 3 }, { feed: 'work/reviews', limit: 5 }])

    selectIn(wrapper, 'menubar-pin-0-limit').vm.$emit('update:modelValue', '8')
    await flushPromises()
    expect(mocks.SetPins).toHaveBeenLastCalledWith([{ feed: 'oss/issues', limit: 8 }, { feed: 'work/reviews', limit: 5 }])

    await wrapper.get('[data-testid="menubar-pin-0-remove"]').trigger('click')
    await flushPromises()
    expect(mocks.SetPins).toHaveBeenLastCalledWith([{ feed: 'work/reviews', limit: 5 }])
  })

  it('rolls back and reports a rejected save', async () => {
    mocks.SetPins.mockRejectedValue(new Error('menu_bar.feeds allows at most 3 feeds'))
    const wrapper = mount(MenuBarSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="menubar-pin-0-remove"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="menubar-pin-1"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="menubar-settings-error"]').text()).toContain('at most 3 feeds')
  })

  it('names a pin whose feed no longer exists', async () => {
    mocks.Pins.mockResolvedValue([{ feed: 'gone/feed', limit: 5 }])
    const wrapper = mount(MenuBarSettingsView)
    await flushPromises()

    expect(wrapper.get('[data-testid="menubar-pin-0"]').text()).toContain('no longer exists')
  })
})
