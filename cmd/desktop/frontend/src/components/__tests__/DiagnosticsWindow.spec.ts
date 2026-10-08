import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AppDateTimePicker from '../ui/AppDateTimePicker.vue'
import DiagnosticsWindow from '../DiagnosticsWindow.vue'

const api = vi.hoisted(() => ({
  Read: vi.fn(),
  Agents: vi.fn(),
  Prepare: vi.fn(),
  Context: vi.fn(),
  Save: vi.fn(),
  Reveal: vi.fn(),
  copy: vi.fn(),
}))
vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/diagnosticsservice',
  () => api,
)
vi.mock('@wailsio/runtime', () => ({ Clipboard: { SetText: api.copy } }))
vi.mock('../DiagnosticsAgent.vue', () => ({
  default: { template: '<div data-testid="agent-terminal">Agent terminal</div>' },
}))

const query = {
  source: '',
  since: '',
  until: '2026-10-07T01:00:00Z',
  level: '',
  search: '',
  reference: '',
  limit: 500,
  omitRoutine: true,
}
const snapshot = {
  version: 'test',
  query,
  capturedAt: query.until,
  truncated: false,
  sources: [
    { id: 'desktop', path: '/logs/hive.log', updatedAt: query.until, error: '', truncated: false },
    { id: 'cli', path: '/logs/hive.log', updatedAt: query.until, error: '', truncated: false },
    { id: 'jobs', path: '', updatedAt: '', error: '', truncated: false },
  ],
  entries: [
    {
      id: 'job-5978',
      time: '2026-10-07T00:17:17Z',
      source: 'jobs',
      level: 'error',
      message: 'review 347 already exists',
      fields: { job_id: '5978', status: 'failed' },
      raw: 'duplicate session',
      truncated: false,
    },
  ],
}

describe('DiagnosticsWindow', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    localStorage.clear()
    api.Read.mockResolvedValue(snapshot)
    api.Agents.mockResolvedValue({ names: ['codex'], defaultAgent: 'codex' })
    api.Context.mockResolvedValue({ text: 'incident evidence' })
    api.Prepare.mockRejectedValue(new Error('Agent unavailable'))
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('opens the log viewer without a terminal and survives agent preparation failure', async () => {
    const wrapper = mount(DiagnosticsWindow)
    await flushPromises()
    expect(wrapper.find('[data-testid="diagnostics-terminal-panel"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="diagnostics-source-log"]').text()).toContain('hive.log')
    expect(wrapper.text()).toContain('review 347 already exists')
    expect(wrapper.text()).toContain('job_id')
    expect(api.Read).toHaveBeenCalledWith(
      expect.objectContaining({ source: '', level: '', search: '', limit: 500, omitRoutine: true }),
    )
    expect(api.Prepare).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="diagnostics-investigate"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Agent unavailable')
    expect(wrapper.text()).toContain('review 347 already exists')

    await wrapper.get('[data-testid="diagnostics-actions"]').trigger('click')
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Copy context')!
      .trigger('click')
    await flushPromises()
    expect(api.copy).toHaveBeenCalledWith('incident evidence')
    expect(api.Context).toHaveBeenCalledWith(expect.objectContaining({ query }))
    wrapper.unmount()
  })

  it('pauses automatic reads and keeps evidence when a later read fails', async () => {
    const wrapper = mount(DiagnosticsWindow)
    await flushPromises()
    await wrapper.get('[data-testid="diagnostics-live-follow"]').trigger('click')
    await vi.advanceTimersByTimeAsync(4000)
    expect(api.Read).toHaveBeenCalledTimes(1)

    api.Read.mockRejectedValueOnce(new Error('Read failed'))
    await wrapper.get('[data-testid="diagnostics-refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Read failed')
    expect(wrapper.text()).toContain('review 347 already exists')
    wrapper.unmount()
  })

  it('offers a floating shortcut after the log is scrolled away from the top', async () => {
    const wrapper = mount(DiagnosticsWindow)
    await flushPromises()
    const entries = wrapper.get('[data-testid="diagnostics-entries"]')
    const scrollTo = vi.fn()
    entries.element.scrollTo = scrollTo
    entries.element.scrollTop = 240
    await entries.trigger('scroll')

    await wrapper.get('[data-testid="diagnostics-scroll-to-top"]').trigger('click')
    expect(scrollTo).toHaveBeenCalledWith({ top: 0, behavior: 'smooth' })
    wrapper.unmount()
  })

  it('opens the investigation terminal only after preparation succeeds', async () => {
    api.Prepare.mockResolvedValueOnce({ command: 'codex prompt', dir: '/tmp/report' })
    const wrapper = mount(DiagnosticsWindow)
    await flushPromises()
    await wrapper.get('[data-testid="diagnostics-incident-description"]').setValue('The window disappeared')
    await wrapper.get('[data-testid="diagnostics-investigate"]').trigger('click')
    await flushPromises()

    expect(api.Prepare).toHaveBeenCalledWith(expect.objectContaining({ description: 'The window disappeared' }))
    expect(wrapper.find('[data-testid="diagnostics-terminal-panel"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="agent-terminal"]').text()).toContain('Agent terminal')
    expect(wrapper.find('[data-testid="diagnostics-incident-description"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="diagnostics-logs-panel"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="resize-handle-diagnostics-terminal"]').attributes('aria-orientation')).toBe(
      'horizontal',
    )

    await wrapper.get('[data-testid="diagnostics-layout-side-by-side"]').trigger('click')
    expect(wrapper.get('[data-testid="resize-handle-diagnostics-terminal"]').attributes('aria-orientation')).toBe(
      'vertical',
    )
    expect(localStorage.getItem('hive.diagnostics.layout')).toBe('side-by-side')
    wrapper.unmount()
  })

  it('applies a custom time range from the popover', async () => {
    const wrapper = mount(DiagnosticsWindow, { attachTo: document.body })
    await flushPromises()
    await wrapper.get('[data-testid="diagnostics-time-range-toggle"]').trigger('click')
    await flushPromises()
    const popover = document.querySelector('[data-testid="diagnostics-time-range"]')!
    expect(popover).not.toBeNull()
    const pickers = wrapper.findAllComponents(AppDateTimePicker)
    pickers[0].vm.$emit('update:modelValue', '2026-10-07T00:00')
    pickers[1].vm.$emit('update:modelValue', '2026-10-07T01:00')
    await wrapper.vm.$nextTick()
    popover.querySelector<HTMLButtonElement>('[data-testid="diagnostics-time-apply"]')?.click()
    await vi.advanceTimersByTimeAsync(300)
    expect(api.Read).toHaveBeenLastCalledWith(
      expect.objectContaining({
        since: new Date('2026-10-07T00:00').toISOString(),
        until: new Date('2026-10-07T01:00').toISOString(),
      }),
    )
    wrapper.unmount()
  })
})
