import { beforeEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SideBar from '../SideBar.vue'
import type { FeedTree, Profile, SidebarNode } from '../../types/feed'

// Collapse state and panel width live in localStorage; isolate every test.
beforeEach(() => localStorage.clear())

const profile: Profile = {
  id: 'personal',
  letter: 'P',
  name: 'Personal',
  sourceSummary: '2 sources',
  totalCount: 3,
  unreadCount: 1,
  feeds: [
    { id: 'desktop', name: 'Desktop UI', count: 2, newCount: 1 },
    { id: 'backend', name: 'Backend', count: 1, newCount: 0 },
  ],
}

function mountSideBar(overrides: Partial<{ flowsDirty: boolean }> = {}) {
  return mount(SideBar, {
    props: { profile, selection: { type: 'view', view: 'inbox' }, ...overrides },
  })
}

describe('SideBar', () => {
  it('has no header Flows pill or delete action; the gear opens profile settings', async () => {
    const wrapper = mountSideBar()

    expect(wrapper.find('[data-testid="sidebar-open-flows"]').exists()).toBe(false)
    expect(wrapper.find('.flow-pill').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-profile-header"]').text()).not.toContain('Flows')
    expect(wrapper.find('[data-testid="sidebar-delete-profile"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-open-settings"]').attributes('aria-label')).toBe('Profile settings')

    await wrapper.find('[data-testid="sidebar-open-settings"]').trigger('click')
    expect(wrapper.emitted('open-settings')).toHaveLength(1)
  })

  it('renders and selects each inbox view', async () => {
    const wrapper = mountSideBar()
    const views = ['inbox', 'open', 'archive', 'all', 'unfiled']
    for (const view of views) {
      const button = wrapper.get(`[data-testid="inbox-view-${view}"]`)
      expect(button.attributes('data-testid')).toBe(`inbox-view-${view}`)
      await button.trigger('click')
    }
    expect(wrapper.emitted('select')).toEqual(views.map((view) => [{ type: 'view', view }]))
    expect(wrapper.get('[data-testid="inbox-view-inbox"]').classes()).toContain('sidebar-entry-selected')
  })

  it('opens the flows canvas from the Edit flow footer', async () => {
    const wrapper = mountSideBar()
    await wrapper.find('[data-testid="sidebar-edit-flow"]').trigger('click')
    expect(wrapper.emitted('open-flows')).toHaveLength(1)
  })

  it('selects a feed when the row is clicked', async () => {
    const wrapper = mountSideBar()

    await wrapper.find('[data-testid="sidebar-feed"][data-id="backend"]').trigger('click')

    expect(wrapper.emitted('select')).toEqual([[{ type: 'feed', feedId: 'backend' }]])
  })


  it('shows the un-deployed changes badge only when flowsDirty is true', () => {
    const clean = mountSideBar({ flowsDirty: false })
    expect(clean.find('[data-testid="undeployed-badge"]').exists()).toBe(false)

    const dirty = mountSideBar({ flowsDirty: true })
    expect(dirty.find('[data-testid="undeployed-badge"]').exists()).toBe(true)
  })

  it('omits the un-deployed changes badge by default (flowsDirty unset)', () => {
    const wrapper = mountSideBar()
    expect(wrapper.find('[data-testid="undeployed-badge"]').exists()).toBe(false)
  })

  it('renders a resize handle that widens the panel on drag and persists the width', async () => {
    const wrapper = mountSideBar()
    const aside = wrapper.get('aside').element as HTMLElement
    expect(aside.style.width).toBe('250px') // default

    const handle = wrapper.get('[data-testid="resize-handle-sidebar"]')
    expect(handle.attributes('role')).toBe('separator')

    await handle.trigger('pointerdown', { clientX: 250, pointerId: 1 })
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 300, pointerId: 1 }))
    await wrapper.vm.$nextTick()

    expect(aside.style.width).toBe('300px')

    window.dispatchEvent(new PointerEvent('pointerup', { clientX: 300, pointerId: 1 }))
    expect(localStorage.getItem('hive.panel.sidebar')).toBe('300')
  })
})

// ── folders + reordering ──────────────────────────────────────────────────────

const desktopFeed = { id: 'desktop', name: 'Desktop UI', count: 2, newCount: 1 }
const backendFeed = { id: 'backend', name: 'Backend', count: 1, newCount: 0 }

const grouped: Profile = {
  ...profile,
  tree: [
    { kind: 'feed', feed: desktopFeed },
    { kind: 'folder', folder: { id: 'work', name: 'Work', feeds: [backendFeed] } },
  ],
}

function mountGrouped() {
  return mount(SideBar, { props: { profile: grouped, selection: { type: 'view', view: 'inbox' } } })
}

// The tree carried by the most recent 'reorder' emit.
function lastReorder(wrapper: ReturnType<typeof mountGrouped>): FeedTree {
  const events = wrapper.emitted('reorder')
  return events![events!.length - 1][0] as FeedTree
}

function folderNamed(tree: FeedTree, id: string) {
  const node = tree.find((n): n is Extract<SidebarNode, { kind: 'folder' }> => n.kind === 'folder' && n.folder.id === id)
  return node?.folder
}

describe('SideBar folders', () => {
  it('renders folders with their nested feeds', () => {
    const wrapper = mountGrouped()
    expect(wrapper.find('[data-testid="sidebar-folder"][data-id="work"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="folder-name"]').text()).toBe('Work')
    // The nested feed renders inside the folder.
    expect(wrapper.find('[data-testid="sidebar-feed"][data-id="backend"]').exists()).toBe(true)
  })

  it('hides a folder\'s feeds when its collapsed state is set in localStorage', () => {
    // Collapse is view state persisted in localStorage (keyed by flow then
    // folder id), not part of the tree/layout.
    localStorage.setItem('hive.sidebar.collapsed', JSON.stringify({ personal: ['work'] }))
    const wrapper = mountGrouped()
    expect(wrapper.find('[data-testid="sidebar-feed"][data-id="backend"]').exists()).toBe(false)
  })

  it('toggles collapse via localStorage on header click, without emitting a reorder', async () => {
    const wrapper = mountGrouped()
    expect(wrapper.find('[data-testid="sidebar-feed"][data-id="backend"]').exists()).toBe(true)

    await wrapper.find('[data-testid="sidebar-folder"][data-id="work"] .folder-header').trigger('click')

    // Feeds hidden, state persisted, and no layout write (collapse is not config).
    expect(wrapper.find('[data-testid="sidebar-feed"][data-id="backend"]').exists()).toBe(false)
    expect(JSON.parse(localStorage.getItem('hive.sidebar.collapsed') ?? '{}')).toEqual({ personal: ['work'] })
    expect(wrapper.emitted('reorder')).toBeUndefined()
  })

  it('appends a new empty folder when the new-folder button is clicked', async () => {
    const wrapper = mountGrouped()
    await wrapper.find('[data-testid="sidebar-new-folder"]').trigger('click')
    const folders = lastReorder(wrapper).filter((n) => n.kind === 'folder')
    expect(folders).toHaveLength(2)
  })

  it('renames a folder, emitting the new name', async () => {
    const wrapper = mountGrouped()
    await wrapper.find('[data-testid="folder-rename"]').trigger('click')
    const input = wrapper.find('[data-testid="folder-rename-input"]')
    expect(input.exists()).toBe(true)
    await input.setValue('Projects')
    await input.trigger('keydown.enter')
    expect(folderNamed(lastReorder(wrapper), 'work')?.name).toBe('Projects')
  })

  it('deleting a folder ungroups its feeds back to the top level', async () => {
    const wrapper = mountGrouped()
    await wrapper.find('[data-testid="folder-delete"]').trigger('click')
    const tree = lastReorder(wrapper)
    expect(tree.some((n) => n.kind === 'folder')).toBe(false)
    expect(tree.some((n) => n.kind === 'feed' && n.feed.id === 'backend')).toBe(true)
  })

  it('emits a reorder when a feed is dragged to the trailing drop zone', async () => {
    const wrapper = mountGrouped()
    // Drag the top-level "desktop" feed to the end.
    await wrapper.find('[data-testid="sidebar-item"]').trigger('dragstart')
    await wrapper.find('[data-testid="sidebar-drop-end"]').trigger('dragover')
    await wrapper.find('[data-testid="sidebar-drop-end"]').trigger('drop')

    const tree = lastReorder(wrapper)
    expect(tree[tree.length - 1]).toMatchObject({ kind: 'feed', feed: { id: 'desktop' } })
  })

  it('falls back to a flat list of feeds when the profile has no tree', () => {
    const wrapper = mountSideBar()
    expect(wrapper.findAll('[data-testid="sidebar-item"]')).toHaveLength(2)
    expect(wrapper.find('[data-testid="sidebar-folder"]').exists()).toBe(false)
  })
})
