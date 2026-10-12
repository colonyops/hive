import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useDrive } from '../useDrive'
import { doc, root, space } from '../../test-utils/drive'

const mocks = vi.hoisted(() => ({
  Spaces: vi.fn(),
  Tree: vi.fn(),
  Search: vi.fn(),
  Focused: vi.fn(),
  On: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/driveservice', () => ({
  Spaces: mocks.Spaces,
  Tree: mocks.Tree,
  Search: mocks.Search,
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
})
