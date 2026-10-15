import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Tokens from '../pages/Tokens.vue'
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

  it('shows the new secret once and not after the page is left', async () => {
    const first = mount(Tokens)
    await flushPromises()
    await first.get('[data-testid="token-name"]').setValue('laptop')
    await first.get('[data-testid="token-form"]').trigger('submit')
    await flushPromises()

    expect(mocks.createToken).toHaveBeenCalledWith({ name: 'laptop', scope: 'read', expires_in: '' })
    expect(first.get('[data-testid="token-secret"]').text()).toContain('hdt_secret_value')

    first.unmount()
    const second = mount(Tokens)
    await flushPromises()

    expect(second.find('[data-testid="token-secret"]').exists()).toBe(false)
    expect(second.text()).not.toContain('hdt_secret_value')
    expect(second.findAll('[data-testid="token-row"]')).toHaveLength(1)
  })

  it('revokes after confirmation and reloads the list', async () => {
    mocks.revokeToken.mockResolvedValue(undefined)
    const wrapper = mount(Tokens, { attachTo: document.body })
    await flushPromises()

    await wrapper.get('[data-testid="token-revoke"]').trigger('click')
    document.body.querySelector<HTMLElement>('[data-testid="revoke-dialog-confirm"]')!.click()
    await flushPromises()

    expect(mocks.revokeToken).toHaveBeenCalledWith('t1')
    expect(mocks.tokens).toHaveBeenCalledTimes(2)
  })
})
