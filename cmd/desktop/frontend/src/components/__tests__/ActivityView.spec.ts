import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import type { Event as ActivityEvent } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/activity/models'

const markSeen = vi.fn()
const load = vi.fn()
const events = ref<ActivityEvent[]>([])

const openFromActivity = vi.fn()
vi.mock('../../composables/useNewSession', () => ({
  useNewSession: () => ({ openFromActivity }),
}))

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
    {
      id: 4,
      createdAt: now - 1000,
      category: 'refresh',
      severity: 'info',
      title: 'Refreshed github:hive/core',
      body: '12 items updated',
    },
    {
      id: 3,
      createdAt: now - 2000,
      category: 'refresh',
      severity: 'error',
      title: 'Refresh failed for rpc:Sentry',
      body: 'exit 1',
    },
    {
      id: 2,
      createdAt: now - 3000,
      category: 'session',
      severity: 'success',
      title: 'Created session review-pr-1',
      body: 'sonnet',
    },
    {
      id: 1,
      createdAt: now - 4000,
      category: 'auto_action',
      severity: 'auto',
      title: 'Auto-action · Triage',
      body: 'rule triage.default',
    },
  ]
}

function rows(wrapper: ReturnType<typeof mount>) {
  return wrapper.findAll('[data-testid="activity-row"]')
}

// What a failed create records: the reason in the body, the form in metadata.
const failedCreate: ActivityEvent = {
  id: 9,
  createdAt: Date.now(),
  category: 'session',
  severity: 'error',
  title: 'Could not create session fix-crash',
  body: 'acme/site · Cloning repository... · exit status 1',
  metadata: {
    retry: 'session-create',
    repository: 'https://github.com/acme/site.git',
    name: 'fix-crash',
    prompt: 'Fix the crash',
    agent: 'claude',
  },
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

  // #439: hue in the ledger means severity and nothing else. The seed covers a
  // failure, an auto-action and two ordinary categories, so a category hue
  // creeping back in shows up here.
  it('paints only failures and auto-actions, leaving every other row neutral', () => {
    events.value = seed()
    const wrapper = mount(ActivityView)
    const dots = wrapper.findAll('[data-testid="activity-row"] span.rounded-full')

    const dotClass = (i: number) => dots[i].attributes('class') ?? ''
    // Rows are newest-first: refresh, failed refresh, session, auto-action.
    expect(dotClass(0)).toContain('bg-text-4')
    expect(dotClass(1)).toContain('bg-severity-error')
    expect(dotClass(2)).toContain('bg-text-4')
    expect(dotClass(3)).toContain('bg-accent')

    const html = wrapper.html()
    for (const banned of ['bg-severity-success', 'bg-node-purple', 'bg-severity-info']) {
      expect(html).not.toContain(banned)
    }
  })

  // Only a failure earns the tinted row. Auto-actions keep their dot but not
  // the rail: they are the most common event here, so railing them tinted the
  // whole page and highlighted nothing.
  it('rails the failure only, leaving auto-actions to their dot', () => {
    events.value = seed()
    const wrapper = mount(ActivityView)
    const r = rows(wrapper)
    expect(r[0].attributes('class')).toContain('hover:bg-row-hover')
    expect(r[1].attributes('class')).toContain('bg-severity-error-tint')
    expect(r[2].attributes('class')).toContain('hover:bg-row-hover')
    expect(r[3].attributes('class')).toContain('hover:bg-row-hover')
    expect(r[3].attributes('class')).not.toContain('bg-accent-tint')
  })

  // A log made entirely of auto-actions must not come out as a wall of tint.
  it('leaves a page of nothing but auto-actions untinted', () => {
    const now = Date.now()
    events.value = Array.from({ length: 6 }, (_, i) => ({
      id: i + 1,
      createdAt: now - i * 1000,
      category: 'auto_action',
      severity: 'auto',
      title: 'Auto-action · Outside contributor',
      body: 'rule notify:personal/notify-external',
    }))
    const wrapper = mount(ActivityView)

    const r = rows(wrapper)
    expect(r).toHaveLength(6)
    for (const row of r) {
      expect(row.attributes('class')).toContain('hover:bg-row-hover')
    }
    expect(wrapper.html()).not.toContain('bg-accent-tint')
  })

  it('emits close on Escape', async () => {
    events.value = seed()
    const wrapper = mount(ActivityView)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('offers keyboard-reachable internal and external links from event metadata', async () => {
    const linked: ActivityEvent = {
      id: 10,
      createdAt: Date.now(),
      category: 'auto_action',
      severity: 'auto',
      title: 'Auto-action · My PR approved',
      metadata: {
        'link.url': 'https://github.com/grafana/adaptive-telemetry-archive-replay/pull/227',
        'link.item.profileId': 'triage',
        'link.item.sourceKind': 'github',
        'link.item.sourceScope': 'grafana/adaptive-telemetry-archive-replay',
        'link.item.externalId': 'grafana/adaptive-telemetry-archive-replay#227',
      },
    }
    events.value = [linked]
    const wrapper = mount(ActivityView)

    const itemButton = wrapper.get('[data-testid="activity-open-item-10"]')
    const urlButton = wrapper.get('[data-testid="activity-open-url-10"]')
    expect(itemButton.element.tagName).toBe('BUTTON')
    expect(urlButton.element.tagName).toBe('BUTTON')

    await itemButton.trigger('click')
    await urlButton.trigger('click')
    expect(wrapper.emitted('open-item')).toEqual([
      [
        {
          profileId: 'triage',
          sourceKind: 'github',
          sourceScope: 'grafana/adaptive-telemetry-archive-replay',
          externalId: 'grafana/adaptive-telemetry-archive-replay#227',
        },
      ],
    ])
    expect(wrapper.emitted('open-url')).toEqual([
      ['https://github.com/grafana/adaptive-telemetry-archive-replay/pull/227'],
    ])
  })

  it('does not render link controls for an event without link metadata', () => {
    events.value = seed()
    const wrapper = mount(ActivityView)
    expect(wrapper.find('[data-testid^="activity-open-item-"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid^="activity-open-url-"]').exists()).toBe(false)
  })

  it('offers Retry only on a row that carries a form, and forwards its metadata untouched', async () => {
    events.value = [failedCreate, ...seed()]
    const wrapper = mount(ActivityView)

    expect(wrapper.findAll('[data-testid^="activity-retry-"]')).toHaveLength(1)
    await wrapper.get('[data-testid="activity-retry-9"]').trigger('click')

    expect(openFromActivity).toHaveBeenCalledWith(failedCreate.metadata)
    // The form is a modal over the view behind this one, so the overlay closes.
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('offers no Retry on a failure with nothing to retry', () => {
    events.value = seed()
    const wrapper = mount(ActivityView)
    expect(wrapper.find('[data-testid^="activity-retry-"]').exists()).toBe(false)
  })
})
