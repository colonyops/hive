import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OrchestratorSettingsView from '../OrchestratorSettingsView.vue'

const mocks = vi.hoisted(() => ({
  ListTokens: vi.fn(),
  CreateToken: vi.fn(),
  RevokeToken: vi.fn(),
  ServerURL: vi.fn(),
}))
vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/orchestrationservice',
  () => mocks,
)

const worker = { id: 7, name: 'worker', hint: 'ab12', createdAt: 1, lastUsedAt: 0 }

function el<T extends HTMLElement>(id: string): T {
  return document.querySelector<T>(`[data-testid="${id}"]`)!
}

beforeEach(() => {
  vi.clearAllMocks()
  document.body.innerHTML = ''
  mocks.ListTokens.mockResolvedValue([worker])
  mocks.ServerURL.mockResolvedValue('http://127.0.0.1:5000/mcp/orchestrator')
})

describe('OrchestratorSettingsView', () => {
  it('lists tokens by name and hint, never the token', async () => {
    mount(OrchestratorSettingsView, { attachTo: document.body })
    await flushPromises()

    expect(el('orchestrator-token-7').textContent).toContain('worker')
    expect(el('orchestrator-token-7').textContent).toContain('…ab12')
    expect(el('orchestrator-server-url').textContent).toContain('/mcp/orchestrator')
  })

  it('shows a created token once, with a config that points at the server', async () => {
    mocks.CreateToken.mockResolvedValue({ ...worker, id: 8, name: 'ci', token: 'hvo_secret' })
    mount(OrchestratorSettingsView, { attachTo: document.body })
    await flushPromises()

    el<HTMLButtonElement>('orchestrator-token-create').click()
    await flushPromises()
    const input = el<HTMLInputElement>('orchestrator-token-name')
    input.value = 'ci'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    el<HTMLButtonElement>('orchestrator-token-save').click()
    await flushPromises()

    expect(mocks.CreateToken).toHaveBeenCalledWith('ci')
    expect(el('orchestrator-token-value').textContent).toContain('hvo_secret')
    expect(el('orchestrator-token-created').textContent).toContain('http://127.0.0.1:5000/mcp/orchestrator')

    el<HTMLButtonElement>('orchestrator-token-done').click()
    await flushPromises()
    expect(el('orchestrator-token-value')).toBeNull()
  })

  it('revokes through the confirmation dialog', async () => {
    mocks.RevokeToken.mockResolvedValue(undefined)
    mount(OrchestratorSettingsView, { attachTo: document.body })
    await flushPromises()

    el<HTMLButtonElement>('orchestrator-token-revoke-7').click()
    await flushPromises()
    el<HTMLButtonElement>('confirmation-dialog-confirm').click()
    await flushPromises()

    expect(mocks.RevokeToken).toHaveBeenCalledWith(7)
    expect(mocks.ListTokens).toHaveBeenCalledTimes(2)
  })
})
