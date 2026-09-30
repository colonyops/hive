import { describe, expect, it } from 'vitest'
import { mount, type DOMWrapper } from '@vue/test-utils'
import ProfileRail from '../ProfileRail.vue'

const profiles = [{
  id: 'personal',
  letter: 'P',
  name: 'Personal',
  enabled: true,
  nodes: 0,
  sourceSummary: '2 sources',
  totalCount: 3,
  unreadCount: 1,
  feeds: [],
}]

// A rail of three, in the alphabetical order the backend serves unconfigured.
const rail = ['hive', 'personal', 'recipinned'].map((id) => ({ ...profiles[0], id, name: id, letter: id[0].toUpperCase() }))

// Drop onto the top or bottom half of a tile: the edge decides whether the
// dragged tile lands before or after it.
function dropOn(tile: DOMWrapper<Element>, half: 'top' | 'bottom') {
  tile.element.getBoundingClientRect = () => ({ top: 0, height: 38 }) as DOMRect
  return tile.trigger('dragover', { clientY: half === 'top' ? 8 : 30 }).then(() => tile.trigger('drop'))
}

describe('ProfileRail', () => {
  it('marks disabled profiles while keeping them selectable', async () => {
    const disabled = { ...profiles[0], enabled: false }
    const wrapper = mount(ProfileRail, { props: { profiles: [disabled], activeProfileId: 'personal' } })
    const tile = wrapper.get('[data-testid="profile-tile"]')

    expect(tile.attributes('data-enabled')).toBe('false')
    expect(tile.attributes('aria-label')).toContain('disabled')
    await tile.trigger('click')
    expect(wrapper.emitted('select')).toEqual([['personal']])
  })

  it('renders the avatar image when set, otherwise the letter chip', () => {
    const withImage = { ...profiles[0], image: 'data:image/png;base64,AAAA' }
    const wrapper = mount(ProfileRail, { props: { profiles: [withImage], activeProfileId: 'personal' } })
    const img = wrapper.get('[data-testid="profile-tile"] img')
    expect(img.attributes('src')).toBe('data:image/png;base64,AAAA')

    const plain = mount(ProfileRail, { props: { profiles, activeProfileId: 'personal' } })
    expect(plain.find('[data-testid="profile-tile"] img').exists()).toBe(false)
    expect(plain.get('[data-testid="profile-tile"]').text()).toContain('P')
  })

  it('emits the whole rail, top first, when a tile is dropped', async () => {
    const wrapper = mount(ProfileRail, { props: { profiles: rail, activeProfileId: 'hive' } })
    const tiles = wrapper.findAll('[data-testid="profile-tile"]')

    await tiles[1].trigger('dragstart', { dataTransfer: new DataTransfer() })
    await dropOn(tiles[0], 'top')

    expect(wrapper.emitted('reorder')).toEqual([[['personal', 'hive', 'recipinned']]])
  })

  it('drops onto the bottom half of a tile to land after it', async () => {
    const wrapper = mount(ProfileRail, { props: { profiles: rail, activeProfileId: 'hive' } })
    const tiles = wrapper.findAll('[data-testid="profile-tile"]')

    await tiles[0].trigger('dragstart', { dataTransfer: new DataTransfer() })
    await dropOn(tiles[2], 'bottom')

    expect(wrapper.emitted('reorder')).toEqual([[['personal', 'recipinned', 'hive']]])
  })

  it('stays quiet when a tile is dropped back where it started', async () => {
    const wrapper = mount(ProfileRail, { props: { profiles: rail, activeProfileId: 'hive' } })
    const tiles = wrapper.findAll('[data-testid="profile-tile"]')

    await tiles[1].trigger('dragstart', { dataTransfer: new DataTransfer() })
    await dropOn(tiles[1], 'top')

    expect(wrapper.emitted('reorder')).toBeUndefined()
  })

  it('moves the focused tile with alt+arrow, and stops at the ends', async () => {
    const wrapper = mount(ProfileRail, { props: { profiles: rail, activeProfileId: 'hive' } })
    const tiles = wrapper.findAll('[data-testid="profile-tile"]')

    await tiles[1].trigger('keydown', { key: 'ArrowUp', altKey: true })
    expect(wrapper.emitted('reorder')).toEqual([[['personal', 'hive', 'recipinned']]])

    await tiles[0].trigger('keydown', { key: 'ArrowUp', altKey: true })
    expect(wrapper.emitted('reorder')).toHaveLength(1)

    await tiles[1].trigger('keydown', { key: 'ArrowUp' })
    expect(wrapper.emitted('reorder')).toHaveLength(1)
  })

  it('shows one application settings action at the bottom', async () => {
    const wrapper = mount(ProfileRail, { props: { profiles, activeProfileId: 'personal' } })

    expect(wrapper.find('[data-testid="application-settings"]').attributes('aria-label')).toBe('Application settings')
    expect(wrapper.text()).not.toContain('hy')

    await wrapper.find('[data-testid="application-settings"]').trigger('click')
    expect(wrapper.emitted('open-settings')).toHaveLength(1)
  })
})
