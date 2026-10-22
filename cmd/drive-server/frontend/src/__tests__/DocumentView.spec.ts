import { fakePlatform } from '@hive/ui/test-utils/platform'
import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { spaceFromKey } from '../lib/space'
import DocumentView from '../pages/DocumentView.vue'
import { mountWithRouter } from '../test-utils/mount'
import type { DriveDocument } from '../types'
import { doc } from './fixtures'

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

function mountView(path: string) {
  return mountWithRouter(DocumentView, {
    path: `/repos/acme/notes/${path}`,
    props: { space, path, documents, loadError: null },
  })
}

async function shareAndCopy(wrapper: Awaited<ReturnType<typeof mountView>>['wrapper']) {
  await wrapper.get('[data-testid="share"]').trigger('click')
  document.body.querySelector<HTMLElement>('[data-testid="share-create"]')!.click()
  await flushPromises()
  await wrapper.get('[data-testid="copy-link"]').trigger('click')
  await flushPromises()
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

  it('offers text size for markdown but only Delete for an html document', async () => {
    const md = await mountView('a.md')
    await md.wrapper.get('[data-testid="more-actions"]').trigger('click')
    expect(md.wrapper.find('[data-testid="menu-larger"]').exists()).toBe(true)
    expect(md.wrapper.find('[data-testid="menu-smaller"]').exists()).toBe(true)
    md.wrapper.unmount()

    const html = await mountView('page.html')
    await html.wrapper.get('[data-testid="more-actions"]').trigger('click')
    expect(html.wrapper.find('[data-testid="menu-larger"]').exists()).toBe(false)
    expect(html.wrapper.find('[data-testid="menu-smaller"]').exists()).toBe(false)
    expect(html.wrapper.find('[data-testid="delete-document"]').exists()).toBe(true)
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

  it('shares from the popover and then offers a quick copy', async () => {
    mocks.share.mockResolvedValue({ url: 'http://drive.test/s/abc', expires_in: '' })
    const { wrapper, toast } = await mountView('a.md')

    await shareAndCopy(wrapper)

    expect(mocks.share).toHaveBeenCalledWith('/api/v1/repos/acme/notes', 'a.md', '')
    expect(wrapper.emitted('changed')).toBeTruthy()
    expect(fakePlatform.clipboard.writeText).toHaveBeenCalledWith('http://drive.test/s/abc')
    expect(toast.show).toHaveBeenLastCalledWith('Link copied')
  })

  it('says so when the link does not reach the clipboard', async () => {
    mocks.share.mockResolvedValue({ url: 'http://drive.test/s/abc', expires_in: '' })
    fakePlatform.clipboard.writeText.mockRejectedValueOnce(new Error('denied'))
    const { wrapper, toast } = await mountView('a.md')

    await shareAndCopy(wrapper)

    expect(toast.show).toHaveBeenLastCalledWith('Could not copy the link')
    expect(toast.show).not.toHaveBeenCalledWith('Link copied')
  })
})
