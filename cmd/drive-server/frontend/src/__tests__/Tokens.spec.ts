import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Tokens from '../pages/Tokens.vue'
import { mountWithRouter } from '../test-utils/mount'
import type { Token } from '../types'

const mocks = vi.hoisted(() => ({
  tokens: vi.fn(),
  createToken: vi.fn(),
  revokeToken: vi.fn(),
}))
vi.mock('../api', () => ({ api: mocks }))

const stored: Token = { id: 't1', name: 'laptop', scope: 'push', created_at: '2026-10-15T12:00:00Z' }

describe('Tokens', () => {
  beforeEach(() => {
    Object.values(mocks).forEach((m) => m.mockReset())
    mocks.tokens.mockResolvedValue([stored])
    mocks.createToken.mockResolvedValue({ ...stored, secret: 'hdt_secret_value' })
  })

  it('shows a spinner, not the empty state, while the list loads', () => {
    mocks.tokens.mockReturnValue(new Promise(() => {}))
    const wrapper = mount(Tokens)

    expect(wrapper.find('[data-testid="tokens-loading"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('No tokens yet.')
  })

  it('shows the load failure instead of an empty list', async () => {
    mocks.tokens.mockRejectedValue(new Error('server down'))
    const { wrapper } = await mountWithRouter(Tokens)

    expect(wrapper.get('[data-testid="tokens-error"]').text()).toContain('server down')
    expect(wrapper.text()).not.toContain('No tokens yet.')
    expect(wrapper.find('[data-testid="tokens-loading"]').exists()).toBe(false)
  })

  it('says there are no tokens once an empty list arrives', async () => {
    mocks.tokens.mockResolvedValue([])
    const { wrapper } = await mountWithRouter(Tokens)

    expect(wrapper.text()).toContain('No tokens yet.')
  })

  it('shows the new secret once and not after the page is left', async () => {
    const { wrapper: first } = await mountWithRouter(Tokens)
    await first.get('[data-testid="token-name"]').setValue('laptop')
    await first.get('[data-testid="token-form"]').trigger('submit')
    await flushPromises()

    expect(mocks.createToken).toHaveBeenCalledWith({ name: 'laptop', scope: 'read', expires_in: '' })
    expect(first.get('[data-testid="token-secret"]').text()).toContain('hdt_secret_value')

    first.unmount()
    const { wrapper: second } = await mountWithRouter(Tokens)

    expect(second.find('[data-testid="token-secret"]').exists()).toBe(false)
    expect(second.text()).not.toContain('hdt_secret_value')
    expect(second.findAll('[data-testid="token-row"]')).toHaveLength(1)
  })

  it('revokes after confirmation and reloads the list', async () => {
    mocks.revokeToken.mockResolvedValue(undefined)
    const { wrapper } = await mountWithRouter(Tokens)

    await wrapper.get('[data-testid="token-revoke"]').trigger('click')
    document.body.querySelector<HTMLElement>('[data-testid="revoke-dialog-confirm"]')!.click()
    await flushPromises()

    expect(mocks.revokeToken).toHaveBeenCalledWith('t1')
    expect(mocks.tokens).toHaveBeenCalledTimes(2)
  })
})
