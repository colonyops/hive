import { afterEach, describe, expect, it, vi } from 'vitest'
import ShareView from '../pages/ShareView.vue'
import { mountWithRouter } from '../test-utils/mount'
import { doc } from './fixtures'

// Stubbing fetch instead of api.ts exercises the real URL and the 404 decoding.
const fetchMock = vi.fn<typeof fetch>()
vi.stubGlobal('fetch', fetchMock)

function respond(status: number, body: unknown): void {
  fetchMock.mockResolvedValue(new Response(JSON.stringify(body), { status }))
}

async function mountShare(slug: string) {
  return (await mountWithRouter(ShareView, { props: { slug } })).wrapper
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
    expect(wrapper.get('[data-testid="share-home"]').attributes('href')).toBe('/')
    expect(wrapper.get('[data-testid="share-title"]').text()).toBe('a')
    expect(wrapper.get('[data-testid="share-notice"]').text()).toContain('ada@example.com')
    expect(wrapper.find('[aria-label*="user content"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="document-markdown"]').text()).toContain('Shared body.')
  })

  it('points an html document at the raw route', async () => {
    respond(200, { owner_email: 'ada@example.com', document: doc({ path: 'page.html', kind: 'html' }) })
    const wrapper = await mountShare('a b')

    expect(wrapper.get('[data-testid="frame"]').attributes('data-src')).toBe('/s/a%20b/raw')
  })

  it('reveals an html document once its frame loads', async () => {
    respond(200, { owner_email: 'ada@example.com', document: doc({ path: 'page.html', kind: 'html' }) })
    const wrapper = await mountShare('abc')
    const reader = () => wrapper.get('[data-testid="document-document"]')

    expect(reader().classes()).toContain('opacity-0')
    wrapper.get('[data-testid="frame"]').element.dispatchEvent(new Event('load'))
    await wrapper.vm.$nextTick()
    expect(reader().classes()).toContain('opacity-100')
  })

  it('shows the dead-link state on 404', async () => {
    respond(404, { kind: 'not_found', message: 'not found' })
    const wrapper = await mountShare('gone')

    expect(wrapper.get('[data-testid="share-missing"]').text()).toContain('does not exist')
    expect(wrapper.find('[data-testid="document-markdown"]').exists()).toBe(false)
  })

  it('shows any other failure as an error', async () => {
    respond(500, { kind: 'internal', message: 'storage offline' })
    const wrapper = await mountShare('abc')

    expect(wrapper.get('[data-testid="share-error"]').text()).toContain('storage offline')
    expect(wrapper.find('[data-testid="share-missing"]').exists()).toBe(false)
  })
})
