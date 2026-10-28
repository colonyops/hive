import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useDrive } from '../useDrive'
import { doc, folder, root, space } from '../../test-utils/drive'

const mocks = vi.hoisted(() => ({
  Spaces: vi.fn(),
  Tree: vi.fn(),
  Search: vi.fn(),
  Recent: vi.fn(),
  CreateFolder: vi.fn(),
  Rename: vi.fn(),
  Move: vi.fn(),
  Delete: vi.fn(),
  Focused: vi.fn(),
  On: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/driveservice', () => ({
  Spaces: mocks.Spaces,
  Tree: mocks.Tree,
  Search: mocks.Search,
  Recent: mocks.Recent,
  CreateFolder: mocks.CreateFolder,
  Rename: mocks.Rename,
  Move: mocks.Move,
  Delete: mocks.Delete,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/windowservice', () => ({
  Focused: mocks.Focused,
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

function fire(name: string, data: unknown): void {
  for (const [registered, handler] of mocks.On.mock.calls as Array<[string, (event: { data: unknown }) => void]>) {
    if (registered === name) handler({ data })
  }
}

describe('useDrive', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.On.mockReturnValue(() => {})
    mocks.Focused.mockResolvedValue(true)
    mocks.Spaces.mockResolvedValue([space('acme/site'), space('shared', 'Shared')])
    mocks.Tree.mockImplementation((key: string) => Promise.resolve(root([doc(`${key === 'shared' ? 's' : 'a'}.md`)])))
  })

  it('lists the spaces and loads a tree once however often it is asked for', async () => {
    const drive = useDrive()
    await Promise.all([drive.ensureTree('acme/site'), drive.ensureTree('acme/site')])
    await drive.ensureTree('acme/site')

    expect(drive.spaces.value.map((known) => known.key)).toEqual(['acme/site', 'shared'])
    expect(mocks.Tree).toHaveBeenCalledTimes(1)
    expect(drive.trees.value.get('acme/site')?.children?.[0]?.path).toBe('a.md')
  })

  it('reloads only the space a drive:updated names', async () => {
    const drive = useDrive()
    await drive.ensureTree('acme/site')
    await drive.ensureTree('shared')
    mocks.Tree.mockClear()
    mocks.Tree.mockResolvedValue(root([doc('a.md'), doc('b.md')]))

    fire('drive:updated', { space: 'acme/site', paths: ['b.md'], session: '', hiveSession: '' })
    await flushPromises()

    expect(mocks.Tree.mock.calls).toEqual([['acme/site']])
    expect(drive.trees.value.get('acme/site')?.children?.map((entry) => entry.path)).toEqual(['a.md', 'b.md'])
    expect(drive.trees.value.get('shared')?.children?.map((entry) => entry.path)).toEqual(['s.md'])
  })

  it('does not load a space nothing asked for, but learns a new space', async () => {
    useDrive()
    await flushPromises()

    fire('drive:updated', { space: 'workspaces/demo', paths: ['x.md'], session: '', hiveSession: '' })
    await flushPromises()

    expect(mocks.Tree).not.toHaveBeenCalled()
    expect(mocks.Spaces).toHaveBeenCalledTimes(2)
  })

  it('re-reads every loaded space when the window regains focus', async () => {
    const drive = useDrive()
    await drive.ensureTree('acme/site')
    mocks.Tree.mockClear()

    fire('window:blur', undefined)
    await flushPromises()
    expect(mocks.Tree).not.toHaveBeenCalled()
    fire('window:focus', undefined)
    await flushPromises()

    expect(mocks.Tree.mock.calls).toEqual([['acme/site']])
  })

  it('keeps a failed tree read as an error for that space', async () => {
    mocks.Tree.mockRejectedValue(new Error('scan failed'))
    const drive = useDrive()
    await drive.ensureTree('acme/site')

    expect(drive.treeErrors.value.get('acme/site')).toBe('scan failed')
    expect(drive.trees.value.has('acme/site')).toBe(false)
  })

  it('searches every space', async () => {
    mocks.Search.mockResolvedValue(null)
    const drive = useDrive()
    expect(await drive.search('plan')).toEqual([])
    expect(mocks.Search).toHaveBeenCalledWith('plan', 50)
  })

  it('loads the recent documents once asked for, then follows every change to any space', async () => {
    mocks.Recent.mockResolvedValue([{ space: space('shared', 'Shared'), entry: doc('s.md') }])
    const drive = useDrive()
    fire('drive:updated', { space: 'acme/site', paths: ['a.md'], session: '', hiveSession: '' })
    await flushPromises()
    expect(mocks.Recent).not.toHaveBeenCalled()

    await Promise.all([drive.ensureRecent(), drive.ensureRecent()])
    expect(mocks.Recent.mock.calls).toEqual([[8]])
    expect(drive.recent.value.map((hit) => hit.entry.path)).toEqual(['s.md'])

    fire('drive:updated', { space: 'acme/site', paths: ['b.md'], session: '', hiveSession: '' })
    await flushPromises()
    expect(mocks.Recent).toHaveBeenCalledTimes(2)
  })

  it('runs each reorganization through its binding and then re-reads the space', async () => {
    mocks.CreateFolder.mockResolvedValue(folder('archive', []))
    mocks.Rename.mockResolvedValue(doc('b.md'))
    mocks.Move.mockResolvedValue(doc('archive/b.md'))
    mocks.Delete.mockResolvedValue(undefined)
    const drive = useDrive()
    await drive.ensureTree('acme/site')

    const steps: Array<[() => Promise<{ error: string | null }>, ReturnType<typeof vi.fn>, unknown[]]> = [
      [() => drive.createFolder('acme/site', 'archive'), mocks.CreateFolder, ['acme/site', 'archive']],
      [() => drive.rename('acme/site', 'a.md', 'b'), mocks.Rename, ['acme/site', 'a.md', 'b']],
      [() => drive.move('acme/site', 'b.md', 'archive'), mocks.Move, ['acme/site', 'b.md', 'archive']],
      [() => drive.remove('acme/site', 'archive/b.md'), mocks.Delete, ['acme/site', 'archive/b.md']],
    ]
    for (const [run, binding, args] of steps) {
      mocks.Tree.mockClear()
      expect((await run()).error).toBeNull()
      expect(binding).toHaveBeenCalledWith(...args)
      expect(mocks.Tree.mock.calls).toEqual([['acme/site']])
    }
    expect((await drive.move('acme/site', 'b.md', 'archive')).result?.path).toBe('archive/b.md')
  })

  it('answers a refused change as its error text and still re-reads the space', async () => {
    mocks.Move.mockRejectedValue(
      Object.assign(new Error('call failed'), { cause: { kind: 'conflict', message: '"plans/a.md" already exists' } }),
    )
    const drive = useDrive()
    await drive.ensureTree('acme/site')
    mocks.Tree.mockClear()

    expect(await drive.move('acme/site', 'a.md', 'plans')).toEqual({
      result: null,
      error: '"plans/a.md" already exists',
    })
    expect(mocks.Tree.mock.calls).toEqual([['acme/site']])
  })
})
