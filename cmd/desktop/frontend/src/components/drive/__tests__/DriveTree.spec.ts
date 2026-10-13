import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DriveTree from '../DriveTree.vue'
import { doc, folder, root } from '../../../test-utils/drive'

const events = vi.hoisted(() => ({ On: vi.fn() }))
vi.mock('@wailsio/runtime', () => ({ Events: { On: events.On } }))

const drive = vi.hoisted(() => ({
  Spaces: vi.fn(),
  Tree: vi.fn(),
  Search: vi.fn(),
  Rename: vi.fn(),
  Move: vi.fn(),
}))
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/driveservice', () => drive)
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/windowservice', () => ({
  Focused: vi.fn().mockResolvedValue(true),
}))

const tree = root([
  folder('canvases', [doc('canvases/report.canvas.json', { title: 'E2E Report' })]),
  folder('plans', [
    folder('plans/old', [doc('plans/old/a.md')]),
    doc('plans/2026-10-11-plan.md', { title: 'The Plan' }),
  ]),
  doc('index.html'),
  doc('notes.txt'),
])

function mountTree(props: Partial<{ selected: string | null; filter: string; tree: typeof tree }> = {}) {
  return mount(DriveTree, {
    props: { space: 'acme/site', tree, selected: null, filter: '', testid: 't', ...props },
    attachTo: document.body,
  })
}

const rowPaths = (wrapper: ReturnType<typeof mountTree>) =>
  wrapper.findAll('[role="treeitem"]').map((row) => row.attributes('data-testid')?.replace('t-row-', ''))

describe('DriveTree', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    events.On.mockReturnValue(() => {})
    drive.Spaces.mockResolvedValue([])
    drive.Tree.mockResolvedValue(tree)
  })

  it('lists folders first as served, folded, with a kind per row', () => {
    const wrapper = mountTree()
    expect(rowPaths(wrapper)).toEqual(['canvases', 'plans', 'index.html', 'notes.txt'])
    expect(wrapper.get('[data-testid="t-row-plans"]').attributes('aria-expanded')).toBe('false')
    expect(wrapper.get('[data-testid="t-row-index.html"]').attributes('data-kind')).toBe('html')
    expect(wrapper.get('[data-testid="t-row-notes.txt"]').attributes('data-kind')).toBe('file')
  })

  it('opens and closes a folder on click, and emits pick for a document', async () => {
    const wrapper = mountTree()
    await wrapper.get('[data-testid="t-row-plans"]').trigger('click')
    expect(rowPaths(wrapper)).toEqual([
      'canvases',
      'plans',
      'plans/old',
      'plans/2026-10-11-plan.md',
      'index.html',
      'notes.txt',
    ])
    expect(wrapper.get('[data-testid="t-row-plans/2026-10-11-plan.md"]').text()).toBe('The Plan')

    await wrapper.get('[data-testid="t-row-plans/2026-10-11-plan.md"]').trigger('click')
    expect(wrapper.emitted('pick')).toEqual([['plans/2026-10-11-plan.md']])

    await wrapper.get('[data-testid="t-row-plans"]').trigger('click')
    expect(rowPaths(wrapper)).toEqual(['canvases', 'plans', 'index.html', 'notes.txt'])
  })

  it('opens every folder while filtering and keeps only the matches and their folders', async () => {
    const wrapper = mountTree({ filter: 'a.md' })
    expect(rowPaths(wrapper)).toEqual(['plans', 'plans/old', 'plans/old/a.md'])

    await wrapper.setProps({ filter: 'report' })
    expect(rowPaths(wrapper)).toEqual(['canvases', 'canvases/report.canvas.json'])
  })

  it('walks the rows with the keyboard, opening a folder and picking a document', async () => {
    const wrapper = mountTree()
    const host = wrapper.get('[data-testid="t-tree"]')
    await host.trigger('keydown', { key: 'ArrowDown' })
    await host.trigger('keydown', { key: 'ArrowRight' })
    expect(wrapper.get('[data-testid="t-row-plans"]').attributes('aria-expanded')).toBe('true')

    await host.trigger('keydown', { key: 'ArrowDown' })
    await host.trigger('keydown', { key: 'ArrowDown' })
    await host.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('pick')).toEqual([['plans/2026-10-11-plan.md']])

    await host.trigger('keydown', { key: 'ArrowLeft' })
    await host.trigger('keydown', { key: 'ArrowLeft' })
    expect(wrapper.get('[data-testid="t-row-plans"]').attributes('aria-expanded')).toBe('false')
  })

  it('reveals the selected document by opening its folders and scrolling to it', async () => {
    const scrolled: string[] = []
    const original = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = function (this: Element) {
      scrolled.push(this.getAttribute('data-testid') ?? '')
    }
    try {
      const wrapper = mountTree({ selected: 'plans/old/a.md' })
      await flushPromises()
      expect(wrapper.get('[data-testid="t-row-plans/old/a.md"]').attributes('aria-selected')).toBe('true')
      expect(wrapper.get('[data-testid="t-row-plans/old"]').attributes('aria-expanded')).toBe('true')
      expect(scrolled).toContain('t-row-plans/old/a.md')
    } finally {
      Element.prototype.scrollIntoView = original
    }
  })

  it('emits menu with the row as its anchor on a right click', async () => {
    const wrapper = mountTree()
    await wrapper.get('[data-testid="t-row-index.html"]').trigger('contextmenu')
    const [entry, anchor, toggle] = wrapper.emitted('menu')?.[0] ?? []
    expect((entry as { path: string }).path).toBe('index.html')
    expect((anchor as HTMLElement).getAttribute('data-testid')).toBe('t-row-index.html')
    expect(toggle).toBeNull()
  })

  it('emits menu from the row button with the row as anchor and the button as toggle', async () => {
    const wrapper = mountTree()
    await wrapper.get('[data-testid="t-row-plans-menu"]').trigger('click')
    const [entry, anchor, toggle] = wrapper.emitted('menu')?.[0] ?? []
    expect((entry as { path: string }).path).toBe('plans')
    expect((anchor as HTMLElement).getAttribute('data-testid')).toBe('t-row-plans')
    expect((toggle as HTMLElement).getAttribute('data-testid')).toBe('t-row-plans-menu')
    expect(wrapper.emitted('pick')).toBeUndefined()
    expect(wrapper.get('[data-testid="t-row-plans"]').attributes('aria-expanded')).toBe('false')
  })

  it("anchors a later row's menu on that row after a folder opens above it", async () => {
    const wrapper = mountTree()
    await wrapper.get('[data-testid="t-row-plans"]').trigger('click')
    await wrapper.get('[data-testid="t-row-index.html-menu"]').trigger('click')
    const [entry, anchor] = wrapper.emitted('menu')?.[0] ?? []
    expect((entry as { path: string }).path).toBe('index.html')
    expect((anchor as HTMLElement).getAttribute('data-testid')).toBe('t-row-index.html')
  })

  it('keeps a folder the user folded closed when the tree is replaced', async () => {
    const wrapper = mountTree({ selected: 'plans/old/a.md' })
    await flushPromises()
    expect(wrapper.get('[data-testid="t-row-plans"]').attributes('aria-expanded')).toBe('true')

    await wrapper.get('[data-testid="t-row-plans"]').trigger('click')
    expect(wrapper.get('[data-testid="t-row-plans"]').attributes('aria-expanded')).toBe('false')

    await wrapper.setProps({ tree: structuredClone(tree) })
    await flushPromises()
    expect(wrapper.get('[data-testid="t-row-plans"]').attributes('aria-expanded')).toBe('false')
  })

  it('reveals a selected document once it first appears in the tree', async () => {
    const wrapper = mountTree({ selected: 'plans/old/b.md' })
    await flushPromises()
    expect(wrapper.get('[data-testid="t-row-plans"]').attributes('aria-expanded')).toBe('false')

    const grown = root([folder('plans', [folder('plans/old', [doc('plans/old/a.md'), doc('plans/old/b.md')])])])
    await wrapper.setProps({ tree: grown })
    await flushPromises()
    expect(wrapper.get('[data-testid="t-row-plans/old/b.md"]').attributes('aria-selected')).toBe('true')
  })

  it('emits menu for the space root on a right click below the rows', async () => {
    const wrapper = mountTree()
    await wrapper.get('[data-testid="t-tree"]').trigger('contextmenu')
    const [entry] = wrapper.emitted('menu')?.[0] ?? []
    expect((entry as { path: string }).path).toBe('')
  })

  it('renames in place on F2 and commits on Enter', async () => {
    drive.Rename.mockResolvedValue(doc('renamed.html'))
    const wrapper = mountTree()
    const host = wrapper.get('[data-testid="t-tree"]')
    for (let i = 0; i < 2; i++) await host.trigger('keydown', { key: 'ArrowDown' })
    await host.trigger('keydown', { key: 'F2' })
    await flushPromises()

    const input = wrapper.get<HTMLInputElement>('[data-testid="t-rename-input"]')
    expect(input.element.value).toBe('index.html')
    expect(document.activeElement).toBe(input.element)
    expect([input.element.selectionStart, input.element.selectionEnd]).toEqual([0, 'index'.length])

    await input.setValue('renamed.html')
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(drive.Rename).toHaveBeenCalledWith('acme/site', 'index.html', 'renamed.html')
    expect(wrapper.emitted('moved')).toEqual([['index.html', 'renamed.html']])
    expect(wrapper.find('[data-testid="t-rename-input"]').exists()).toBe(false)
  })

  it('cancels a rename on Escape and commits one on blur', async () => {
    drive.Rename.mockResolvedValue(folder('canvases-old', []))
    const wrapper = mountTree()
    const exposed = wrapper.vm as unknown as { startRename: (path: string) => void }

    exposed.startRename('canvases')
    await flushPromises()
    await wrapper.get('[data-testid="t-rename-input"]').setValue('ignored')
    await wrapper.get('[data-testid="t-rename-input"]').trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('[data-testid="t-rename-input"]').exists()).toBe(false)
    expect(drive.Rename).not.toHaveBeenCalled()

    exposed.startRename('canvases')
    await flushPromises()
    await wrapper.get('[data-testid="t-rename-input"]').setValue('canvases-old')
    await wrapper.get('[data-testid="t-rename-input"]').trigger('blur')
    await flushPromises()
    expect(drive.Rename).toHaveBeenCalledWith('acme/site', 'canvases', 'canvases-old')
  })

  it('keeps the field open with the error when the drive refuses a rename', async () => {
    drive.Rename.mockRejectedValue(new Error('"notes.txt" already exists in acme/site'))
    const wrapper = mountTree()
    ;(wrapper.vm as unknown as { startRename: (path: string) => void }).startRename('index.html')
    await flushPromises()
    await wrapper.get('[data-testid="t-rename-input"]').setValue('notes.txt')
    await wrapper.get('[data-testid="t-rename-input"]').trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(wrapper.get('[data-testid="t-rename-error"]').text()).toContain('already exists')
    expect(wrapper.find('[data-testid="t-rename-input"]').exists()).toBe(true)
    expect(wrapper.emitted('moved')).toBeUndefined()
  })

  it('moves a dragged document onto a folder, and onto the root below the rows', async () => {
    drive.Move.mockResolvedValue(doc('plans/index.html'))
    const wrapper = mountTree()
    const row = wrapper.get('[data-testid="t-row-index.html"]')
    expect(row.attributes('draggable')).toBe('true')
    expect(wrapper.get('[data-testid="t-row-plans"]').attributes('draggable')).toBe('false')

    await row.trigger('dragstart')
    await wrapper.get('[data-testid="t-row-plans"]').trigger('dragover')
    expect(wrapper.get('[data-testid="t-row-plans"]').attributes('data-drop')).toBe('true')
    await wrapper.get('[data-testid="t-row-plans"]').trigger('drop')
    await flushPromises()
    expect(drive.Move).toHaveBeenCalledWith('acme/site', 'index.html', 'plans')
    expect(wrapper.emitted('moved')).toEqual([['index.html', 'plans/index.html']])
    expect(wrapper.find('[data-drop]').exists()).toBe(false)

    await wrapper.get('[data-testid="t-row-plans"]').trigger('click')
    drive.Move.mockResolvedValue(doc('plans/2026-10-11-plan.md'))
    await wrapper.get('[data-testid="t-row-plans/2026-10-11-plan.md"]').trigger('dragstart')
    await wrapper.get('[data-testid="t-tree"]').trigger('dragover')
    expect(wrapper.get('[data-testid="t-tree"]').attributes('data-drop')).toBe('true')
    await wrapper.get('[data-testid="t-tree"]').trigger('drop')
    await flushPromises()
    expect(drive.Move).toHaveBeenLastCalledWith('acme/site', 'plans/2026-10-11-plan.md', '')
  })

  it('does not move a document dropped into the folder it is already in', async () => {
    const wrapper = mountTree()
    await wrapper.get('[data-testid="t-row-index.html"]').trigger('dragstart')
    await wrapper.get('[data-testid="t-row-notes.txt"]').trigger('dragover')
    expect(wrapper.get('[data-testid="t-tree"]').attributes('data-drop')).toBe('true')
    await wrapper.get('[data-testid="t-row-notes.txt"]').trigger('drop')
    await flushPromises()
    expect(drive.Move).not.toHaveBeenCalled()
  })
})
