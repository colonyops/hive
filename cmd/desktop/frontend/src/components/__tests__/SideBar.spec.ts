import { beforeEach, describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SideBar from '../SideBar.vue'
import type { FeedTree, Profile, SidebarNode } from '../../types/feed'

// Collapse state and panel width live in localStorage; isolate every test. The
// folder dialogs teleport to the body, so clear that between tests too.
beforeEach(() => {
  localStorage.clear()
  document.body.innerHTML = ''
})

const profile: Profile = {
  id: 'personal',
  letter: 'P',
  name: 'Personal',
  enabled: true,
  nodes: 0,
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
    props: { profile, selection: { type: 'feed', feedId: 'desktop' }, ...overrides },
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

  it('carries only the profile name in the header; the source count lives in profile settings', () => {
    expect(mountSideBar().get('[data-testid="sidebar-profile-header"]').text()).toBe('Personal')
  })

  it('has no aggregate inbox views; feeds are the only primary destinations', () => {
    const wrapper = mountSideBar()
    expect(wrapper.find('[data-testid="inbox-view-switcher"]').exists()).toBe(false)
    for (const view of ['inbox', 'open', 'archive', 'all', 'ignored']) {
      expect(wrapper.find(`[data-testid="inbox-view-${view}"]`).exists()).toBe(false)
    }
  })

  it('renders a de-emphasized trash entry without a count and selects it', async () => {
    const wrapper = mountSideBar()
    const trash = wrapper.get('[data-testid="sidebar-trash"]')
    expect(trash.text()).toBe('Trash')
    await trash.trigger('click')
    expect(wrapper.emitted('select')).toEqual([[{ type: 'trash' }]])
    expect(trash.classes()).not.toContain('footer-entry-selected')

    const selected = mount(SideBar, { props: { profile, selection: { type: 'trash' } } })
    expect(selected.get('[data-testid="sidebar-trash"]').classes()).toContain('footer-entry-selected')
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


  it('does not render per-feed option menus', () => {
    const wrapper = mountSideBar()
    expect(wrapper.find('[data-testid="sidebar-feed-menu-toggle"]').exists()).toBe(false)
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
  return mount(SideBar, {
    props: { profile: grouped, selection: { type: 'feed', feedId: 'desktop' } },
    attachTo: document.body,
  })
}

// The folder edit dialog and its confirmation both teleport out of the wrapper.
function dialog<T extends HTMLElement>(testid: string): T {
  return document.querySelector<T>(`[data-testid="${testid}"]`)!
}

async function openFolderEditor(wrapper: ReturnType<typeof mountGrouped>): Promise<void> {
  await wrapper.get('[data-testid="folder-edit"]').trigger('click')
  await flushPromises()
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

  it('appends a new empty folder and opens it for naming', async () => {
    const wrapper = mountGrouped()
    await wrapper.find('[data-testid="sidebar-new-folder"]').trigger('click')
    const tree = lastReorder(wrapper)
    expect(tree.filter((n) => n.kind === 'folder')).toHaveLength(2)

    // Once the parent persists the tree, the new folder opens in its edit
    // dialog with the placeholder name ready to be typed over.
    await wrapper.setProps({ profile: { ...grouped, tree } })
    await flushPromises()
    expect(dialog<HTMLInputElement>('folder-edit-name').value).toBe('New folder')
    wrapper.unmount()
  })

  it('renames a folder from the edit dialog, emitting the new name', async () => {
    const wrapper = mountGrouped()
    await openFolderEditor(wrapper)

    const input = dialog<HTMLInputElement>('folder-edit-name')
    input.value = 'Projects'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    dialog<HTMLButtonElement>('folder-edit-save').click()
    await flushPromises()

    expect(folderNamed(lastReorder(wrapper), 'work')?.name).toBe('Projects')
    expect(document.querySelector('[data-testid="folder-edit-modal"]')).toBeNull()
    wrapper.unmount()
  })

  it('offers no single-click delete in the tree; deletion starts in the edit dialog', async () => {
    const wrapper = mountGrouped()
    expect(wrapper.find('[data-testid="folder-delete"]').exists()).toBe(false)

    // Every button in the folder header, exercised: none of them writes a layout.
    for (const button of wrapper.findAll('[data-testid="sidebar-folder"] .folder-header button')) {
      await button.trigger('click')
    }
    await flushPromises()
    expect(wrapper.emitted('reorder')).toBeUndefined()
    expect(dialog('folder-edit-modal')).not.toBeNull()
    wrapper.unmount()
  })

  it('deletes a folder only after confirming, ungrouping its feeds to the top level', async () => {
    const wrapper = mountGrouped()
    await openFolderEditor(wrapper)
    dialog<HTMLButtonElement>('folder-edit-delete').click()
    await flushPromises()

    // The confirm expands inside the dialog rather than stacking another one.
    expect(dialog('folder-edit-modal')).not.toBeNull()
    expect(dialog('folder-delete-confirm-description').textContent).toContain('Its 1 feed moves to the top level')
    expect(wrapper.emitted('reorder')).toBeUndefined()

    dialog<HTMLButtonElement>('folder-delete-confirm-confirm').click()
    await flushPromises()

    const tree = lastReorder(wrapper)
    expect(tree.some((n) => n.kind === 'folder')).toBe(false)
    expect(tree.some((n) => n.kind === 'feed' && n.feed.id === 'backend')).toBe(true)
    expect(document.querySelector('[data-testid="folder-edit-modal"]')).toBeNull()
    wrapper.unmount()
  })

  it('cancelling the delete confirmation keeps the folder and the dialog', async () => {
    const wrapper = mountGrouped()
    await openFolderEditor(wrapper)
    dialog<HTMLButtonElement>('folder-edit-delete').click()
    await flushPromises()
    dialog<HTMLButtonElement>('folder-delete-confirm-cancel').click()
    await flushPromises()

    expect(wrapper.emitted('reorder')).toBeUndefined()
    expect(document.querySelector('[data-testid="folder-delete-confirm"]')).toBeNull()
    expect(dialog('folder-edit-modal')).not.toBeNull()
    wrapper.unmount()
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
