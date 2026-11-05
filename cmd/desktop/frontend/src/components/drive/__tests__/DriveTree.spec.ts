import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DriveTree from '../DriveTree.vue'
import { doc, folder, root } from '../../../test-utils/drive'
import { settleTree, treeKey, treePaths, treeRow, treeShadow } from '../../../test-utils/pathTree'

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
  folder('empty', []),
  doc('index.html'),
  doc('notes.txt'),
])

async function mountTree(props: Partial<{ selected: string | null; filter: string; tree: typeof tree }> = {}) {
  const wrapper = mount(DriveTree, {
    props: { space: 'acme/site', tree, selected: null, filter: '', testid: 't', ...props },
    attachTo: document.body,
  })
  await settleTree()
  return wrapper
}

const scopeOf = (wrapper: Awaited<ReturnType<typeof mountTree>>) => wrapper.element as Element
const rows = (wrapper: Awaited<ReturnType<typeof mountTree>>) => treePaths(scopeOf(wrapper))
const row = (wrapper: Awaited<ReturnType<typeof mountTree>>, path: string) => treeRow(scopeOf(wrapper), path)

describe('DriveTree', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    document.body.innerHTML = ''
    events.On.mockReturnValue(() => {})
    drive.Spaces.mockResolvedValue([])
    drive.Tree.mockResolvedValue(tree)
  })

  it('lists folders first, folded, and keeps an empty folder', async () => {
    const wrapper = await mountTree()
    expect(rows(wrapper)).toEqual(['canvases/', 'empty/', 'plans/', 'index.html', 'notes.txt'])
    expect(row(wrapper, 'plans/').getAttribute('aria-expanded')).toBe('false')
  })

  it('opens a folder on click and emits pick for a document', async () => {
    const wrapper = await mountTree()
    row(wrapper, 'plans/').click()
    await settleTree()
    expect(rows(wrapper)).toContain('plans/2026-10-11-plan.md')

    row(wrapper, 'plans/2026-10-11-plan.md').click()
    await settleTree()
    expect(wrapper.emitted('pick')).toEqual([['plans/2026-10-11-plan.md']])
  })

  it('keeps the shown document selected when a folder is clicked', async () => {
    const wrapper = await mountTree({ selected: 'index.html' })
    row(wrapper, 'plans/').click()
    await settleTree()
    expect(row(wrapper, 'index.html').getAttribute('aria-selected')).toBe('true')
    expect(row(wrapper, 'plans/').getAttribute('aria-selected')).toBe('false')
    expect(wrapper.emitted('pick')).toBeUndefined()
  })

  it('keeps only the matches and their folders while filtering', async () => {
    const wrapper = await mountTree({ filter: 'a.md' })
    expect(rows(wrapper)).toEqual(['plans/', 'plans/old/', 'plans/old/a.md'])

    await wrapper.setProps({ filter: '' })
    await settleTree()
    expect(rows(wrapper)).toEqual(['canvases/', 'empty/', 'plans/', 'index.html', 'notes.txt'])
  })

  it('walks the rows from a search field and picks with Enter', async () => {
    const wrapper = await mountTree()
    const exposed = wrapper.vm as unknown as { onKeydown: (event: KeyboardEvent) => boolean }
    const key = (name: string) => exposed.onKeydown(new KeyboardEvent('keydown', { key: name, cancelable: true }))
    expect(key('End')).toBe(true)
    expect(key('Enter')).toBe(true)
    expect(wrapper.emitted('pick')).toEqual([['notes.txt']])
    expect(key('x')).toBe(false)
  })

  it('reveals the selected document by opening its folders', async () => {
    const wrapper = await mountTree({ selected: 'plans/old/a.md' })
    expect(row(wrapper, 'plans/').getAttribute('aria-expanded')).toBe('true')
    expect(row(wrapper, 'plans/old/').getAttribute('aria-expanded')).toBe('true')
    expect(row(wrapper, 'plans/old/a.md').getAttribute('aria-selected')).toBe('true')
  })

  it('keeps a folder the user folded closed when the tree is replaced', async () => {
    const wrapper = await mountTree({ selected: 'plans/old/a.md' })
    row(wrapper, 'plans/').click()
    await settleTree()
    expect(row(wrapper, 'plans/').getAttribute('aria-expanded')).toBe('false')

    await wrapper.setProps({ tree: structuredClone(tree) })
    await settleTree()
    expect(row(wrapper, 'plans/').getAttribute('aria-expanded')).toBe('false')
  })

  it('keeps folders open across a remount', async () => {
    const first = await mountTree()
    row(first, 'plans/').click()
    await settleTree()
    first.unmount()

    const second = await mountTree()
    expect(row(second, 'plans/').getAttribute('aria-expanded')).toBe('true')
  })

  it('reveals a selected document once it first appears in the tree', async () => {
    const wrapper = await mountTree({ selected: 'plans/old/b.md' })
    expect(row(wrapper, 'plans/').getAttribute('aria-expanded')).toBe('false')

    const grown = root([folder('plans', [folder('plans/old', [doc('plans/old/a.md'), doc('plans/old/b.md')])])])
    await wrapper.setProps({ tree: grown })
    await settleTree()
    expect(row(wrapper, 'plans/old/b.md').getAttribute('aria-selected')).toBe('true')
  })

  it('emits menu with the row as its anchor on a right click', async () => {
    const wrapper = await mountTree()
    row(wrapper, 'index.html').dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, composed: true, cancelable: true }),
    )
    await settleTree()
    const [entry, anchor] = wrapper.emitted('menu')?.[0] ?? []
    expect((entry as { path: string }).path).toBe('index.html')
    expect((anchor as HTMLElement).dataset.itemPath).toBe('index.html')
  })

  it('emits menu for the space root on a right click below the rows', async () => {
    const wrapper = await mountTree()
    await wrapper.get('[data-testid="t-tree"]').trigger('contextmenu')
    const [entry, , toggle] = wrapper.emitted('menu')?.[0] ?? []
    expect((entry as { path: string }).path).toBe('')
    expect(toggle).toBeNull()
  })

  it('marks a document written since it was last opened', async () => {
    const { useDriveActivity } = await import('../../../stores/useDriveActivity')
    vi.spyOn(useDriveActivity(), 'isUnopened').mockImplementation((_space, path) => path === 'notes.txt')
    const wrapper = await mountTree()
    const decoration = (path: string) =>
      row(wrapper, path).querySelector('[data-item-section="decoration"]')?.textContent
    expect(decoration('notes.txt')).toBe('●')
    expect(decoration('index.html')).toBe('')
  })

  it('renames through the drive and reports the move', async () => {
    drive.Rename.mockResolvedValue(doc('renamed.html'))
    const wrapper = await mountTree()
    ;(wrapper.vm as unknown as { startRename: (path: string) => void }).startRename('index.html')
    await settleTree()

    const input = treeShadow(scopeOf(wrapper)).querySelector<HTMLInputElement>('[data-item-rename-input]')
    expect(input?.value).toBe('index.html')
    input!.value = 'renamed.html'
    input!.dispatchEvent(new Event('input', { bubbles: true, composed: true }))
    treeKey(input!, 'Enter')
    await settleTree()

    expect(drive.Rename).toHaveBeenCalledWith('acme/site', 'index.html', 'renamed.html')
    expect(wrapper.emitted('moved')).toEqual([['index.html', 'renamed.html']])
  })

  it('puts the old name back and shows the error when the drive refuses a rename', async () => {
    drive.Rename.mockRejectedValue(new Error('"notes.txt" already exists in acme/site'))
    const wrapper = await mountTree()
    ;(wrapper.vm as unknown as { startRename: (path: string) => void }).startRename('index.html')
    await settleTree()
    const input = treeShadow(scopeOf(wrapper)).querySelector<HTMLInputElement>('[data-item-rename-input]')!
    input.value = 'other.html'
    input.dispatchEvent(new Event('input', { bubbles: true, composed: true }))
    treeKey(input, 'Enter')
    await settleTree()

    expect(wrapper.get('[data-testid="t-rename-error"]').text()).toContain('already exists')
    expect(rows(wrapper)).toContain('index.html')
    expect(rows(wrapper)).not.toContain('other.html')
    expect(wrapper.emitted('moved')).toBeUndefined()
  })
})
