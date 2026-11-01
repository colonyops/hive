import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetStores } from '../../stores/defineStore'
import AppletsSettingsView from '../AppletsSettingsView.vue'

const service = vi.hoisted(() => ({
  CancelExport: vi.fn(),
  CancelImport: vi.fn(),
  SetEnabled: vi.fn(),
  ConfirmImport: vi.fn(),
  Delete: vi.fn(),
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

const settingsService = vi.hoisted(() => ({
  AppletSurface: vi.fn(),
  SetAppletSurface: vi.fn(),
}))

vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice',
  async (importOriginal) => ({ ...(await importOriginal<object>()), ...settingsService }),
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
  enabled: true,
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
    const wrapper = mount(AppletsSettingsView, { attachTo: document.body })
    await flushPromises()
    return wrapper
  }

  // The drawer and its menus teleport to the body.
  const inBody = <T extends HTMLElement = HTMLElement>(testid: string) =>
    document.querySelector<T>(`[data-testid="${testid}"]`)

  async function openDetails(wrapper: Awaited<ReturnType<typeof render>>, id: string) {
    await wrapper.get(`[data-testid="settings-applet-${id}-details"]`).trigger('click')
    await flushPromises()
  }

  async function openMore(id: string) {
    inBody(`settings-applet-${id}-more`)?.click()
    await flushPromises()
  }

  it('says whether each applet loaded, and why not', async () => {
    const wrapper = await render()
    const state = (id: string) => wrapper.get(`[data-testid="settings-applet-${id}-state"]`).text()

    expect(wrapper.find('[data-testid="settings-applet-docker-state"]').exists()).toBe(false)
    expect(state('stale')).toBe('Running last good file')
    expect(state('broken')).toBe('Invalid')
    expect(wrapper.get('[data-testid="settings-applet-broken"]').text()).toContain('title is required')
    expect(wrapper.get('[data-testid="settings-applet-applet-builder"]').text()).toContain('Ships with Hive')
    wrapper.unmount()
  })

  it('turns an applet off and back on, and an off applet cannot open', async () => {
    service.SetEnabled.mockResolvedValue(undefined)
    const wrapper = await render()
    await openDetails(wrapper, 'docker')
    const open = () => inBody<HTMLButtonElement>('settings-applet-docker-open')
    expect(open()?.disabled).toBe(false)

    service.List.mockResolvedValue([{ ...applet, enabled: false }])
    await wrapper.get('[data-testid="settings-applet-docker-enabled"]').trigger('click')
    await flushPromises()
    expect(service.SetEnabled).toHaveBeenCalledWith('docker', false)
    expect(wrapper.get('[data-testid="settings-applet-docker-state"]').text()).toBe('Off')
    expect(open()?.disabled).toBe(true)

    service.List.mockResolvedValue([applet])
    await wrapper.get('[data-testid="settings-applet-docker-enabled"]').trigger('click')
    await flushPromises()
    expect(service.SetEnabled).toHaveBeenCalledWith('docker', true)
    expect(wrapper.find('[data-testid="settings-applet-docker-state"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows why turning one off failed', async () => {
    service.SetEnabled.mockRejectedValue(new Error('saving settings failed'))
    const wrapper = await render()
    await wrapper.get('[data-testid="settings-applet-docker-enabled"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="settings-applets-share-error"]').text()).toContain('saving settings failed')
    wrapper.unmount()
  })

  describe('delete', () => {
    async function mountView() {
      const wrapper = mount(AppletsSettingsView, { attachTo: document.body })
      await flushPromises()
      return wrapper
    }
    const confirmButton = () =>
      document.querySelector<HTMLButtonElement>('[data-testid="settings-applet-delete-confirm"]')

    async function requestDelete(wrapper: Awaited<ReturnType<typeof render>>, id: string) {
      await openDetails(wrapper, id)
      await openMore(id)
      inBody(`settings-applet-${id}-delete`)?.click()
      await flushPromises()
    }

    it('offers delete for anything on disk, broken included, and not for a built-in', async () => {
      const wrapper = await mountView()
      for (const id of ['docker', 'broken']) {
        await openDetails(wrapper, id)
        await openMore(id)
        expect(inBody(`settings-applet-${id}-delete`)).not.toBeNull()
        inBody('settings-applet-details-close')?.click()
        await flushPromises()
      }
      await openDetails(wrapper, 'applet-builder')
      expect(inBody('settings-applet-applet-builder-more')).toBeNull()
      wrapper.unmount()
    })

    it('deletes nothing until the user confirms, then drops the row', async () => {
      service.Delete.mockResolvedValue(undefined)
      const wrapper = await mountView()
      await requestDelete(wrapper, 'docker')
      expect(service.Delete).not.toHaveBeenCalled()
      expect(document.body.textContent).toContain('Delete Docker?')

      service.List.mockResolvedValue([])
      confirmButton()?.click()
      await flushPromises()
      expect(service.Delete).toHaveBeenCalledWith('docker')
      expect(wrapper.find('[data-testid="settings-applet-docker"]').exists()).toBe(false)
      expect(inBody('settings-applet-details')).toBeNull()
      expect(confirmButton()).toBeNull()
      wrapper.unmount()
    })

    it('keeps the dialog open with the reason when the delete fails', async () => {
      service.Delete.mockRejectedValue(new Error('settings.yaml could not be cleared'))
      const wrapper = await mountView()
      await requestDelete(wrapper, 'docker')
      confirmButton()?.click()
      await flushPromises()
      expect(document.body.textContent).toContain('settings.yaml could not be cleared')
      expect(wrapper.find('[data-testid="settings-applet-docker"]').exists()).toBe(true)
      wrapper.unmount()
    })
  })

  it('lists what this Hive skipped in the file', async () => {
    service.List.mockResolvedValue([{ ...applet, warnings: ['this version of Hive ignores "sidebar"'] }])
    const wrapper = await render()
    expect(wrapper.get('[data-testid="settings-applet-docker-warnings"]').text()).toBe('1 warning')
    await openDetails(wrapper, 'docker')
    expect(inBody('settings-applet-details-warnings')?.textContent).toContain('"sidebar"')
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

  it('stops a process from the drawer of the applet that owns it', async () => {
    service.Processes.mockResolvedValue([
      { handle: 'p1', applet: 'docker', command: 'docker stats', startedMillis: 1 },
      { handle: 'p2', applet: 'stale', command: 'other', startedMillis: 1 },
    ])
    const wrapper = await render()
    await openDetails(wrapper, 'docker')
    expect(inBody('settings-applet-details-process-p2-stop')).toBeNull()

    inBody('settings-applet-details-process-p1-stop')?.click()
    await flushPromises()
    expect(service.SpawnKill).toHaveBeenCalledWith('p1')
    wrapper.unmount()
  })

  it('opens an applet from its drawer and closes the drawer', async () => {
    const wrapper = await render()
    await openDetails(wrapper, 'docker')
    inBody('settings-applet-docker-open')?.click()
    await flushPromises()
    expect(inBody('settings-applet-details')).toBeNull()
    wrapper.unmount()
  })

  it('shows inside the drawer why a change from it failed', async () => {
    service.SetEnabled.mockRejectedValue(new Error('saving settings failed'))
    const wrapper = await render()
    await openDetails(wrapper, 'docker')
    inBody('settings-applet-details-enabled')?.click()
    await flushPromises()
    expect(inBody('settings-applet-details-error')?.textContent).toContain('saving settings failed')
    wrapper.unmount()
  })

  describe('places', () => {
    const titlebar = { entry: 'bar.html', width: 28 }

    it('says panel only when an applet offers no place', async () => {
      const wrapper = await render()
      expect(wrapper.find('[data-testid="settings-applet-docker-places"]').exists()).toBe(false)
      expect(wrapper.get('[data-testid="settings-applet-docker"]').text()).toContain('Panel only')
      wrapper.unmount()
    })

    it('puts an applet in a place from the row menu, and the menu stays open', async () => {
      settingsService.AppletSurface.mockResolvedValue([])
      settingsService.SetAppletSurface.mockResolvedValue(undefined)
      service.List.mockResolvedValue([{ ...applet, titlebar }])
      const wrapper = await render()
      const toggle = wrapper.get('[data-testid="settings-applet-docker-places"]')
      expect(toggle.text()).toContain('Add to a place')

      await toggle.trigger('click')
      inBody('settings-applet-docker-places-titlebar')?.click()
      await flushPromises()
      expect(settingsService.SetAppletSurface).toHaveBeenCalledWith('titlebar', ['docker'])
      expect(toggle.text()).toContain('Title bar')
      expect(inBody('settings-applet-docker-places-menu')).not.toBeNull()
      wrapper.unmount()
    })

    it('locks the places of an applet that is off', async () => {
      service.List.mockResolvedValue([{ ...applet, titlebar, enabled: false }])
      const wrapper = await render()
      const toggle = wrapper.get('[data-testid="settings-applet-docker-places"]').element as HTMLButtonElement
      expect(toggle.disabled).toBe(true)
      wrapper.unmount()
    })
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
      const close = async () => {
        inBody('settings-applet-details-close')?.click()
        await flushPromises()
      }
      await openDetails(wrapper, 'docker')
      await openMore('docker')
      expect(inBody('settings-applet-docker-export')).not.toBeNull()
      await close()
      await openDetails(wrapper, 'broken')
      await openMore('broken')
      expect(inBody('settings-applet-broken-export')).toBeNull()
      await close()
      await openDetails(wrapper, 'applet-builder')
      expect(inBody('settings-applet-applet-builder-more')).toBeNull()
      wrapper.unmount()
    })

    it('shows what an export includes and leaves out', async () => {
      service.ExportPreview.mockResolvedValue({
        token: 'export-token',
        id: 'docker',
        title: 'Docker',
        fileName: 'docker.hiveapplet',
        files: [{ path: 'index.html', bytes: 10 }],
        leftOut: ['.localstorage.json'],
      })
      const wrapper = await mountView()
      await openDetails(wrapper, 'docker')
      await openMore('docker')
      inBody('settings-applet-docker-export')?.click()
      await flushPromises()
      expect(document.querySelector('[data-testid="applet-export-files"]')?.textContent).toContain('index.html')
      expect(document.querySelector('[data-testid="applet-export-left-out"]')?.textContent).toContain(
        '.localstorage.json',
      )
      wrapper.unmount()
      expect(service.CancelExport).toHaveBeenCalledWith('export-token')
    })

    it('shows the newest file when two are read at once', async () => {
      let finishFirst: (value: object) => void = () => {}
      service.PrepareImport.mockImplementationOnce(() => new Promise((resolve) => (finishFirst = resolve)))
      service.PrepareImport.mockResolvedValueOnce({
        token: 'second',
        id: 'b',
        suggestedId: 'b',
        title: 'Second',
        entry: 'index.html',
        files: [],
        totalBytes: 0,
      })
      const wrapper = await mountView()
      vi.useRealTimers()
      const input = wrapper.get('[data-testid="settings-applets-import-input"]').element as HTMLInputElement
      const choose = (name: string) => {
        Object.defineProperty(input, 'files', { value: [new File(['x'], name)], configurable: true })
        input.dispatchEvent(new Event('change'))
      }
      choose('first.hiveapplet')
      await vi.waitFor(() => expect(service.PrepareImport).toHaveBeenCalledTimes(1))
      choose('second.hiveapplet')
      await vi.waitFor(() =>
        expect(document.querySelector('[data-testid="applet-import-title"]')?.textContent).toBe('Second'),
      )
      finishFirst({
        token: 'first',
        id: 'a',
        suggestedId: 'a',
        title: 'First',
        entry: 'index.html',
        files: [],
        totalBytes: 0,
      })
      await flushPromises()
      expect(document.querySelector('[data-testid="applet-import-title"]')?.textContent).toBe('Second')
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
