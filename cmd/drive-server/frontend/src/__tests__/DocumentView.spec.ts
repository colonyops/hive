import { fakePlatform } from '@hive/ui/test-utils/platform'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { meKey } from '../lib/me'
import { spaceFromKey } from '../lib/space'
import DocumentView from '../pages/DocumentView.vue'
import type { DriveDocument } from '../types'
import { doc, frameStub } from './fixtures'

const mocks = vi.hoisted(() => ({
  documentBody: vi.fn(),
  deleteDocument: vi.fn(),
  share: vi.fn(),
  unshare: vi.fn(),
}))
vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  api: mocks,
}))

const space = spaceFromKey('acme/notes')

let documents: DriveDocument[] = []

async function mountView(path: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/repos/:owner/:repo/:path(.*)*', component: {} }],
  })
  await router.push(`/repos/acme/notes/${path}`)
  const refresh = vi.fn().mockResolvedValue(undefined)
  const wrapper = mount(DocumentView, {
    props: { space, path, documents, loadError: null },
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
    documents = [doc({ path: 'a.md' }), doc({ path: 'page.html', kind: 'html' })]
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

    await wrapper.get('[data-testid="more-actions"]').trigger('click')
    await wrapper.get('[data-testid="delete-document"]').trigger('click')
    expect(mocks.deleteDocument).not.toHaveBeenCalled()

    document.body.querySelector<HTMLElement>('[data-testid="delete-dialog-confirm"]')!.click()
    await flushPromises()

    expect(mocks.deleteDocument).toHaveBeenCalledWith('/api/v1/repos/acme/notes', 'a.md')
    expect(refresh).toHaveBeenCalled()
    expect(router.currentRoute.value.path).toBe('/repos/acme/notes')
  })

  it('keeps the header free of a raw link and offers Share', async () => {
    const { wrapper } = await mountView('a.md')

    expect(wrapper.find('[data-testid="copy-link"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="share"]').text()).toBe('Share')
    expect(wrapper.text()).not.toContain('http')
  })

  it('shares from the popover and then offers a quick copy', async () => {
    mocks.share.mockResolvedValue({ url: 'http://drive.test/s/abc', expires_in: '' })
    const { wrapper } = await mountView('a.md')

    await wrapper.get('[data-testid="share"]').trigger('click')
    document.body.querySelector<HTMLElement>('[data-testid="share-create"]')!.click()
    await flushPromises()

    expect(mocks.share).toHaveBeenCalledWith('/api/v1/repos/acme/notes', 'a.md', '')
    expect(wrapper.emitted('changed')).toBeTruthy()
    await wrapper.get('[data-testid="copy-link"]').trigger('click')
    await flushPromises()
    expect(fakePlatform.clipboard.writeText).toHaveBeenCalledWith('http://drive.test/s/abc')
  })

  it('starts from an existing share link', async () => {
    documents = [doc({ path: 'a.md', shared: { url: 'http://drive.test/s/old', expires_in: '' } })]
    const { wrapper } = await mountView('a.md')

    await wrapper.get('[data-testid="share"]').trigger('click')

    expect((document.body.querySelector('[data-testid="share-url"]') as HTMLInputElement).value).toBe(
      'http://drive.test/s/old',
    )
    expect(wrapper.find('[data-testid="copy-link"]').exists()).toBe(true)
  })
})
