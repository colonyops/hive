import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetStores } from '../../../stores/defineStore'
import { usePopupTerminal } from '../../../stores/usePopupTerminal'
import AppletView from '../AppletView.vue'

const { rowsMock, runMock } = vi.hoisted(() => ({ rowsMock: vi.fn(), runMock: vi.fn() }))

vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice', () => ({
  Rows: rowsMock,
  Run: runMock,
  List: vi.fn(() => Promise.resolve([])),
}))

const table = {
  id: 'docker',
  title: 'Docker',
  icon: '',
  view: 'table',
  path: '/x/applet.yaml',
  requiresSession: false,
  refreshMillis: 0,
  runnable: true,
  error: '',
  columns: [
    { key: 'Names', label: 'Name', tones: {} },
    { key: 'State', label: 'State', tones: { running: 'success' } },
  ],
  actions: [
    { id: 'logs', label: 'Logs', mode: 'terminal', key: 'l', confirm: '' },
    { id: 'restart', label: 'Restart', mode: 'background', key: 'r', confirm: '' },
    { id: 'stop', label: 'Stop', mode: 'background', key: 's', confirm: 'Stop it?' },
  ],
}

const rows = [
  { Names: 'web', State: 'running', ID: 'a1' },
  { Names: 'db', State: 'exited', ID: 'b2' },
]

async function mountView(applet: object = table) {
  rowsMock.mockResolvedValue({ rows, truncated: false })
  const wrapper = mount(AppletView, {
    props: { applet: applet as never, sessionSlug: 'sess' },
    attachTo: document.body,
  })
  await flushPromises()
  return wrapper
}

function press(key: string): void {
  window.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))
}

describe('AppletView', () => {
  beforeEach(() => {
    resetStores()
    rowsMock.mockReset()
    runMock.mockReset()
    runMock.mockResolvedValue('')
  })

  it('renders rows from the data command in the session it opened on', async () => {
    const wrapper = await mountView()
    expect(rowsMock).toHaveBeenCalledWith('docker', 'sess')
    expect(wrapper.findAll('[data-testid="applet-row"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('web')
    wrapper.unmount()
  })

  it('runs a keyed action against the selected row and refreshes', async () => {
    const wrapper = await mountView()
    press('j')
    await flushPromises()
    press('r')
    await flushPromises()
    expect(runMock).toHaveBeenCalledWith('docker', 'restart', 'sess', rows[1])
    expect(rowsMock).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('asks first when the action declares a confirm', async () => {
    const wrapper = await mountView()
    press('s')
    await flushPromises()
    expect(runMock).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('Stop it?')
    wrapper.unmount()
  })

  it('opens a terminal action by identity, never by command', async () => {
    const wrapper = await mountView()
    press('Enter')
    await flushPromises()
    const popup = usePopupTerminal()
    expect(popup.request.value).toMatchObject({ applet: 'docker', appletAction: 'logs', sessionSlug: 'sess' })
    expect(popup.visible.value).toBe(true)
    expect(runMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('leaves keys alone while the pop-up terminal is on screen', async () => {
    const wrapper = await mountView()
    usePopupTerminal().show({})
    press('r')
    await flushPromises()
    expect(runMock).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps the last rows beside a failed refresh', async () => {
    const wrapper = await mountView()
    rowsMock.mockRejectedValue(new Error('daemon is down'))
    await wrapper.get('[data-testid="applet-refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="applet-load-error"]').text()).toContain('daemon is down')
    expect(wrapper.findAll('[data-testid="applet-row"]')).toHaveLength(2)
    wrapper.unmount()
  })

  it('fuzzy filters and types action keys into the filter rather than running them', async () => {
    const picker = {
      ...table,
      view: 'picker',
      actions: [{ id: 'run', label: 'Run', mode: 'background', key: 'r', confirm: '' }],
    }
    const wrapper = await mountView(picker)
    const input = wrapper.get('input')
    expect(document.activeElement).toBe(input.element)
    await input.setValue('db')
    expect(wrapper.findAll('[data-testid="applet-row"]')).toHaveLength(1)
    input.element.dispatchEvent(new KeyboardEvent('keydown', { key: 'r', bubbles: true }))
    await flushPromises()
    expect(runMock).not.toHaveBeenCalled()

    input.element.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await flushPromises()
    expect(runMock).toHaveBeenCalledWith('docker', 'run', 'sess', rows[1])
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })
})
