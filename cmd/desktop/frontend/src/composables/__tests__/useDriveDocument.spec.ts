import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { DocumentWire } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { useDrive } from '../../stores/useDrive'
import { doc, documentFor, root, space } from '../../test-utils/drive'
import { useDriveDocument } from '../useDriveDocument'

vi.mock('../useWailsEvent', () => ({ useWailsEvent: vi.fn() }))

const drive = vi.hoisted(() => ({
  Spaces: vi.fn(),
  Tree: vi.fn(),
  Search: vi.fn(),
  Document: vi.fn(),
  CreateFolder: vi.fn(),
  Delete: vi.fn(),
  Move: vi.fn(),
  Rename: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/driveservice', () => drive)
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/windowservice', () => ({
  Focused: vi.fn().mockResolvedValue(true),
}))

const spaceA = 'workspaces/a'
const spaceB = 'workspaces/b'
const docA = doc('a.md')
const docB = doc('b.md')
const trees: Record<string, ReturnType<typeof root>> = { [spaceA]: root([docA]), [spaceB]: root([docB]) }

interface Deferred {
  promise: Promise<DocumentWire>
  resolve: (value: DocumentWire) => void
  reject: (error: Error) => void
}

function deferred(): Deferred {
  let resolve!: Deferred['resolve']
  let reject!: Deferred['reject']
  const promise = new Promise<DocumentWire>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

function mountSurface(): ReturnType<typeof useDriveDocument> {
  let api!: ReturnType<typeof useDriveDocument>
  mount(
    defineComponent({
      setup() {
        api = useDriveDocument()
        return () => null
      },
    }),
  )
  return api
}

describe('useDriveDocument', () => {
  let pending: Map<string, Deferred>

  beforeEach(() => {
    vi.clearAllMocks()
    pending = new Map()
    drive.Spaces.mockResolvedValue([space(spaceA), space(spaceB)])
    drive.Tree.mockImplementation((key: string) => Promise.resolve(trees[key]))
    drive.Document.mockImplementation((key: string) => {
      if (!pending.has(key)) pending.set(key, deferred())
      return pending.get(key)!.promise
    })
  })

  async function raceAThenB(): Promise<ReturnType<typeof useDriveDocument>> {
    const surface = mountSurface()
    surface.show(spaceA, null, null)
    await flushPromises()
    expect(drive.Document).toHaveBeenLastCalledWith(spaceA, 'a.md')

    surface.show(spaceB, null, null)
    await flushPromises()
    expect(drive.Document).toHaveBeenLastCalledWith(spaceB, 'b.md')

    pending.get(spaceB)!.resolve(documentFor(spaceB, docB, { markdown: '# B' }))
    await flushPromises()
    return surface
  }

  it('keeps the newer request when an older document load answers last', async () => {
    const surface = await raceAThenB()
    pending.get(spaceA)!.resolve(documentFor(spaceA, docA, { markdown: '# A' }))
    await flushPromises()

    expect(surface.document.value?.space.key).toBe(spaceB)
    expect(surface.shown.value).toBe('b.md')
    expect(surface.loading.value).toBe(false)
  })

  it('ignores a failure from a superseded document load', async () => {
    const surface = await raceAThenB()
    pending.get(spaceA)!.reject(new Error('gone'))
    await flushPromises()

    expect(surface.error.value).toBeNull()
    expect(surface.document.value?.space.key).toBe(spaceB)
    expect(surface.loading.value).toBe(false)
  })

  it('loads the default document once a tree that failed first becomes available', async () => {
    drive.Tree.mockRejectedValueOnce(new Error('drive is busy'))
    const surface = mountSurface()
    surface.show(spaceA, null, null)
    await flushPromises()
    expect(drive.Document).not.toHaveBeenCalled()
    expect(surface.treeError.value).not.toBeNull()

    await useDrive().reload()
    await flushPromises()
    expect(drive.Document).toHaveBeenLastCalledWith(spaceA, 'a.md')

    pending.get(spaceA)!.resolve(documentFor(spaceA, docA, { markdown: '# A' }))
    await flushPromises()
    expect(surface.shown.value).toBe('a.md')
    expect(surface.document.value?.entry.path).toBe('a.md')
    expect(surface.loading.value).toBe(false)
  })
})
