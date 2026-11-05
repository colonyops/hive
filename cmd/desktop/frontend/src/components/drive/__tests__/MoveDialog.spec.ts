import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MoveDialog from '../MoveDialog.vue'
import { doc, folder, root } from '../../../test-utils/drive'
import { settleTree, treeRow, treeRows } from '../../../test-utils/pathTree'
import type { EntryWire } from '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn().mockReturnValue(() => {}) } }))
const drive = vi.hoisted(() => ({ Spaces: vi.fn(), Tree: vi.fn(), Search: vi.fn(), Move: vi.fn() }))
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/driveservice', () => drive)
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/windowservice', () => ({
  Focused: vi.fn().mockResolvedValue(true),
}))

const plan = doc('plans/plan.md')
const tree = root([
  folder('archive', [folder('archive/2025', [])]),
  folder('plans', [folder('plans/old', []), plan]),
  doc('index.html'),
])

async function mountDialog(entry: EntryWire = plan) {
  const wrapper = mount(MoveDialog, {
    props: { space: 'acme/site', tree, entry, testid: 'mv' },
    attachTo: document.body,
  })
  await settleTree()
  return wrapper
}

const folders = () => document.querySelector('[data-testid="mv-folders"]')!
/** Each destination with whether it is chosen: the root button, then the folder rows. */
const options = () => [
  ['mv-root', document.querySelector('[data-testid="mv-root"]')?.getAttribute('aria-pressed')],
  ...treeRows(folders()).map((row) => [row.dataset.itemPath, row.getAttribute('aria-selected')]),
]

describe('MoveDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    document.body.innerHTML = ''
    drive.Spaces.mockResolvedValue([])
    drive.Tree.mockResolvedValue(tree)
  })

  it('lists the root then every folder, with the current parent selected', async () => {
    await mountDialog()
    expect(options()).toEqual([
      ['mv-root', 'false'],
      ['archive/', 'false'],
      ['archive/2025/', 'false'],
      ['plans/', 'true'],
      ['plans/old/', 'false'],
    ])
    expect(document.querySelector<HTMLButtonElement>('[data-testid="mv-confirm"]')?.disabled).toBe(true)
  })

  it('leaves a folder out of its own destinations', async () => {
    await mountDialog(folder('plans', [folder('plans/old', []), plan]))
    expect(options().map(([id]) => id)).toEqual(['mv-root', 'archive/', 'archive/2025/'])
  })

  it('moves into the chosen folder and reports where the entry went', async () => {
    drive.Move.mockResolvedValue(doc('archive/2025/plan.md'))
    const wrapper = await mountDialog()
    treeRow(folders(), 'archive/2025/').click()
    await settleTree()
    document.querySelector<HTMLButtonElement>('[data-testid="mv-confirm"]')?.click()
    await flushPromises()

    expect(drive.Move).toHaveBeenCalledWith('acme/site', 'plans/plan.md', 'archive/2025')
    expect(wrapper.emitted('moved')).toEqual([['plans/plan.md', 'archive/2025/plan.md']])
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('shows a conflict inline and stays open', async () => {
    drive.Move.mockRejectedValue(
      Object.assign(new Error('call failed'), { cause: { kind: 'conflict', message: '"plan.md" already exists' } }),
    )
    const wrapper = await mountDialog()
    document.querySelector<HTMLButtonElement>('[data-testid="mv-root"]')?.click()
    await flushPromises()
    document.querySelector<HTMLButtonElement>('[data-testid="mv-confirm"]')?.click()
    await flushPromises()

    expect(document.querySelector('[data-testid="mv-error"]')?.textContent).toContain('already exists')
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.emitted('moved')).toBeUndefined()
  })
})
