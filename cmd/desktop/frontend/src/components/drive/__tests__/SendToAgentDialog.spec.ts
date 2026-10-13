import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SendToAgentDialog from '../SendToAgentDialog.vue'
import type {
  AgentTarget,
  AgentTargetRef,
} from '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

vi.mock('../../../composables/useWailsEvent', () => ({ useWailsEvent: vi.fn() }))

const toasts = vi.hoisted(() => ({ showToast: vi.fn() }))
vi.mock('../../../composables/useToasts', () => ({ useToasts: () => toasts }))

const reviews = vi.hoisted(() => ({
  Review: vi.fn().mockResolvedValue({ sessionId: 's', current: true, comments: [] }),
  AgentTargets: vi.fn(),
  Send: vi.fn(),
}))
vi.mock(
  '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reviewservice',
  () => reviews,
)

function target(kind: string, id: string, pane: string, label: string, status: string): AgentTarget {
  return { kind, id, pane, label, status }
}

const targets = [
  target('session', 's1', '%1', 'app / feat · claude', 'ready'),
  target('session', 's2', '%4', 'app / other · claude', 'approval'),
  target('chat', 'c1', '', 'ws / chat', 'active'),
]

async function mountDialog(agent: AgentTargetRef | null = null) {
  const wrapper = mount(SendToAgentDialog, {
    props: { space: 'workspaces/ws', path: 'plans/p.md', agent, testid: 'r' },
    attachTo: document.body,
  })
  await flushPromises()
  return wrapper
}

const row = (index: number) => document.querySelector<HTMLButtonElement>(`[data-testid="r-send-target-${index}"]`)!

describe('SendToAgentDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    reviews.AgentTargets.mockResolvedValue(targets)
    reviews.Send.mockResolvedValue(undefined)
  })

  it('groups Code before Chats and disables an agent on a prompt', async () => {
    await mountDialog()
    const text = document.querySelector('[data-testid="r-send-dialog"]')!.textContent
    expect(text.indexOf('Code')).toBeLessThan(text.indexOf('Chats'))
    expect(row(1).disabled).toBe(true)
    expect(row(1).textContent).toContain('Waiting on a prompt')
    expect(row(0).getAttribute('aria-checked')).toBe('false')
  })

  it('lists and preselects the surface agent first', async () => {
    await mountDialog({ kind: 'chat', id: 'c1', pane: '' })
    expect(row(0).textContent).toContain('ws / chat')
    expect(row(0).getAttribute('aria-checked')).toBe('true')
    expect(document.querySelector('[data-testid="r-send-warning"]')!.textContent).toContain('mid-turn')
  })

  it('does not preselect an agent on a prompt', async () => {
    await mountDialog({ kind: 'session', id: 's2', pane: '' })
    expect(document.querySelector('[aria-checked="true"]')).toBeNull()
  })

  it('sends to the pick and closes', async () => {
    const wrapper = await mountDialog({ kind: 'session', id: 's1', pane: '' })
    document.querySelector<HTMLButtonElement>('[data-testid="r-send-submit"]')!.click()
    await flushPromises()
    expect(reviews.Send).toHaveBeenCalledWith('workspaces/ws', 'plans/p.md', '', {
      kind: 'session',
      id: 's1',
      pane: '%1',
    })
    expect(toasts.showToast).toHaveBeenCalledWith('Sent to app / feat · claude')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('shows a failed send inline and stays open', async () => {
    reviews.Send.mockRejectedValue(new Error('pane gone'))
    const wrapper = await mountDialog({ kind: 'session', id: 's1', pane: '' })
    document.querySelector<HTMLButtonElement>('[data-testid="r-send-submit"]')!.click()
    await flushPromises()
    expect(document.querySelector('[data-testid="r-send-error"]')!.textContent).toContain('pane gone')
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('says so when no agent is running', async () => {
    reviews.AgentTargets.mockResolvedValue([])
    await mountDialog()
    expect(document.querySelector('[data-testid="r-send-empty"]')!.textContent).toContain('No agents are running.')
  })
})
