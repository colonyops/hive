import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useKeybindings } from '../../../composables/useKeybindings'
import { resetStores } from '../../../stores/defineStore'
import { useApplets } from '../../../stores/useApplets'
import AppletImportDialog from '../AppletImportDialog.vue'

const service = vi.hoisted(() => ({
  CancelImport: vi.fn(),
  ConfirmImport: vi.fn(),
  List: vi.fn(),
}))

vi.mock(
  '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice',
  () => service,
)

const preview = {
  token: 'token',
  id: 'radio',
  suggestedId: 'radio',
  title: 'Radio',
  entry: 'index.html',
  cwd: '',
  titlebar: { entry: 'bar.html', width: 28 },
  commands: [{ id: 'next', title: 'Next station', icon: '', keywords: null }],
  files: [{ path: 'applet.yaml', bytes: 20 }],
  totalBytes: 20,
}

function open(overrides: object = {}) {
  return mount(AppletImportDialog, {
    props: { preview: { ...preview, ...overrides } as never },
    attachTo: document.body,
  })
}

const byID = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`)

async function tickTrust() {
  const box = byID('applet-import-trust') as HTMLInputElement
  box.checked = true
  box.dispatchEvent(new Event('change'))
  await flushPromises()
}

describe('AppletImportDialog', () => {
  beforeEach(() => {
    resetStores()
    for (const mock of Object.values(service)) mock.mockReset()
    service.List.mockResolvedValue([])
    service.ConfirmImport.mockResolvedValue(undefined)
  })

  it('says what an applet can do, and what this one would run', async () => {
    const wrapper = open()
    await flushPromises()
    const warning = byID('applet-import-warning')?.textContent
    expect(warning).toContain('run any command or program')
    expect(warning).toContain('read, change and delete any of your files')
    expect(warning).toContain('reach the network')
    const runs = byID('applet-import-runs')?.textContent
    expect(runs).toContain('Nothing runs on import')
    expect(runs).toContain('bar.html')
    expect(runs).toContain('stays off')
    expect(runs).toContain('Next station')
    expect(byID('applet-import-files')?.textContent).toContain('applet.yaml')
    wrapper.unmount()
  })

  it('starts on cancel, and importing takes the tick', async () => {
    const wrapper = open()
    await flushPromises()
    expect(document.activeElement).toBe(byID('applet-import-cancel'))
    const confirm = byID('applet-import-confirm') as HTMLButtonElement
    expect(confirm.disabled).toBe(true)
    confirm.click()
    await flushPromises()
    expect(service.ConfirmImport).not.toHaveBeenCalled()
    await tickTrust()
    expect(confirm.disabled).toBe(false)
    wrapper.unmount()
  })

  it('drops the waiting file on cancel', async () => {
    const wrapper = open()
    await flushPromises()
    byID('applet-import-cancel')?.click()
    await flushPromises()
    expect(service.CancelImport).toHaveBeenCalledWith('token')
    expect(service.ConfirmImport).not.toHaveBeenCalled()
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('imports under the id offered when the one asked for is taken', async () => {
    const wrapper = open({ suggestedId: 'radio-2' })
    await flushPromises()
    expect(byID('applet-import-conflict')?.textContent).toContain('will not be replaced')
    await tickTrust()
    byID('applet-import-confirm')?.click()
    await flushPromises()
    expect(service.ConfirmImport).toHaveBeenCalledWith('token', 'radio-2')
    wrapper.unmount()
  })

  it('refuses an id in the catalog and takes nothing from the applet there', async () => {
    service.List.mockResolvedValue([{ id: 'radio', title: 'Mine', runnable: true, titlebar: { entry: 'b', width: 1 } }])
    const applets = useApplets()
    await applets.reload()
    applets.setInTitlebar('radio', true)
    const wrapper = open()
    await flushPromises()
    await tickTrust()
    byID('applet-import-confirm')?.click()
    await flushPromises()
    expect(service.ConfirmImport).not.toHaveBeenCalled()
    expect(byID('applet-import-error')?.textContent).toContain('already exists')
    expect(applets.isInTitlebar('radio')).toBe(true)
    wrapper.unmount()
  })

  it('gives the new applet nothing an old one of that id had', async () => {
    const applets = useApplets()
    const keybindings = useKeybindings()
    applets.setInTitlebar('radio', true)
    // A setting the old applet left behind: the id is not in the catalog any more.
    keybindings.removeBinding('applet.radio', 'mod+r')
    expect(keybindings.isOverridden('applet.radio')).toBe(true)
    const wrapper = open()
    await flushPromises()
    await tickTrust()
    byID('applet-import-confirm')?.click()
    await flushPromises()
    expect(applets.isInTitlebar('radio')).toBe(false)
    expect(keybindings.isOverridden('applet.radio')).toBe(false)
    expect(service.ConfirmImport).toHaveBeenCalledWith('token', 'radio')
    wrapper.unmount()
  })

  it('shows why an import failed and stays open', async () => {
    service.ConfirmImport.mockRejectedValue(new Error('disk full'))
    const wrapper = open()
    await flushPromises()
    await tickTrust()
    byID('applet-import-confirm')?.click()
    await flushPromises()
    expect(byID('applet-import-error')?.textContent).toContain('disk full')
    expect(wrapper.emitted('close')).toBeUndefined()
    wrapper.unmount()
  })
})
