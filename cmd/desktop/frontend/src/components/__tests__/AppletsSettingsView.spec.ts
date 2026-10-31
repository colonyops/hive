import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetStores } from '../../stores/defineStore'
import AppletsSettingsView from '../AppletsSettingsView.vue'

const service = vi.hoisted(() => ({
  CancelImport: vi.fn(),
  ConfirmImport: vi.fn(),
  Dir: vi.fn(),
  Export: vi.fn(),
  ExportPreview: vi.fn(),
  List: vi.fn(),
  PrepareImport: vi.fn(),
  Processes: vi.fn(),
  SpawnKill: vi.fn(),
}))

vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice',
  () => service,
)

const applet = {
  id: 'docker',
  title: 'Docker',
  icon: 'container',
  path: '/applets/docker/applet.yaml',
  builtin: false,
  entry: 'index.html',
  width: 0,
  height: 0,
  revision: 1,
  runnable: true,
  error: '',
}

describe('AppletsSettingsView', () => {
  beforeEach(() => {
    resetStores()
    vi.useFakeTimers()
    service.PrepareImport.mockReset()
    service.Dir.mockResolvedValue('/applets')
    service.SpawnKill.mockResolvedValue(undefined)
    service.Processes.mockResolvedValue([])
    service.List.mockResolvedValue([
      applet,
      { ...applet, id: 'applet-builder', title: 'Applet Builder', builtin: true },
      { ...applet, id: 'stale', title: 'Stale', error: 'line 2: bad' },
      { ...applet, id: 'broken', title: 'Broken', runnable: false, error: 'title is required' },
    ])
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  async function render() {
    const wrapper = mount(AppletsSettingsView)
    await flushPromises()
    return wrapper
  }

  it('says whether each applet loaded, and why not', async () => {
    const wrapper = await render()
    const state = (id: string) => wrapper.get(`[data-testid="settings-applet-${id}-state"]`).text()

    expect(state('docker')).toBe('Ready')
    expect(state('stale')).toBe('Running last good file')
    expect(state('broken')).toBe('Invalid')
    expect(wrapper.get('[data-testid="settings-applet-broken"]').text()).toContain('title is required')
    expect(wrapper.get('[data-testid="settings-applet-applet-builder"]').text()).toContain('ships with Hive')
    wrapper.unmount()
  })

  it('shows what is running, and stops it on request', async () => {
    const process = { handle: 'p1', applet: 'docker', command: 'docker stats', startedMillis: 1 }
    service.Processes.mockResolvedValue([process])
    const wrapper = await render()

    expect(wrapper.get('[data-testid="settings-applet-docker"]').text()).toContain('1 running')
    expect(wrapper.get('[data-testid="settings-applet-process-p1"]').text()).toContain('docker stats')

    service.Processes.mockResolvedValue([])
    await wrapper.get('[data-testid="settings-applet-process-p1-stop"]').trigger('click')
    await flushPromises()
    expect(service.SpawnKill).toHaveBeenCalledWith('p1')
    expect(wrapper.find('[data-testid="settings-applets-processes-empty"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('notices a process that starts while the page is open', async () => {
    const wrapper = await render()
    expect(wrapper.find('[data-testid="settings-applets-processes-empty"]').exists()).toBe(true)

    service.Processes.mockResolvedValue([{ handle: 'p2', applet: 'git', command: 'x', startedMillis: 1 }])
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(wrapper.find('[data-testid="settings-applet-process-p2"]').exists()).toBe(true)
    wrapper.unmount()
  })

  describe('sharing', () => {
    async function mountView() {
      const wrapper = mount(AppletsSettingsView, { attachTo: document.body })
      await flushPromises()
      return wrapper
    }

    it('offers export only for an applet there is something to share of', async () => {
      const wrapper = await mountView()
      expect(wrapper.find('[data-testid="settings-applet-docker-export"]').exists()).toBe(true)
      expect(wrapper.find('[data-testid="settings-applet-applet-builder-export"]').exists()).toBe(false)
      expect(wrapper.find('[data-testid="settings-applet-broken-export"]').exists()).toBe(false)
      wrapper.unmount()
    })

    it('shows what an export includes and leaves out', async () => {
      service.ExportPreview.mockResolvedValue({
        id: 'docker',
        title: 'Docker',
        fileName: 'docker.hiveapplet',
        files: [{ path: 'index.html', bytes: 10 }],
        leftOut: ['.localstorage.json'],
      })
      const wrapper = await mountView()
      await wrapper.get('[data-testid="settings-applet-docker-export"]').trigger('click')
      await flushPromises()
      expect(document.querySelector('[data-testid="applet-export-files"]')?.textContent).toContain('index.html')
      expect(document.querySelector('[data-testid="applet-export-left-out"]')?.textContent).toContain(
        '.localstorage.json',
      )
      wrapper.unmount()
    })

    it('shows why a file was refused, and opens no dialog', async () => {
      service.PrepareImport.mockRejectedValue(new Error('this file has a path outside the applet'))
      const wrapper = await mountView()
      vi.useRealTimers()
      const input = wrapper.get('[data-testid="settings-applets-import-input"]').element as HTMLInputElement
      Object.defineProperty(input, 'files', { value: [new File(['x'], 'bad.hiveapplet')], configurable: true })
      input.dispatchEvent(new Event('change'))
      await vi.waitFor(() =>
        expect(wrapper.find('[data-testid="settings-applets-share-error"]').text()).toContain('outside the applet'),
      )
      expect(document.querySelector('[data-testid="applet-import"]')).toBeNull()
      wrapper.unmount()
    })

    it('refuses a file over the limit without reading it', async () => {
      const wrapper = await mountView()
      vi.useRealTimers()
      const input = wrapper.get('[data-testid="settings-applets-import-input"]').element as HTMLInputElement
      const big = new File(['x'], 'big.hiveapplet')
      Object.defineProperty(big, 'size', { value: (32 << 20) + 1 })
      Object.defineProperty(input, 'files', { value: [big], configurable: true })
      input.dispatchEvent(new Event('change'))
      await vi.waitFor(() =>
        expect(wrapper.find('[data-testid="settings-applets-share-error"]').text()).toContain('32 MB'),
      )
      expect(service.PrepareImport).not.toHaveBeenCalled()
      wrapper.unmount()
    })
  })
})
