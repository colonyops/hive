import { fakePlatform } from '@hive/ui/test-utils/platform'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import DriveListing from '../components/DriveListing.vue'
import { documentEntries, folderContents } from '../lib/entries'
import { spaceFromKey } from '../lib/space'
import { useViewMode } from '../lib/viewMode'
import { doc } from './fixtures'

const api = vi.hoisted(() => ({ share: vi.fn(), unshare: vi.fn(), deleteDocument: vi.fn() }))
vi.mock('../api', async (importOriginal) => ({ ...(await importOriginal<typeof import('../api')>()), api }))

const space = spaceFromKey('acme/notes')
const docs = [
  doc({ path: 'plan.md', title: 'Plan', shared: { url: 'http://drive.test/s/x', expires_in: '' } }),
  doc({ path: 'docs/guide.md', title: 'Guide' }),
]

async function mountListing(entries = folderContents(docs, space, '')) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/repos/:owner/:repo/:path(.*)*', component: {} }],
  })
  await router.push('/repos/acme/notes')
  const wrapper = mount(DriveListing, {
    props: { crumbs: [{ label: 'acme/notes', to: '/repos/acme/notes' }], entries, emptyMessage: 'Nothing here.' },
    attachTo: document.body,
    global: { plugins: [router] },
  })
  return { router, wrapper }
}

describe('DriveListing', () => {
  beforeEach(() => {
    localStorage.clear()
    Object.values(api).forEach((m) => m.mockReset())
  })

  it('lists folders first with the title as name and the file name as tooltip', async () => {
    const { wrapper } = await mountListing()

    const rows = wrapper.findAll('[data-testid="entry"]')
    expect(rows.map((r) => r.attributes('data-kind'))).toEqual(['dir', 'doc'])
    expect(rows[1].text()).toContain('Plan')
    expect(rows[1].text()).not.toContain('plan.md')
    expect(rows[1].attributes('title')).toBe('plan.md')
    expect(rows[1].text()).toContain('Shared')
    expect(rows[0].text()).not.toContain('Shared')
  })

  it('renders every list row as a single-line name cell', async () => {
    const { wrapper } = await mountListing()

    for (const row of wrapper.findAll('[data-testid="entry"]')) {
      const label = row.get('[data-testid="entry-label"]')
      expect(label.classes()).toContain('truncate')
      expect(label.element.children).toHaveLength(0)
      expect(row.classes()).toContain('h-13')
    }
    expect(wrapper.find('[data-testid="entry-location"]').exists()).toBe(false)
  })

  it('shows a Location column only in cross-space listings', async () => {
    const across = await mountListing(documentEntries([doc({ space: 'acme/notes', path: 'docs/guide.md' })]))
    expect(across.wrapper.get('[data-testid="entry-location"]').text()).toBe('acme/notes / docs')
  })

  it('opens a folder or a document on double click', async () => {
    const { wrapper, router } = await mountListing()
    const rows = wrapper.findAll('[data-testid="entry"]')

    await rows[0].trigger('dblclick')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/repos/acme/notes/docs')

    await rows[1].trigger('dblclick')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/repos/acme/notes/plan.md')
  })

  it('opens the active entry with Enter and walks with the arrow keys', async () => {
    const { wrapper, router } = await mountListing()
    const list = wrapper.get('[data-testid="entries"]')

    await list.trigger('keydown', { key: 'ArrowDown' })
    await list.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/repos/acme/notes/plan.md')
  })

  it('switches between list and grid and remembers the choice', async () => {
    const { wrapper } = await mountListing()
    expect(wrapper.get('[data-testid="entries"]').attributes('data-mode')).toBe('list')

    await wrapper.get('[data-testid="view-toggle-grid"]').trigger('click')

    expect(wrapper.get('[data-testid="entries"]').attributes('data-mode')).toBe('grid')
    expect(localStorage.getItem('drive.view')).toBe('grid')
    expect(useViewMode().mode.value).toBe('grid')
  })

  it('falls back to the list when storage is blocked', () => {
    const spy = vi.spyOn(localStorage, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })

    expect(useViewMode().mode.value).toBe('list')
    spy.mockRestore()
  })

  it('shows an empty state for an empty folder', async () => {
    const { wrapper } = await mountListing([])

    expect(wrapper.get('[data-testid="listing-empty"]').text()).toBe('Nothing here.')
  })

  it('opens the share popover from the row menu and shares', async () => {
    api.share.mockResolvedValue({ url: 'http://drive.test/s/new', expires_in: '' })
    const entries = documentEntries([doc({ path: 'a.md' })])
    const { wrapper } = await mountListing(entries)

    await wrapper.get('[data-testid="entry-menu"]').trigger('click')
    expect(document.body.querySelector('[data-testid="menu-copy-link"]')).toBeNull()
    document.body.querySelector<HTMLElement>('[data-testid="menu-share"]')!.click()
    await flushPromises()
    document.body.querySelector<HTMLElement>('[data-testid="share-create"]')!.click()
    await flushPromises()

    expect(api.share).toHaveBeenCalledWith('/api/v1/repos/acme/notes', 'a.md', '')
    expect(wrapper.emitted('changed')).toBeTruthy()
  })

  it('copies the link of a shared document from the row menu', async () => {
    const { wrapper } = await mountListing()

    await wrapper.findAll('[data-testid="entry-menu"]')[1].trigger('click')
    document.body.querySelector<HTMLElement>('[data-testid="menu-copy-link"]')!.click()
    await flushPromises()

    expect(fakePlatform.clipboard.writeText).toHaveBeenCalledWith('http://drive.test/s/x')
  })
})
