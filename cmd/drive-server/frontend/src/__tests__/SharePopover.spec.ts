import { fakePlatform } from '@hive/ui/test-utils/platform'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SharePopover from '../components/SharePopover.vue'
import type { Share } from '../types'

const api = vi.hoisted(() => ({ share: vi.fn(), unshare: vi.fn() }))
vi.mock('../api', async (importOriginal) => ({ ...(await importOriginal<typeof import('../api')>()), api }))

const prefix = '/api/v1/repos/acme/notes'

function mountPopover(share: Share | null) {
  const anchor = document.body.appendChild(document.createElement('button'))
  const wrapper = mount(SharePopover, { props: { anchor, prefix, path: 'a.md', share }, attachTo: document.body })
  return wrapper
}

const panel = () => document.body.querySelector<HTMLElement>('[data-testid="share-popover"]')!
const pick = (testid: string) => document.body.querySelector<HTMLElement>(`[data-testid="${testid}"]`)!

describe('SharePopover', () => {
  beforeEach(() => {
    Object.values(api).forEach((m) => m.mockReset())
    document.body.innerHTML = ''
  })

  it('offers to create a link while sharing is off', async () => {
    api.share.mockResolvedValue({ url: 'http://drive.test/s/new', expires_in: '' })
    const wrapper = mountPopover(null)

    expect(pick('share-state').getAttribute('aria-label')).toBe('Sharing is off')
    expect(pick('share-url')).toBeNull()
    pick('share-expiry-7d').click()
    pick('share-create').click()
    await flushPromises()

    expect(api.share).toHaveBeenCalledWith(prefix, 'a.md', '7d')
    expect(wrapper.emitted('changed')?.[0]).toEqual([{ url: 'http://drive.test/s/new', expires_in: '' }])
    expect(pick('share-url')).toHaveProperty('value', 'http://drive.test/s/new')
  })

  it('shows the link read-only and copies it', async () => {
    mountPopover({ url: 'http://drive.test/s/abc', expires_in: '' })

    const field = pick('share-url') as HTMLInputElement
    expect(field.readOnly).toBe(true)
    expect(field.value).toBe('http://drive.test/s/abc')
    pick('share-copy').click()
    await flushPromises()

    expect(fakePlatform.clipboard.writeText).toHaveBeenCalledWith('http://drive.test/s/abc')
    expect(panel()).not.toBeNull()
  })

  it('stops sharing', async () => {
    api.unshare.mockResolvedValue(undefined)
    const wrapper = mountPopover({ url: 'http://drive.test/s/abc', expires_in: '' })

    pick('share-stop').click()
    await flushPromises()

    expect(api.unshare).toHaveBeenCalledWith(prefix, 'a.md')
    expect(wrapper.emitted('changed')?.[0]).toEqual([null])
    expect(pick('share-url')).toBeNull()
  })

  const url = 'http://drive.test/s/abc'
  const pressed = (suffix: string) => pick(`share-expiry-${suffix}`).getAttribute('aria-pressed')

  it('marks the stored preset and shows its date', () => {
    mountPopover({ url, expires_in: '30d', expires_at: '2026-11-14T13:46:57Z' })

    expect(pressed('30d')).toBe('true')
    expect(pressed('')).toBe('false')
    expect(pick('share-expiry-text').textContent).toMatch(/^Expires Nov 1[45], 2026$/)
    expect(pick('share-expiry-text').title).not.toBe('')
  })

  it('clears the expiry when None is chosen and keeps the new state', async () => {
    api.share.mockResolvedValue({ url, expires_in: '' })
    const wrapper = mountPopover({ url, expires_in: '30d', expires_at: '2026-11-14T13:46:57Z' })

    pick('share-expiry-').click()
    await flushPromises()

    expect(api.share).toHaveBeenCalledWith(prefix, 'a.md', '')
    expect(wrapper.emitted('changed')?.[0]).toEqual([{ url, expires_in: '' }])
    expect(pressed('')).toBe('true')
    expect(pick('share-expiry-text').textContent).toBe('No expiry')
  })

  it('sends a chosen preset and marks it', async () => {
    api.share.mockResolvedValue({ url, expires_in: '7d', expires_at: '2026-10-22T00:00:00Z' })
    mountPopover({ url, expires_in: '' })

    pick('share-expiry-7d').click()
    await flushPromises()

    expect(api.share).toHaveBeenCalledWith(prefix, 'a.md', '7d')
    expect(pressed('7d')).toBe('true')
  })
})
