import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useChangesReview } from '../useChangesReview'
import { resetStores } from '../defineStore'

const mocks = vi.hoisted(() => ({
  ChangesTargets: vi.fn(),
  SendChanges: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reviewservice', () => mocks)

const span = {
  startSide: 'additions',
  startLine: 4,
  endSide: 'additions',
  endLine: 4,
  quote: '+x',
  base: 'branch',
  version: 'v1',
} as const

describe('useChangesReview', () => {
  beforeEach(() => {
    localStorage.clear()
    resetStores()
    vi.clearAllMocks()
  })

  it('keeps comments per session, ordered by file and line', () => {
    const store = useChangesReview()
    store.add('s1', { ...span, path: 'b.go', text: 'second' })
    store.add('s1', { ...span, path: 'a.go', startLine: 9, text: 'late' })
    store.add('s1', { ...span, path: 'a.go', startLine: 2, text: 'early' })
    store.add('s2', { ...span, path: 'c.go', text: 'other' })
    expect(store.comments('s1').map((comment) => comment.text)).toEqual(['early', 'late', 'second'])
    expect(store.comments('s2')).toHaveLength(1)
  })

  it('edits and removes one comment', () => {
    const store = useChangesReview()
    const kept = store.add('s1', { ...span, path: 'a.go', text: 'kept' })
    const gone = store.add('s1', { ...span, path: 'b.go', text: 'gone' })
    store.update('s1', kept.id, 'edited')
    store.remove('s1', gone.id)
    expect(store.comments('s1').map((comment) => comment.text)).toEqual(['edited'])
  })

  it('survives a relaunch', async () => {
    useChangesReview().add('s1', { ...span, path: 'a.go', text: 'written' })
    await nextTick()
    resetStores()
    const store = useChangesReview()
    expect(store.comments('s1').map((comment) => comment.text)).toEqual(['written'])
  })

  it('clears the review once it is sent', async () => {
    mocks.SendChanges.mockResolvedValue(undefined)
    const store = useChangesReview()
    const comment = store.add('s1', { ...span, path: 'a.go', text: 'x' })
    expect(await store.send('s1', '%1')).toBeNull()
    expect(mocks.SendChanges).toHaveBeenCalledWith('s1', '%1', [comment])
    expect(store.comments('s1')).toEqual([])
  })

  it('keeps every comment and says why when a send fails', async () => {
    mocks.SendChanges.mockRejectedValue(new Error('pane gone'))
    const store = useChangesReview()
    store.add('s1', { ...span, path: 'a.go', text: 'x' })
    expect(await store.send('s1', '%1')).toContain('pane gone')
    expect(store.comments('s1')).toHaveLength(1)
  })
})
