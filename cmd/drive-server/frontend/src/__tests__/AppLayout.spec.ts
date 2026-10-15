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
          { path: '', name: 'spaces', component: {} },
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
          ? [doc({ space: 'workspaces/scratch', path: 'todo.md', shared: { url: 'http://x/s/1' } })]
          : [
              doc({ path: 'docs/guide.md', title: 'Guide' }),
              doc({ path: 'docs/api/auth.md', title: 'Auth' }),
              doc({ path: 'plan.md', title: 'Plan' }),
            ],
      ),
    )
    mocks.documentBody.mockResolvedValue('# Hello')
  })

  it('shows grouped spaces, the folder tree of the open space, and the folder contents', async () => {
    const { app } = await mountAt('/repos/acme/notes/docs')

    expect(app.get('[data-testid="sidebar"]').text()).toContain('Repositories')
    expect(app.get('[data-testid="sidebar"]').text()).toContain('Workspaces')
    expect(app.get('[data-testid="storage-meter"]').text()).toContain('2.0 KB of 4.0 KB')
    expect(app.find('[data-testid="tree-row-docs/api"]').exists()).toBe(true)
    const labels = app.findAll('[data-testid="entry-label"]').map((e) => e.text())
    expect(labels).toEqual(['api', 'Guide'])
    expect(app.get('[data-testid="breadcrumb"]').text()).toContain('docs')
  })

  it('opens a document path in the viewer', async () => {
    const { app } = await mountAt('/repos/acme/notes/docs/guide.md')

    expect(app.get('[data-testid="document-title"]').text()).toBe('Guide')
    expect(app.get('[data-testid="document-markdown"]').text()).toContain('Hello')
  })

  it('lists shared documents across spaces under Shared links', async () => {
    const { app } = await mountAt('/links')

    expect(app.findAll('[data-testid="entry-label"]').map((e) => e.text())).toEqual(['todo'])
  })

  it('navigates into a folder from the nav tree and filters it from the search field', async () => {
    const { app, router } = await mountAt('/repos/acme/notes')

    await app.get('[data-testid="tree-row-docs"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/repos/acme/notes/docs')

    await app.get('[data-testid="search-input"]').setValue('plan')
    await flushPromises()
    expect(app.findAll('[data-testid="entry-label"]').map((e) => e.text())).toEqual(['Plan'])
  })
})
