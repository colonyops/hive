import { flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ShareView from '../pages/ShareView.vue'
import { mountWithRouter } from '../test-utils/mount'
import { FakeEventSource } from '../test-utils/eventSource'
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

  it('reloads an html frame only for a new version', async () => {
    let sha = 'v1'
    fetchMock.mockImplementation(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            owner_email: 'ada@example.com',
            document: doc({ path: 'page.html', kind: 'html', sha256: sha }),
          }),
          { status: 200 },
        ),
      ),
    )
    const wrapper = await mountShare('abc')
    const src = () => wrapper.get('[data-testid="frame"]').attributes('data-src')

    FakeEventSource.latest().emit('hello', { id: 'p_1', token: 't', sha256: 'v1' })
    await flushPromises()
    expect(src()).toBe('/s/abc/raw')

    sha = 'v2'
    FakeEventSource.latest().emit('document', { sha256: 'v2' })
    await flushPromises()
    expect(src()).toBe('/s/abc/raw?v=1')
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

  it('names the owner and shows the dead-link state when the stream closes', async () => {
    respond(200, {
      owner_email: 'ada@example.com',
      owner_name: 'Ada L',
      document: doc({ path: 'notes/a.md' }),
      body: 'x',
    })
    const wrapper = await mountShare('abc')
    expect(wrapper.get('[data-testid="share-notice"]').text()).toBe('Shared by Ada L')
    expect(FakeEventSource.latest().url).toBe('/api/v1/public/abc/events')

    FakeEventSource.latest().emit('closed')
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="share-missing"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="document-markdown"]').exists()).toBe(false)
  })
})
