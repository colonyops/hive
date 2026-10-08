import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
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
vi.mock('../DiagnosticsAgent.vue', () => ({ default: { template: '<div>Agent terminal</div>' } }))

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
    { id: 'desktop', path: '/logs/desktop.log', updatedAt: query.until, error: '', truncated: false },
    { id: 'cli', path: '/logs/hive.log', error: 'File unavailable' },
  ],
  entries: [
    {
      id: 'job-5978',
      time: '2026-10-07T00:17:17Z',
      source: 'jobs',
      level: 'error',
      message: 'review 347 already exists',
      raw: 'duplicate session',
      truncated: false,
    },
  ],
}

describe('DiagnosticsWindow', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    api.Read.mockResolvedValue(snapshot)
    api.Agents.mockResolvedValue({ names: ['codex'], defaultAgent: 'codex' })
    api.Context.mockResolvedValue({ text: 'incident evidence' })
    api.Prepare.mockRejectedValue(new Error('Agent unavailable'))
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('opens evidence without starting an agent and survives agent preparation failure', async () => {
    const wrapper = mount(DiagnosticsWindow)
    await flushPromises()
    expect(wrapper.text()).toContain('review 347 already exists')
    expect(wrapper.text()).toContain('File unavailable')
    expect(api.Prepare).not.toHaveBeenCalled()
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Investigate')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Agent unavailable')
    expect(wrapper.text()).toContain('review 347 already exists')
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Copy context')!
      .trigger('click')
    await flushPromises()
    expect(api.copy).toHaveBeenCalledWith('incident evidence')
    expect(wrapper.text()).toContain('Agent unavailable')
    expect(api.Context).toHaveBeenCalledWith(expect.objectContaining({ query }))
    wrapper.unmount()
  })

  it('pauses automatic reads and keeps evidence when a later read fails', async () => {
    const wrapper = mount(DiagnosticsWindow)
    await flushPromises()
    await wrapper.findAll('input[type=checkbox]')[0]!.setValue(false)
    await vi.advanceTimersByTimeAsync(4000)
    expect(api.Read).toHaveBeenCalledTimes(1)
    api.Read.mockRejectedValueOnce(new Error('Read failed'))
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Refresh')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Read failed')
    expect(wrapper.text()).toContain('review 347 already exists')
    wrapper.unmount()
  })
})
