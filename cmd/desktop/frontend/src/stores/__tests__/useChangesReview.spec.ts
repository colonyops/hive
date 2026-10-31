import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useChangesReview } from '../useChangesReview'
import { resetStores } from '../defineStore'

const mocks = vi.hoisted(() => ({
  writeText: vi.fn(),
  ChangesTargets: vi.fn(),
  ChangesFeedback: vi.fn(),
  SendChanges: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reviewservice', () => mocks)

vi.mock('../../lib/hiveUiPlatform', () => ({ hiveUiPlatform: { clipboard: { writeText: mocks.writeText } } }))

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

  it('keeps what was added or edited during the send, and records only what went', async () => {
    let finish!: () => void
    mocks.SendChanges.mockImplementation(() => new Promise<void>((resolve) => (finish = resolve)))
    const store = useChangesReview()
    const edited = store.add('s1', { ...span, path: 'a.go', text: 'before' })
    store.add('s1', { ...span, path: 'b.go', text: 'delivered' })

    const sending = store.send('s1', '%1')
    store.update('s1', edited.id, 'after')
    store.add('s1', { ...span, path: 'c.go', text: 'late' })
    finish()
    expect(await sending).toBeNull()

    expect(store.comments('s1').map((comment) => comment.text)).toEqual(['after', 'late'])
    expect(store.sentReviews('s1')[0].comments.map((comment) => comment.text)).toEqual(['before', 'delivered'])
  })

  it('keeps the last three sent reviews, newest first', async () => {
    mocks.SendChanges.mockResolvedValue(undefined)
    const store = useChangesReview()
    for (const text of ['one', 'two', 'three', 'four']) {
      store.add('s1', { ...span, path: 'a.go', text })
      await store.send('s1', '%1')
    }
    expect(store.sentReviews('s1').map((review) => review.comments[0].text)).toEqual(['four', 'three', 'two'])
    expect(store.sentReviews('s2')).toEqual([])
  })

  it('restores a sent review beside what was written since', async () => {
    mocks.SendChanges.mockResolvedValue(undefined)
    const store = useChangesReview()
    store.add('s1', { ...span, path: 'b.go', text: 'sent' })
    await store.send('s1', '%1')
    store.add('s1', { ...span, path: 'a.go', text: 'newer' })

    store.restore('s1', store.sentReviews('s1')[0].id)
    expect(store.comments('s1').map((comment) => comment.text)).toEqual(['newer', 'sent'])
    expect(store.sentReviews('s1')).toEqual([])
  })

  it('leaves a failed send out of the backlog', async () => {
    mocks.SendChanges.mockRejectedValue(new Error('pane gone'))
    const store = useChangesReview()
    store.add('s1', { ...span, path: 'a.go', text: 'x' })
    await store.send('s1', '%1')
    expect(store.sentReviews('s1')).toEqual([])
  })

  it('copies the text Go builds, and says why it could not', async () => {
    mocks.ChangesFeedback.mockResolvedValue('# feedback')
    mocks.writeText.mockResolvedValue(undefined)
    const store = useChangesReview()
    const comment = store.add('s1', { ...span, path: 'a.go', text: 'x' })
    expect(await store.copy([comment])).toBeNull()
    expect(mocks.ChangesFeedback).toHaveBeenCalledWith([comment])
    expect(mocks.writeText).toHaveBeenCalledWith('# feedback')

    mocks.writeText.mockRejectedValue(new Error('no clipboard'))
    expect(await store.copy([comment])).toContain('no clipboard')
  })

  it('forgets a session whole', async () => {
    mocks.SendChanges.mockResolvedValue(undefined)
    const store = useChangesReview()
    store.add('s1', { ...span, path: 'a.go', text: 'sent' })
    await store.send('s1', '%1')
    store.add('s1', { ...span, path: 'a.go', text: 'unsent' })
    store.forget('s1')
    expect(store.comments('s1')).toEqual([])
    expect(store.sentReviews('s1')).toEqual([])
  })

  it('keeps every comment and says why when a send fails', async () => {
    mocks.SendChanges.mockRejectedValue(new Error('pane gone'))
    const store = useChangesReview()
    store.add('s1', { ...span, path: 'a.go', text: 'x' })
    expect(await store.send('s1', '%1')).toContain('pane gone')
    expect(store.comments('s1')).toHaveLength(1)
  })
})
