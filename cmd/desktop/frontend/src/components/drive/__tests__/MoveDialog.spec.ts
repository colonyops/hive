import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MoveDialog from '../MoveDialog.vue'
import { doc, folder, root } from '../../../test-utils/drive'
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

function mountDialog(entry: EntryWire = plan) {
  return mount(MoveDialog, {
    props: { space: 'acme/site', tree, entry, testid: 'mv' },
    attachTo: document.body,
  })
}

const options = () =>
  [...document.querySelectorAll('[role="option"]')].map((option) => [
    option.getAttribute('data-testid'),
    option.getAttribute('aria-selected'),
  ])

describe('MoveDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    document.body.innerHTML = ''
    drive.Spaces.mockResolvedValue([])
    drive.Tree.mockResolvedValue(tree)
  })

  it('lists the root then every folder, with the current parent selected', () => {
    mountDialog()
    expect(options()).toEqual([
      ['mv-root', 'false'],
      ['mv-folder-archive', 'false'],
      ['mv-folder-archive/2025', 'false'],
      ['mv-folder-plans', 'true'],
      ['mv-folder-plans/old', 'false'],
    ])
    expect(document.querySelector<HTMLButtonElement>('[data-testid="mv-confirm"]')?.disabled).toBe(true)
  })

  it('leaves a folder out of its own destinations', () => {
    mountDialog(folder('plans', [folder('plans/old', []), plan]))
    expect(options().map(([id]) => id)).toEqual(['mv-root', 'mv-folder-archive', 'mv-folder-archive/2025'])
  })

  it('moves into the chosen folder and reports where the entry went', async () => {
    drive.Move.mockResolvedValue(doc('archive/2025/plan.md'))
    const wrapper = mountDialog()
    document.querySelector<HTMLButtonElement>('[data-testid="mv-folder-archive/2025"]')?.click()
    await flushPromises()
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
    const wrapper = mountDialog()
    document.querySelector<HTMLButtonElement>('[data-testid="mv-root"]')?.click()
    await flushPromises()
    document.querySelector<HTMLButtonElement>('[data-testid="mv-confirm"]')?.click()
    await flushPromises()

    expect(document.querySelector('[data-testid="mv-error"]')?.textContent).toContain('already exists')
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.emitted('moved')).toBeUndefined()
  })
})
