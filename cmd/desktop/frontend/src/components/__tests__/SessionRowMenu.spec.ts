import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import SessionRowMenu from '../SessionRowMenu.vue'
import type { SessionSummary } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const session: SessionSummary = {
  id: 's1',
  name: 'review 81',
  slug: 'review-81',
  repo: 'acme/site',
  state: 'active',
  canvasOwner: 'acme/site',
  createdAt: '2026-10-01T12:00:00Z',
}

function mountMenu(props: Partial<InstanceType<typeof SessionRowMenu>['$props']> = {}) {
  return mount(SessionRowMenu, { props: { session, ...props } })
}

describe('SessionRowMenu', () => {
  it('emits the operation each entry stands for and closes', async () => {
    const wrapper = mountMenu()

    for (const [testid, event] of [
      ['session-menu-start', 'start'],
      ['session-menu-kill', 'kill'],
      ['session-menu-detail', 'detail'],
      ['session-menu-rename', 'rename'],
      ['session-menu-recycle', 'recycle'],
      ['session-menu-delete', 'delete'],
    ] as const) {
      await wrapper.get(`[data-testid="${testid}"]`).trigger('click')
      expect(wrapper.emitted(event), testid).toHaveLength(1)
    }
    expect(wrapper.emitted('close')).toHaveLength(6)
  })

  it('offers the terminal lifecycle and recycle only for an active session', () => {
    const active = mountMenu()
    expect(active.find('[data-testid="session-menu-start"]').exists()).toBe(true)
    expect(active.find('[data-testid="session-menu-kill"]').exists()).toBe(true)
    expect(active.find('[data-testid="session-menu-recycle"]').exists()).toBe(true)

    // Hive rejects recycling a session that is not active, and a recycled one
    // has no checkout left to run a terminal in.
    const recycled = mountMenu({ session: { ...session, state: 'recycled' } })
    expect(recycled.find('[data-testid="session-menu-start"]').exists()).toBe(false)
    expect(recycled.find('[data-testid="session-menu-kill"]').exists()).toBe(false)
    expect(recycled.find('[data-testid="session-menu-recycle"]').exists()).toBe(false)
    expect(recycled.find('[data-testid="session-menu-delete"]').exists()).toBe(true)
  })

  // The scratch terminal's tmux session is the whole thing: there is no record
  // to rename or delete, and no checkout to recycle or read details of.
  it('carries only the terminal lifecycle for the scratch terminal', async () => {
    const wrapper = mountMenu({ scratch: true })

    expect(wrapper.get('[data-testid="session-menu-start"]').text()).toBe('Start terminal')
    for (const entry of ['detail', 'rename', 'recycle', 'delete']) {
      expect(wrapper.find(`[data-testid="session-menu-${entry}"]`).exists()).toBe(false)
    }

    await wrapper.get('[data-testid="session-menu-kill"]').trigger('click')
    expect(wrapper.emitted('kill')).toHaveLength(1)
  })

  it('appends host entries in their own section and reports them by id', async () => {
    const wrapper = mountMenu({
      extra: [{ kind: 'action', id: 'new-tab:claude', label: 'New Claude tab', testid: 'extra-claude' }],
      extraLabel: 'New tab',
    })

    expect(wrapper.text()).toContain('New tab')
    await wrapper.get('[data-testid="extra-claude"]').trigger('click')
    expect(wrapper.emitted('extra')).toEqual([['new-tab:claude']])
  })
})
