import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import AppLayout from '../pages/AppLayout.vue'
import DocumentFeed from '../pages/DocumentFeed.vue'
import SpaceRoute from '../pages/SpaceRoute.vue'
import { doc, frameStub } from './fixtures'

const mocks = vi.hoisted(() => ({
  me: vi.fn(),
  spaces: vi.fn(),
  documents: vi.fn(),
  recent: vi.fn(),
  sharedLinks: vi.fn(),
  documentBody: vi.fn(),
  search: vi.fn(),
}))
vi.mock('../api', async (importOriginal) => ({ ...(await importOriginal<typeof import('../api')>()), api: mocks }))

async function mountAt(path: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: '/',
        component: AppLayout,
        children: [
          { path: 'links', name: 'links', component: DocumentFeed, props: { feed: 'links' } },
          { path: 'recent', name: 'recent', component: DocumentFeed, props: { feed: 'recent' } },
          { path: 'tokens', name: 'tokens', component: {} },
          { path: 'repos/:owner/:repo/:path(.*)*', name: 'repo', component: SpaceRoute },
        ],
      },
      { path: '/sign-in', name: 'sign-in', component: {} },
    ],
  })
  await router.push(path)
  const app = mount(
    { template: '<RouterView />' },
    { attachTo: document.body, global: { plugins: [router], stubs: { HtmlDocumentFrame: frameStub } } },
  )
  await flushPromises()
  return { router, app }
}

describe('drive shell', () => {
  beforeEach(() => {
    localStorage.clear()
    Object.values(mocks).forEach((m) => m.mockReset())
    mocks.me.mockResolvedValue({ email: 'ada@example.com', policy: 'x', quota: { used: 2048, limit: 4096 } })
    mocks.spaces.mockResolvedValue([
      { space: 'acme/notes', kind: 'repository', documents: 3, bytes: 3000 },
      { space: 'workspaces/scratch', kind: 'workspace', documents: 1, bytes: 10 },
    ])
    mocks.documents.mockImplementation((prefix: string) =>
      Promise.resolve(
        prefix.includes('scratch')
          ? [doc({ space: 'workspaces/scratch', path: 'todo.md', shared: { url: 'http://x/s/1', expires_in: '' } })]
          : [
              doc({ path: 'docs/guide.md', title: 'Guide' }),
              doc({ path: 'docs/api/auth.md', title: 'Auth' }),
              doc({ path: 'plan.md', title: 'Plan' }),
            ],
      ),
    )
    mocks.documentBody.mockResolvedValue('# Hello')
    mocks.recent.mockResolvedValue([
      doc({ path: 'plan.md', title: 'Plan' }),
      doc({ space: 'workspaces/scratch', path: 'todo.md', title: 'Todo' }),
    ])
    mocks.sharedLinks.mockResolvedValue([
      doc({ space: 'workspaces/scratch', path: 'todo.md', shared: { url: 'http://x/s/1', expires_in: '' } }),
    ])
  })

  it('lists the spaces flat in the nav, marks the open one, and shows the folder contents', async () => {
    const { app } = await mountAt('/repos/acme/notes/docs')

    const sidebar = app.get('[data-testid="sidebar"]')
    expect(sidebar.text()).toContain('Repositories')
    expect(sidebar.text()).toContain('Workspaces')
    expect(sidebar.findAll('[data-testid="nav-space"]').map((e) => e.text())).toEqual(['acme/notes3', 'scratch1'])
    expect(sidebar.find('[data-testid^="tree-row"]').exists()).toBe(false)
    const current = sidebar.findAll('[aria-current="page"]')
    expect(current.map((e) => e.attributes('title'))).toEqual(['acme/notes'])
    expect(app.get('[data-testid="storage-meter"]').text()).toContain('2.0 KB of 4.0 KB')
    expect(app.findAll('[data-testid="entry-label"]').map((e) => e.text())).toEqual(['api', 'Guide'])
    expect(app.get('[data-testid="breadcrumb"]').text()).toContain('docs')
  })

  it('goes back to the containing folder from the document header', async () => {
    const { app, router } = await mountAt('/repos/acme/notes/docs/guide.md')

    await app.get('[data-testid="back"]').trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/repos/acme/notes/docs')
  })

  it('opens a document path in the viewer', async () => {
    const { app } = await mountAt('/repos/acme/notes/docs/guide.md')

    expect(app.get('[data-testid="document-title"]').text()).toBe('Guide')
    expect(app.get('[data-testid="document-markdown"]').text()).toContain('Hello')
  })

  it('lists shared documents across spaces under Shared links', async () => {
    const { app } = await mountAt('/links')

    expect(app.findAll('[data-testid="entry-label"]').map((e) => e.text())).toEqual(['todo'])
    expect(mocks.sharedLinks).toHaveBeenCalledTimes(1)
    expect(mocks.documents).not.toHaveBeenCalled()
  })

  it('lists recent documents with one request instead of one per space', async () => {
    const { app } = await mountAt('/recent')

    expect(app.findAll('[data-testid="entry-label"]').map((e) => e.text())).toEqual(['Plan', 'Todo'])
    expect(mocks.recent).toHaveBeenCalledTimes(1)
    expect(mocks.documents).not.toHaveBeenCalled()
  })

  it('shows the error when the feed request fails', async () => {
    mocks.recent.mockRejectedValue(new Error('boom'))
    const { app } = await mountAt('/recent')

    expect(app.text()).toContain('boom')
  })

  it('drills into a folder and leaves the rows alone while typing in search', async () => {
    const { app, router } = await mountAt('/repos/acme/notes')

    await app.get('[data-testid="entry"][data-kind="dir"]').trigger('dblclick')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/repos/acme/notes/docs')

    await app.get('[data-testid="search-input"]').setValue('guide')
    await flushPromises()
    expect(app.findAll('[data-testid="entry-label"]').map((e) => e.text())).toEqual(['api', 'Guide'])
  })

  it('shows the email and both items in the account menu', async () => {
    const { app } = await mountAt('/links')

    expect(app.find('[data-testid="account-menu-panel"]').exists()).toBe(false)
    await app.get('[data-testid="account-menu"]').trigger('click')

    const panel = app.get('[data-testid="account-menu-panel"]')
    expect(panel.text()).toContain('ada@example.com')
    expect(panel.get('[data-testid="account-tokens"]').text()).toBe('API tokens')
    expect(panel.get('[data-testid="sign-out"]').text()).toBe('Sign out')
  })
})
