import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { meKey } from '../lib/me'
import { spaceFromKey } from '../lib/space'
import DocumentView from '../pages/DocumentView.vue'
import { doc, frameStub } from './fixtures'

const mocks = vi.hoisted(() => ({
  documents: vi.fn(),
  documentBody: vi.fn(),
  deleteDocument: vi.fn(),
}))
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  api: mocks,
}))

const space = spaceFromKey('acme/notes')

async function mountView(path: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/repos/:owner/:repo/:path(.*)*', component: {} }],
  })
  await router.push(`/repos/acme/notes/${path}`)
  const refresh = vi.fn().mockResolvedValue(undefined)
  const wrapper = mount(DocumentView, {
    props: { space, path },
    attachTo: document.body,
    global: {
      plugins: [router],
      stubs: { HtmlDocumentFrame: frameStub },
      provide: { [meKey as symbol]: { me: ref(null), refresh } },
    },
  })
  await flushPromises()
  return { router, wrapper, refresh }
}

describe('DocumentView', () => {
  beforeEach(() => {
    Object.values(mocks).forEach((m) => m.mockReset())
    mocks.documents.mockResolvedValue([doc({ path: 'a.md' }), doc({ path: 'page.html', kind: 'html' })])
    mocks.documentBody.mockResolvedValue('# Hello\n\nBody text.')
    mocks.deleteDocument.mockResolvedValue(undefined)
  })

  it('renders a markdown body', async () => {
    const { wrapper } = await mountView('a.md')

    expect(mocks.documentBody).toHaveBeenCalledWith('/api/v1/repos/acme/notes', 'a.md')
    expect(wrapper.get('[data-testid="document-markdown"]').text()).toContain('Body text.')
  })

  it('loads an html document from the raw route without fetching its body', async () => {
    const { wrapper } = await mountView('page.html')

    expect(mocks.documentBody).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="frame"]').attributes('data-src')).toBe(
      '/api/v1/repos/acme/notes/docs/page.html?raw=1',
    )
  })

  it('deletes only after confirmation, then refreshes storage and returns to the list', async () => {
    const { router, wrapper, refresh } = await mountView('a.md')

    await wrapper.get('[data-testid="delete-document"]').trigger('click')
    expect(mocks.deleteDocument).not.toHaveBeenCalled()

    document.body.querySelector<HTMLElement>('[data-testid="delete-dialog-confirm"]')!.click()
    await flushPromises()

    expect(mocks.deleteDocument).toHaveBeenCalledWith('/api/v1/repos/acme/notes', 'a.md')
    expect(refresh).toHaveBeenCalled()
    expect(router.currentRoute.value.path).toBe('/repos/acme/notes')
  })
})
