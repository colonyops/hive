import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ShareView from '../pages/ShareView.vue'
import { doc, frameStub } from './fixtures'

// Stubbing fetch instead of api.ts exercises the real URL and the 404 decoding.
const fetchMock = vi.fn<typeof fetch>()
vi.stubGlobal('fetch', fetchMock)

function respond(status: number, body: unknown): void {
  fetchMock.mockResolvedValue(new Response(JSON.stringify(body), { status }))
}

async function mountShare(slug: string) {
  const wrapper = mount(ShareView, { props: { slug }, global: { stubs: { HtmlDocumentFrame: frameStub } } })
  await flushPromises()
  return wrapper
}

describe('ShareView', () => {
  afterEach(() => fetchMock.mockReset())

  it('renders a markdown document with its owner and the user content notice', async () => {
    respond(200, {
      owner_email: 'ada@example.com',
      document: doc({ path: 'notes/a.md' }),
      body: '# Title\n\nShared body.',
    })
    const wrapper = await mountShare('abc')

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/public/abc')
    expect(wrapper.get('[data-testid="share-notice"]').text()).toContain('ada@example.com')
    expect(wrapper.get('[data-testid="share-notice"]').text()).toContain('user content')
    expect(wrapper.get('[data-testid="document-markdown"]').text()).toContain('Shared body.')
  })

  it('points an html document at the raw route', async () => {
    respond(200, { owner_email: 'ada@example.com', document: doc({ path: 'page.html', kind: 'html' }) })
    const wrapper = await mountShare('a b')

    expect(wrapper.get('[data-testid="frame"]').attributes('data-src')).toBe('/s/a%20b/raw')
  })

  it('shows the dead-link state on 404', async () => {
    respond(404, { kind: 'not_found', message: 'not found' })
    const wrapper = await mountShare('gone')

    expect(wrapper.get('[data-testid="share-missing"]').text()).toContain('does not exist')
    expect(wrapper.find('[data-testid="document-markdown"]').exists()).toBe(false)
  })
})
