import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SendToAgentDialog from '../SendToAgentDialog.vue'
import type { AgentTarget } from '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const toasts = vi.hoisted(() => ({ showToast: vi.fn() }))
vi.mock('../../../composables/useToasts', () => ({ useToasts: () => toasts }))

const host = { load: vi.fn(), deliver: vi.fn() }

function target(kind: string, id: string, pane: string, label: string, status: string): AgentTarget {
  return { kind, id, pane, label, status }
}

const targets = [
  target('session', 's1', '%1', 'app / feat · claude', 'ready'),
  target('session', 's2', '%4', 'app / other · claude', 'approval'),
  target('chat', 'c1', '', 'ws / chat', 'active'),
]

async function mountDialog() {
  const wrapper = mount(SendToAgentDialog, {
    props: { load: host.load, deliver: host.deliver, testid: 'r' },
    attachTo: document.body,
  })
  await flushPromises()
  return wrapper
}

const row = (index: number) => document.querySelector<HTMLButtonElement>(`[data-testid="r-send-target-${index}"]`)!

describe('SendToAgentDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    host.load.mockResolvedValue(targets)
    host.deliver.mockResolvedValue(null)
  })

  it('groups Code before Chats and disables an agent on a prompt', async () => {
    await mountDialog()
    const text = document.querySelector('[data-testid="r-send-dialog"]')!.textContent
    expect(text.indexOf('Code')).toBeLessThan(text.indexOf('Chats'))
    expect(row(1).disabled).toBe(true)
    expect(row(1).textContent).toContain('Waiting on a prompt')
    expect(row(0).getAttribute('aria-selected')).toBe('false')
  })

  it('warns that an active agent gets the feedback mid-turn', async () => {
    await mountDialog()
    row(2).click()
    await flushPromises()
    expect(document.querySelector('[data-testid="r-send-warning"]')!.textContent).toContain('mid-turn')
  })

  it('sends to the pick and closes', async () => {
    const wrapper = await mountDialog()
    row(0).click()
    await flushPromises()
    document.querySelector<HTMLButtonElement>('[data-testid="r-send-submit"]')!.click()
    await flushPromises()
    expect(host.deliver).toHaveBeenCalledWith(targets[0])
    expect(toasts.showToast).toHaveBeenCalledWith('Sent to app / feat · claude')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('shows a failed send inline and stays open', async () => {
    host.deliver.mockResolvedValue('pane gone')
    const wrapper = await mountDialog()
    row(0).click()
    await flushPromises()
    document.querySelector<HTMLButtonElement>('[data-testid="r-send-submit"]')!.click()
    await flushPromises()
    expect(document.querySelector('[data-testid="r-send-error"]')!.textContent).toContain('pane gone')
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('says so when no agent is running', async () => {
    host.load.mockResolvedValue([])
    await mountDialog()
    expect(document.querySelector('[data-testid="r-send-empty"]')!.textContent).toContain('No agents are running.')
  })
})
