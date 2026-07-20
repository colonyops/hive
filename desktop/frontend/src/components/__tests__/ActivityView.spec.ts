import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import type { Event as ActivityEvent } from '../../../bindings/github.com/colonyops/hive/internal/desktop/activity/models'

const markSeen = vi.fn()
const load = vi.fn()
const events = ref<ActivityEvent[]>([])

vi.mock('../../composables/useActivity', () => ({
  useActivity: () => ({
    events,
    loading: ref(false),
    error: ref<string | null>(null),
    load,
    markSeen,
  }),
}))

import ActivityView from '../ActivityView.vue'

function seed(): ActivityEvent[] {
  const now = Date.now()
  return [
    { id: 4, createdAt: now - 1000, category: 'refresh', severity: 'info', title: 'Refreshed github:hive/core', body: '12 items updated' },
    { id: 3, createdAt: now - 2000, category: 'refresh', severity: 'error', title: 'Refresh failed for rpc:Sentry', body: 'exit 1' },
    { id: 2, createdAt: now - 3000, category: 'session', severity: 'success', title: 'Created session review-pr-1', body: 'sonnet' },
    { id: 1, createdAt: now - 4000, category: 'auto_action', severity: 'auto', title: 'Auto-action · Triage', body: 'rule triage.default' },
  ]
}

function rows(wrapper: ReturnType<typeof mount>) {
  return wrapper.findAll('[data-testid="activity-row"]')
}

describe('ActivityView', () => {
  it('clears the unseen marker on open and shows every event under All', () => {
    events.value = seed()
    const wrapper = mount(ActivityView)
    expect(markSeen).toHaveBeenCalled()
    expect(rows(wrapper)).toHaveLength(4)
  })

  it('filters to errors by severity, not category', async () => {
    events.value = seed()
    const wrapper = mount(ActivityView)
    await wrapper.find('[data-testid="activity-filter-error"]').trigger('click')
    const r = rows(wrapper)
    expect(r).toHaveLength(1)
    expect(r[0].text()).toContain('Refresh failed for rpc:Sentry')
  })

  it('filters by category for the session/auto/refresh pills', async () => {
    events.value = seed()
    const wrapper = mount(ActivityView)
    await wrapper.find('[data-testid="activity-filter-session"]').trigger('click')
    expect(rows(wrapper)).toHaveLength(1)
    expect(rows(wrapper)[0].text()).toContain('Created session review-pr-1')
  })

  it('filters by the search box across title and body', async () => {
    events.value = seed()
    const wrapper = mount(ActivityView)
    await wrapper.find('[data-testid="activity-search"]').setValue('triage')
    expect(rows(wrapper)).toHaveLength(1)
    expect(rows(wrapper)[0].text()).toContain('Auto-action')
  })

  it('shows the empty state when nothing matches', async () => {
    events.value = seed()
    const wrapper = mount(ActivityView)
    await wrapper.find('[data-testid="activity-search"]').setValue('nothing-matches-this')
    expect(rows(wrapper)).toHaveLength(0)
    expect(wrapper.find('[data-testid="activity-empty"]').exists()).toBe(true)
  })

  it('emits close from the back button', async () => {
    events.value = seed()
    const wrapper = mount(ActivityView)
    await wrapper.find('[data-testid="activity-close"]').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
