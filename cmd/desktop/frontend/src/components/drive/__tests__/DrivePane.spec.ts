import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DrivePane from '../DrivePane.vue'
import type { EntryWire } from '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { doc, documentFor, folder, root, space } from '../../../test-utils/drive'
import { useDriveActivity } from '../../../stores/useDriveActivity'

const wailsEvents = vi.hoisted(() => ({
  handlers: [] as Array<[string, (event: { data: unknown }) => void]>,
  fire(name: string, data: unknown) {
    for (const [registered, handler] of this.handlers) {
      if (registered === name) handler({ data })
    }
  },
}))
vi.mock('../../../composables/useWailsEvent', () => ({
  useWailsEvent: (name: string, handler: (event: { data: unknown }) => void) => {
    wailsEvents.handlers.push([name, handler])
  },
}))

const runtime = vi.hoisted(() => ({
  setText: vi.fn().mockResolvedValue(undefined),
  saveFile: vi.fn().mockResolvedValue('/tmp/plan.md'),
}))
vi.mock('@wailsio/runtime', () => ({
  Clipboard: { SetText: runtime.setText },
  Dialogs: { SaveFile: runtime.saveFile },
}))

const drive = vi.hoisted(() => ({
  Spaces: vi.fn(),
  Tree: vi.fn(),
  Search: vi.fn(),
  Document: vi.fn(),
  Markdown: vi.fn(),
  Export: vi.fn(),
  Delete: vi.fn(),
  Rename: vi.fn(),
}))
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/driveservice', () => drive)
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/windowservice', () => ({
  Focused: vi.fn().mockResolvedValue(true),
}))
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: vi.fn().mockResolvedValue({ canvasFontSize: '', canvasLineSpacing: '' }),
}))

const plan = doc('plans/plan.md', { title: 'The Plan', modified: '2026-10-03T00:00:00Z' })
const perf = doc('reports/perf.canvas.json', { title: 'Perf', modified: '2026-10-05T00:00:00Z' })
let tree: EntryWire

async function mountPane(
  props: Partial<{ session: string; space: string; path: string | null; agentSetup: boolean }> = {},
) {
  const wrapper = mount(DrivePane, {
    props: { session: '7', space: 'workspaces/web-app', path: null, ...props },
    attachTo: document.body,
  })
  await flushPromises()
  return wrapper
}

describe('DrivePane', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    wailsEvents.handlers = []
    localStorage.clear()
    document.body.innerHTML = ''
    tree = root([folder('plans', [plan]), folder('reports', [perf])])
    drive.Spaces.mockResolvedValue([space('workspaces/web-app', 'web-app')])
    // A fresh object per read, as the binding answers.
    drive.Tree.mockImplementation(() => Promise.resolve({ ...tree }))
    drive.Document.mockImplementation((key: string, path: string) =>
      Promise.resolve(
        path === plan.path
          ? documentFor(key, plan, { markdown: '# The Plan\n\nSee [perf](../reports/perf.canvas.json).' })
          : documentFor(key, perf, { canvas: { title: 'Perf', blocks: [] } }),
      ),
    )
    drive.Markdown.mockResolvedValue('# The Plan\n')
    drive.Export.mockResolvedValue(undefined)
    drive.Delete.mockResolvedValue(undefined)
  })

  it('opens on the newest document in the space and titles the header with it', async () => {
    const wrapper = await mountPane()
    expect(drive.Tree).toHaveBeenCalledWith('workspaces/web-app')
    expect(drive.Document).toHaveBeenLastCalledWith('workspaces/web-app', 'reports/perf.canvas.json')
    expect(wrapper.get('[data-testid="drive-pane-title"]').text()).toContain('Perf')
  })

  // The tree carries no authors, so the default learns the session's writes
  // from drive:updated.
  it("prefers the open chat's newest write over a newer document another wrote", async () => {
    await mountPane()
    wailsEvents.fire('drive:updated', {
      space: 'workspaces/web-app',
      paths: [plan.path],
      session: '7',
      hiveSession: '',
    })
    await flushPromises()
    expect(drive.Document).toHaveBeenLastCalledWith('workspaces/web-app', 'plans/plan.md')
  })

  it("counts the session's writes as seen only while the pane shows that session", async () => {
    const activity = useDriveActivity()
    const write = (session: string) =>
      wailsEvents.fire('drive:updated', { space: 'workspaces/web-app', paths: [plan.path], session, hiveSession: '' })
    write('7')
    write('9')
    const wrapper = await mountPane()
    expect(activity.isUnseen('7')).toBe(false)

    write('7')
    expect(activity.isUnseen('7')).toBe(false)

    await wrapper.setProps({ session: '9' })
    expect(activity.isUnseen('9')).toBe(false)
    write('7')
    expect(activity.isUnseen('7')).toBe(true)

    wrapper.unmount()
    write('9')
    expect(activity.isUnseen('9')).toBe(true)
  })

  it('names its resize handle after the pane', async () => {
    const wrapper = await mountPane()
    expect(wrapper.find('[data-testid="resize-handle-drive-pane"]').exists()).toBe(true)
  })

  it('pins the route-named document and marks it in the tree', async () => {
    const wrapper = await mountPane({ path: plan.path })
    expect(drive.Document).toHaveBeenLastCalledWith('workspaces/web-app', 'plans/plan.md')

    await wrapper.get('[data-testid="drive-pane-title"]').trigger('click')
    expect(wrapper.get('[data-testid="drive-pane-row-plans/plan.md"]').attributes('aria-selected')).toBe('true')
  })

  it('emits pick from the tree instead of switching locally, and from a link', async () => {
    const wrapper = await mountPane({ path: plan.path })
    await wrapper.get('[data-testid="drive-pane-markdown"] a').trigger('click')
    expect(wrapper.emitted('pick')).toEqual([['reports/perf.canvas.json']])

    await wrapper.get('[data-testid="drive-pane-title"]').trigger('click')
    await wrapper.get('[data-testid="drive-pane-row-plans/plan.md"]').trigger('click')
    expect(wrapper.emitted('pick')?.at(-1)).toEqual(['plans/plan.md'])
    expect(wrapper.find('[data-testid="drive-pane-browse"]').exists()).toBe(false)
  })

  it('filters the tree from the search input', async () => {
    const wrapper = await mountPane()
    await wrapper.get('[data-testid="drive-pane-title"]').trigger('click')
    await wrapper.get('[data-testid="drive-pane-search"]').setValue('perf')
    expect(wrapper.find('[data-testid="drive-pane-row-plans/plan.md"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="drive-pane-row-reports/perf.canvas.json"]').exists()).toBe(true)

    await wrapper.get('[data-testid="drive-pane-search"]').setValue('nothing-matches')
    expect(wrapper.get('[data-testid="drive-pane-browse"]').text()).toContain('No documents match.')
  })

  it('falls back to the default when the pinned document disappears', async () => {
    await mountPane({ path: plan.path })
    tree = root([folder('reports', [perf])])
    wailsEvents.fire('drive:updated', { space: 'workspaces/web-app', paths: [plan.path], session: '', hiveSession: '' })
    await flushPromises()
    expect(drive.Document).toHaveBeenLastCalledWith('workspaces/web-app', 'reports/perf.canvas.json')
  })

  // open_document can name a document before the agent's first write to it.
  it('keeps a pinned path that was never listed, blank', async () => {
    const wrapper = await mountPane({ path: 'plans/draft.md' })
    expect(drive.Document).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="drive-pane-empty"]').exists()).toBe(true)
  })

  it('re-reads the document on drive:updated only when its own entry changed', async () => {
    await mountPane()
    const reads = drive.Document.mock.calls.length
    wailsEvents.fire('drive:updated', { space: 'acme/site', paths: ['x.md'], session: '', hiveSession: '' })
    await flushPromises()
    expect(drive.Document.mock.calls.length).toBe(reads)

    // The other document changed (and stays older): the tree is re-read, the
    // shown one is kept.
    tree = root([
      folder('plans', [doc(plan.path, { ...plan, modified: '2026-10-04T00:00:00Z' })]),
      folder('reports', [perf]),
    ])
    wailsEvents.fire('drive:updated', { space: 'workspaces/web-app', paths: [plan.path], session: '', hiveSession: '' })
    await flushPromises()
    expect(drive.Document.mock.calls.length).toBe(reads)

    tree = root([folder('plans', [plan]), folder('reports', [doc(perf.path, { ...perf, size: 11 })])])
    wailsEvents.fire('drive:updated', { space: 'workspaces/web-app', paths: [perf.path], session: '', hiveSession: '' })
    await flushPromises()
    expect(drive.Document.mock.calls.length).toBe(reads + 1)
  })

  it('asks for the full-page view on the document it is showing, and closes', async () => {
    const wrapper = await mountPane({ path: plan.path })
    await wrapper.get('[data-testid="drive-pane-expand"]').trigger('click')
    await wrapper.get('[data-testid="drive-pane-close"]').trigger('click')
    expect(wrapper.emitted('open-page')).toEqual([['plans/plan.md']])
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  // Hive wires a workspace's agent itself. A Code session's agent is the
  // user's to configure, so its empty pane says how.
  it('offers agent setup on an empty space only where it is asked to', async () => {
    tree = root([])
    const inChats = await mountPane()
    expect(inChats.find('[data-testid="drive-pane-setup"]').exists()).toBe(false)
    expect(inChats.find('[data-testid="drive-pane-empty"]').exists()).toBe(true)

    const inCode = await mountPane({ session: 'abc123', space: 'acme/site', agentSetup: true })
    await inCode.get('[data-testid="drive-pane-setup-open"]').trigger('click')
    expect(inCode.emitted('setup')).toHaveLength(1)
  })

  it('copies and saves the markdown form through the native runtime', async () => {
    const wrapper = await mountPane({ path: plan.path })
    await wrapper.get('[data-testid="drive-pane-copy"]').trigger('click')
    await flushPromises()
    expect(drive.Markdown).toHaveBeenCalledWith('workspaces/web-app', 'plans/plan.md')
    expect(runtime.setText).toHaveBeenCalledWith('# The Plan\n')

    await wrapper.get('[data-testid="drive-pane-download"]').trigger('click')
    await flushPromises()
    expect(runtime.saveFile).toHaveBeenCalledWith(expect.objectContaining({ Filename: 'plan.md' }))
    expect(drive.Export).toHaveBeenCalledWith('workspaces/web-app', 'plans/plan.md', '/tmp/plan.md')

    runtime.saveFile.mockResolvedValueOnce('')
    await wrapper.get('[data-testid="drive-pane-download"]').trigger('click')
    await flushPromises()
    expect(drive.Export).toHaveBeenCalledTimes(1)
  })

  it('deletes after confirmation and picks the next document', async () => {
    const wrapper = await mountPane({ path: plan.path })
    await wrapper.get('[data-testid="drive-pane-more"]').trigger('click')
    document.querySelector<HTMLButtonElement>('[data-testid="drive-pane-menu-delete"]')?.click()
    await flushPromises()
    expect(drive.Delete).not.toHaveBeenCalled()
    document.querySelector<HTMLButtonElement>('[data-testid="drive-pane-delete-confirmation-confirm"]')?.click()
    await flushPromises()

    expect(drive.Delete).toHaveBeenCalledWith('workspaces/web-app', 'plans/plan.md')
    expect(wrapper.emitted('pick')).toEqual([['reports/perf.canvas.json']])
  })

  it('follows the shown document when the menu renames it', async () => {
    drive.Rename.mockResolvedValue(doc('plans/roadmap.md'))
    const wrapper = await mountPane({ path: plan.path })
    await wrapper.get('[data-testid="drive-pane-more"]').trigger('click')
    document.querySelector<HTMLButtonElement>('[data-testid="drive-pane-menu-rename"]')?.click()
    await flushPromises()
    const input = document.querySelector<HTMLInputElement>('[data-testid="drive-pane-rename-input"]')
    expect(input?.value).toBe('plan.md')
    input!.value = 'roadmap'
    input!.dispatchEvent(new Event('input'))
    document.querySelector<HTMLButtonElement>('[data-testid="drive-pane-rename-save"]')?.click()
    await flushPromises()

    expect(drive.Rename).toHaveBeenCalledWith('workspaces/web-app', 'plans/plan.md', 'roadmap')
    expect(wrapper.emitted('pick')).toEqual([['plans/roadmap.md']])
  })

  it('offers no markdown copy for an html document', async () => {
    const page = doc('page.html')
    tree = root([page])
    drive.Document.mockResolvedValue(documentFor('workspaces/web-app', page, { html: '<p>hi</p>' }))
    const wrapper = await mountPane()
    expect(wrapper.find('[data-testid="drive-pane-frame"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="drive-pane-copy"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="drive-pane-more"]').exists()).toBe(true)
  })

  it('marks an html document sandboxed in the header and toggles it between filling the reader and inline', async () => {
    const page = doc('page.html')
    tree = root([page])
    drive.Document.mockResolvedValue(documentFor('workspaces/web-app', page, { html: '<p>hi</p>' }))
    const wrapper = await mountPane()
    expect(wrapper.find('[data-testid="drive-pane-sandbox"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="drive-pane-frame"]').classes()).toContain('absolute')

    await wrapper.get('[data-testid="drive-pane-frame-expand"]').trigger('click')

    expect(wrapper.get('[data-testid="drive-pane-frame"]').classes()).not.toContain('absolute')
  })

  it('shows no sandbox controls for a markdown document', async () => {
    const wrapper = await mountPane({ path: plan.path })
    expect(wrapper.find('[data-testid="drive-pane-sandbox"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="drive-pane-frame-expand"]').exists()).toBe(false)
  })
})
