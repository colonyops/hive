import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import ChangesView from '../ChangesView.vue'
import { useChangesReview } from '../../../stores/useChangesReview'
import { useChangesViewed } from '../../../stores/useChangesViewed'
import { fileVersion } from '../../../composables/useFileDiffs'
import { resetStores } from '../../../stores/defineStore'
import { resetToastsForTests, useToasts } from '../../../composables/useToasts'

const mocks = vi.hoisted(() => ({
  SessionChanges: vi.fn(),
  SessionBranches: vi.fn(),
  scroll: null as ((top: number) => void) | null,
  tops: new Map<string, number>(),
  SessionFileDiffs: vi.fn(),
  OpenSessionInEditor: vi.fn(),
  setRenderOptions: vi.fn(() => Promise.resolve()),
  setOptions: vi.fn(),
  scrollTo: vi.fn(),
  setSelectedLines: vi.fn(),
  lineSelectionEnd: null as ((...args: unknown[]) => unknown) | null,
  ChangesTargets: vi.fn(),
  ChangesFeedback: vi.fn(),
  SendChanges: vi.fn(),
  writeText: vi.fn(),
}))
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice', () => ({
  SessionChanges: mocks.SessionChanges,
  SessionBranches: mocks.SessionBranches,
  SessionFileDiffs: mocks.SessionFileDiffs,
  OpenSessionInEditor: mocks.OpenSessionInEditor,
}))
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/reviewservice', () => ({
  ChangesTargets: mocks.ChangesTargets,
  ChangesFeedback: mocks.ChangesFeedback,
  SendChanges: mocks.SendChanges,
}))
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  ReviewLabels: vi.fn().mockResolvedValue([]),
  SetReviewLabels: vi.fn(),
}))
vi.mock('../../../lib/hiveUiPlatform', async (importOriginal) => {
  const original = await importOriginal<typeof import('../../../lib/hiveUiPlatform')>()
  return { ...original, hiveUiPlatform: { ...original.hiveUiPlatform, clipboard: { writeText: mocks.writeText } } }
})
vi.mock('../../../stores/useWindowFocus', () => ({ useWindowFocus: () => ({ focused: ref(true) }) }))
vi.mock('../../../lib/diffHighlight', () => ({
  diffWorkerPool: () => ({ setRenderOptions: mocks.setRenderOptions }),
  diffThemeName: (theme: string) => `code-${theme}`,
}))
// jsdom has no layout, so the fake mounts every item the way CodeView mounts the
// ones near the viewport: its header in its slot, and its lines unless collapsed.
vi.mock('@pierre/diffs', async (importOriginal) => {
  class FakeCodeView {
    root: HTMLElement | null = null
    constructor(public options: Record<string, (...args: unknown[]) => HTMLElement>) {
      mocks.setOptions.mockClear()
      mocks.lineSelectionEnd = options.onLineSelectionEnd
    }
    getRenderedItems(): unknown[] {
      return []
    }
    setSelectedLines = mocks.setSelectedLines
    clearSelectedLines(): void {}
    setup(root: HTMLElement): void {
      this.root = root
    }
    cleanUp(): void {}
    subscribeToScroll(listener: (top: number) => void): void {
      mocks.scroll = listener
    }
    getTopForItem(id: string): number {
      return mocks.tops.get(id) ?? 0
    }
    scrollTo = mocks.scrollTo
    setOptions = mocks.setOptions
    setItems(items: FakeItem[]): void {
      for (const item of items) {
        const id = `fake-${item.id}`
        const body = this.root?.querySelector(`[data-fake="${id}"]`) ?? document.createElement('div')
        body.setAttribute('data-fake', id)
        const host = this.options.renderCustomHeader(item.fileDiff, { item })
        const lines = item.collapsed
          ? ''
          : item.fileDiff.hunks.flatMap((hunk) => hunk.hunkContent).length
            ? [...item.fileDiff.deletionLines, ...item.fileDiff.additionLines].join('')
            : ''
        body.replaceChildren(host, document.createTextNode(lines))
        this.root?.append(body)
      }
    }
  }
  return { ...(await importOriginal<typeof import('@pierre/diffs')>()), CodeView: FakeCodeView }
})

interface FakeItem {
  id: string
  collapsed: boolean
  fileDiff: { hunks: { hunkContent: unknown[] }[]; deletionLines: string[]; additionLines: string[] }
}

function file(path: string, extra: Record<string, unknown> = {}) {
  return { path, oldPath: '', status: 'modified', additions: 1, deletions: 1, binary: false, stamp: '1', ...extra }
}
function changes(files: unknown[], extra: Record<string, unknown> = {}) {
  return {
    resolved: true,
    branch: 'feat/x',
    defaultBranch: 'main',
    base: 'branch',
    branchAvailable: true,
    against: '',
    commit: '',
    commits: [],
    truncated: false,
    files,
    ...extra,
  }
}
const noState = { state: 'text', error: '' }

async function mountView() {
  const wrapper = mount(ChangesView, { props: { sessionId: 's1' }, attachTo: document.body })
  await flushPromises()
  await flushPromises()
  return wrapper
}
const testid = (id: string) => `[data-testid="${id}"]`
async function pick(wrapper: VueWrapper, select: string, value: string) {
  await wrapper.get(testid(select)).trigger('click')
  await flushPromises()
  document.querySelector<HTMLElement>(testid(`${select}-option-${value}`))!.click()
  await flushPromises()
}
const COMMITS = [
  { id: 'c3', subject: 'third', author: 'a', unix: 3 },
  { id: 'c2', subject: 'second', author: 'a', unix: 2 },
  { id: 'c1', subject: 'first', author: 'a', unix: 1 },
]
async function selectLines(path: string, line: number) {
  mocks.lineSelectionEnd!({ start: line, side: 'additions', end: line, endSide: 'additions' }, { item: { id: path } })
  await flushPromises()
}
async function comment(text: string) {
  await selectLines('a.go', 1)
  const field = document.querySelector<HTMLTextAreaElement>(testid('changes-comment-text'))!
  field.value = text
  field.dispatchEvent(new Event('input'))
  await flushPromises()
  document.querySelector<HTMLButtonElement>(testid('changes-comment-save'))!.click()
  await flushPromises()
}
const agent = (pane: string, status: string) => ({ kind: 'session', id: 's1', pane, label: `app · ${pane}`, status })
const press = async (key: string, init: KeyboardEventInit = {}) => {
  window.dispatchEvent(new KeyboardEvent('keydown', { key, ...init }))
  await flushPromises()
}

describe('ChangesView', () => {
  beforeEach(() => {
    localStorage.clear()
    resetStores()
    resetToastsForTests()
    vi.clearAllMocks()
    mocks.tops.clear()
    mocks.OpenSessionInEditor.mockResolvedValue(undefined)
    mocks.SessionChanges.mockResolvedValue(changes([file('a.go'), file('b.go')]))
    mocks.SessionBranches.mockResolvedValue(['refs/heads/main', 'refs/heads/release', 'refs/remotes/origin/release'])
    mocks.SessionFileDiffs.mockImplementation((_id: string, _target: unknown, paths: string[]) =>
      Promise.resolve(paths.map((path) => ({ path, text: `@@ -1 +1 @@\n-old ${path}\n+new ${path}\n`, ...noState }))),
    )
  })
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('orders the stream as the rail does, folders first', async () => {
    mocks.SessionChanges.mockResolvedValue(changes([file('a.go'), file('z/b.go')]))
    const wrapper = await mountView()
    const order = wrapper.findAll('[data-fake]').map((body) => body.attributes('data-fake'))
    expect(order).toEqual(['fake-z/b.go', 'fake-a.go'])
    wrapper.unmount()
  })

  it('shows every diff in one scroll with its totals', async () => {
    const wrapper = await mountView()
    expect(wrapper.get(testid('changes-branch')).text()).toBe('feat/x')
    expect(wrapper.get(testid('changes-total')).text()).toBe('+2-2')
    expect(wrapper.get('[data-fake="fake-a.go"]').text()).toContain('new a.go')
    expect(wrapper.get('[data-fake="fake-b.go"]').text()).toContain('old b.go')
    expect(mocks.SessionFileDiffs).toHaveBeenCalledWith('s1', { base: 'branch', against: '', commit: '' }, [
      'a.go',
      'b.go',
    ])
    wrapper.unmount()
  })

  it('switches the viewer between side by side and unified', async () => {
    const wrapper = await mountView()
    await press('s')
    expect(mocks.setOptions).toHaveBeenLastCalledWith(expect.objectContaining({ diffStyle: 'split' }))
    await wrapper.get(testid('changes-mode')).trigger('click')
    await flushPromises()
    expect(mocks.setOptions).toHaveBeenLastCalledWith(expect.objectContaining({ diffStyle: 'unified' }))
    wrapper.unmount()
  })

  it('jumps to the next file with j', async () => {
    const wrapper = await mountView()
    await press('j')
    expect(mocks.scrollTo).toHaveBeenCalledWith({ type: 'item', id: 'b.go', align: 'start' })
    expect(wrapper.get(testid('changes-row-b.go')).attributes('aria-selected')).toBe('true')
    wrapper.unmount()
  })

  it('folds a viewed file, counts it, and tracks the current file', async () => {
    const wrapper = await mountView()
    await press('v')
    expect(wrapper.get(testid('changes-progress')).text()).toBe('1/2 viewed')
    expect(wrapper.get('[data-fake="fake-a.go"]').text()).not.toContain('new a.go')
    expect(wrapper.get(testid('changes-file-a.go-viewed')).attributes('aria-pressed')).toBe('true')

    await press('j')
    await press('v')
    expect(wrapper.get(testid('changes-progress')).text()).toBe('2/2 viewed')
    wrapper.unmount()
  })

  it('clears the viewed mark and reopens the file when it changes again', async () => {
    const wrapper = await mountView()
    await wrapper.get(testid('changes-file-a.go-viewed')).trigger('click')
    expect(wrapper.get(testid('changes-progress')).text()).toBe('1/2 viewed')

    mocks.SessionChanges.mockResolvedValue(changes([file('a.go', { stamp: '2' }), file('b.go')]))
    vi.useFakeTimers()
    try {
      const again = mount(ChangesView, { props: { sessionId: 's1' }, attachTo: document.body })
      await vi.advanceTimersByTimeAsync(0)
      await flushPromises()
      expect(again.get(testid('changes-progress')).text()).toBe('0/2 viewed')
      expect(again.get(testid('changes-file-a.go-header')).attributes('aria-expanded')).toBe('true')
      again.unmount()
    } finally {
      vi.useRealTimers()
    }
    wrapper.unmount()
  })

  it('folds and unfolds from the header without marking the file viewed', async () => {
    const wrapper = await mountView()
    const header = wrapper.get(testid('changes-file-a.go-header'))
    await header.trigger('click')
    expect(wrapper.get('[data-fake="fake-a.go"]').text()).not.toContain('new a.go')
    expect(wrapper.get(testid('changes-progress')).text()).toBe('0/2 viewed')
    await header.trigger('click')
    expect(wrapper.get('[data-fake="fake-a.go"]').text()).toContain('new a.go')
    wrapper.unmount()
  })

  it('adds a file to the stream only once its diff is read', async () => {
    let answer: (diffs: unknown[]) => void = () => {}
    mocks.SessionFileDiffs.mockImplementation(() => new Promise((resolve) => (answer = resolve)))
    const wrapper = await mountView()
    expect(wrapper.find('[data-fake]').exists()).toBe(false)
    answer([{ path: 'a.go', text: '@@ -1 +1 @@\n-old\n+new\n', ...noState }])
    await flushPromises()
    expect(wrapper.find('[data-fake="fake-a.go"]').exists()).toBe(true)
    expect(wrapper.find('[data-fake="fake-b.go"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('does not read a file while it is folded', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      const wrapper = await mountView()
      mocks.SessionFileDiffs.mockClear()
      await press('o')
      mocks.SessionChanges.mockResolvedValue(changes([file('a.go', { stamp: '2' }), file('b.go', { stamp: '2' })]))
      await vi.advanceTimersByTimeAsync(5_000)
      expect(mocks.SessionFileDiffs).toHaveBeenCalledExactlyOnceWith('s1', expect.anything(), ['b.go'])
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not jump back to a file that loads after a newer jump', async () => {
    let answer!: (diffs: unknown[]) => void
    mocks.SessionFileDiffs.mockImplementationOnce(() => new Promise((resolve) => (answer = resolve)))
    const wrapper = await mountView()
    await press('o')
    await press('j')
    await press('k')
    mocks.scrollTo.mockClear()

    answer([{ path: 'b.go', text: '@@ -1 +1 @@\n-old\n+new\n', ...noState }])
    await flushPromises()

    expect(mocks.scrollTo).not.toHaveBeenCalledWith(expect.objectContaining({ id: 'b.go' }))
    wrapper.unmount()
  })

  describe('stepping through hunks', () => {
    const TWO_HUNKS = '@@ -1,2 +1,2 @@\n-a\n+b\n c\n@@ -50,2 +50,2 @@\n-d\n+e\n f\n'
    const atPosition = (position: number) => ({ type: 'position', position })

    async function mountTwoFiles() {
      mocks.SessionFileDiffs.mockImplementation((_id: string, _target: unknown, paths: string[]) =>
        Promise.resolve(paths.map((path) => ({ path, text: TWO_HUNKS, ...noState }))),
      )
      mocks.tops.set('a.go', 0)
      mocks.tops.set('b.go', 1000)
      const wrapper = await mountView()
      mocks.scrollTo.mockClear()
      return wrapper
    }

    it('moves forward within a file and on to the next, and back again', async () => {
      const wrapper = await mountTwoFiles()
      await press('n')
      expect(mocks.scrollTo).toHaveBeenLastCalledWith(atPosition(60))

      mocks.scroll!(60)
      await press('n')
      expect(mocks.scrollTo).toHaveBeenLastCalledWith(atPosition(1000))

      mocks.scroll!(1000)
      await press('N')
      expect(mocks.scrollTo).toHaveBeenLastCalledWith(atPosition(60))
      wrapper.unmount()
    })

    it('measures hunks by side-by-side rows in that mode', async () => {
      const wrapper = await mountTwoFiles()
      await press('s')
      await press('n')
      expect(mocks.scrollTo).toHaveBeenLastCalledWith(atPosition(40))
      wrapper.unmount()
    })

    it('skips a folded file, and moves by file when no hunk is left', async () => {
      const wrapper = await mountTwoFiles()
      await press('j')
      await press('o')
      mocks.scroll!(0)
      await press('k')
      mocks.scroll!(60)
      mocks.scrollTo.mockClear()

      await press('n')
      expect(mocks.scrollTo).toHaveBeenCalledExactlyOnceWith({ type: 'item', id: 'b.go', align: 'start' })
      wrapper.unmount()
    })
  })

  describe('picking a comment', () => {
    const atLine = (id: string, lineNumber: number) => ({
      type: 'line',
      id,
      lineNumber,
      side: 'additions',
      align: 'center',
    })
    const span = {
      startSide: 'additions',
      startLine: 1,
      endSide: 'additions',
      endLine: 1,
      quote: '+new',
      text: 'note',
    } as const
    const pickFrom = async (wrapper: VueWrapper, id: string) => {
      await press('r')
      await wrapper.get(testid(`changes-comment-${id}-pick`)).trigger('click')
      await flushPromises()
    }

    it('switches comparison and scrolls once that diff is read', async () => {
      mocks.SessionChanges.mockImplementation((_id: string, target: { base: string }) =>
        Promise.resolve(changes([file('a.go'), file('b.go')], { base: target.base })),
      )
      const { id } = useChangesReview().add('s1', { ...span, path: 'b.go', base: 'staged', staged: true, version: 'v' })
      const wrapper = await mountView()
      let answer!: (diffs: unknown[]) => void
      mocks.SessionFileDiffs.mockImplementationOnce(() => new Promise((resolve) => (answer = resolve)))
      mocks.scrollTo.mockClear()

      await pickFrom(wrapper, id)
      expect(mocks.SessionFileDiffs).toHaveBeenLastCalledWith(
        's1',
        expect.objectContaining({ base: 'staged' }),
        expect.anything(),
      )
      expect(mocks.scrollTo).not.toHaveBeenCalledWith(atLine('b.go', 1))

      answer(['a.go', 'b.go'].map((path) => ({ path, text: `@@ -1 +1 @@\n-old\n+new ${path}\n`, ...noState })))
      await flushPromises()
      expect(mocks.scrollTo).toHaveBeenCalledWith(atLine('b.go', 1))
      expect(mocks.setSelectedLines).toHaveBeenLastCalledWith(
        { id: 'b.go', range: { start: 1, side: 'additions', end: 1, endSide: 'additions' } },
        { notify: false },
      )
      wrapper.unmount()
    })

    it('unfolds a viewed file and scrolls once its diff is read', async () => {
      useChangesViewed().setViewed('s1', 'a.go', fileVersion(file('a.go')), true)
      const { id } = useChangesReview().add('s1', { ...span, path: 'a.go', base: 'branch', version: 'v' })
      const wrapper = await mountView()
      expect(mocks.SessionFileDiffs).not.toHaveBeenCalledWith('s1', expect.anything(), expect.arrayContaining(['a.go']))
      mocks.scrollTo.mockClear()

      await pickFrom(wrapper, id)
      expect(mocks.SessionFileDiffs).toHaveBeenLastCalledWith('s1', expect.anything(), ['a.go'])
      expect(mocks.scrollTo).toHaveBeenCalledWith(atLine('a.go', 1))
      wrapper.unmount()
    })

    it('goes to the file when the line has left the diff', async () => {
      const { id } = useChangesReview().add('s1', { ...span, endLine: 99, path: 'b.go', base: 'branch', version: 'v' })
      const wrapper = await mountView()
      mocks.scrollTo.mockClear()

      await pickFrom(wrapper, id)
      expect(mocks.scrollTo).toHaveBeenCalledExactlyOnceWith({ type: 'item', id: 'b.go', align: 'start' })
      wrapper.unmount()
    })

    it('stays put when the file has left the comparison', async () => {
      const { id } = useChangesReview().add('s1', { ...span, path: 'gone.go', base: 'branch', version: 'v' })
      const wrapper = await mountView()
      mocks.scrollTo.mockClear()

      await pickFrom(wrapper, id)
      expect(mocks.scrollTo).not.toHaveBeenCalled()
      wrapper.unmount()
    })

    it('stays put when the comparison falls back to another one', async () => {
      mocks.SessionChanges.mockResolvedValue(changes([file('b.go')], { base: 'uncommitted' }))
      const { id } = useChangesReview().add('s1', { ...span, path: 'b.go', base: 'branch', version: 'v' })
      const wrapper = await mountView()
      mocks.scrollTo.mockClear()

      await pickFrom(wrapper, id)
      expect(mocks.scrollTo).not.toHaveBeenCalled()
      wrapper.unmount()
    })

    it('gives way to a jump the user makes while the diff is read', async () => {
      const { id } = useChangesReview().add('s1', { ...span, path: 'b.go', base: 'staged', staged: true, version: 'v' })
      mocks.SessionChanges.mockImplementation((_id: string, target: { base: string }) =>
        Promise.resolve(changes([file('a.go'), file('b.go')], { base: target.base })),
      )
      const wrapper = await mountView()
      let answer!: (diffs: unknown[]) => void
      mocks.SessionFileDiffs.mockImplementationOnce(() => new Promise((resolve) => (answer = resolve)))
      await pickFrom(wrapper, id)
      await press('k')

      answer(['a.go', 'b.go'].map((path) => ({ path, text: '@@ -1 +1 @@\n-old\n+new\n', ...noState })))
      await flushPromises()
      expect(mocks.scrollTo).not.toHaveBeenCalledWith(atLine('b.go', 1))
      wrapper.unmount()
    })
  })

  it('keeps a draft to the comparison it was started in', async () => {
    mocks.SessionChanges.mockImplementation((_id: string, target: { base: string }) =>
      Promise.resolve(changes([file('a.go')], { base: target.base })),
    )
    const wrapper = await mountView()
    await selectLines('a.go', 1)
    const field = document.querySelector<HTMLTextAreaElement>(testid('changes-comment-text'))!
    field.value = 'on the branch diff'
    field.dispatchEvent(new Event('input'))

    await press('[')
    mocks.SessionChanges.mockResolvedValue(changes([], { base: 'commit' }))
    await flushPromises()
    document.querySelector<HTMLButtonElement>(testid('changes-comment-save'))!.click()
    await flushPromises()

    expect(useChangesReview().comments('s1')).toMatchObject([
      { path: 'a.go', base: 'branch', text: 'on the branch diff' },
    ])
    wrapper.unmount()
  })

  it('marks a comment outdated only in the comparison it belongs to', async () => {
    mocks.SessionChanges.mockImplementation((_id: string, target: { base: string }) =>
      Promise.resolve(changes([file('a.go', { stamp: target.base })], { base: target.base })),
    )
    const wrapper = await mountView()
    await comment('note')
    await press('r')
    expect(wrapper.find('[data-testid$="-outdated"]').exists()).toBe(false)

    await press('[')
    await flushPromises()
    expect(wrapper.find('[data-testid$="-outdated"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows each state a diff can have', async () => {
    mocks.SessionChanges.mockResolvedValue(
      changes(['bin', 'big', 'pipe', 'gone', 'bad', 'renamed'].map((path) => file(path))),
    )
    mocks.SessionFileDiffs.mockResolvedValue([
      { path: 'bin', text: '', state: 'binary', error: '' },
      { path: 'big', text: '', state: 'truncated', error: '' },
      { path: 'pipe', text: '', state: 'unavailable', error: '' },
      { path: 'gone', text: '', state: 'gone', error: '' },
      { path: 'bad', text: '', state: 'error', error: 'exit 128' },
      { path: 'renamed', text: '', ...noState },
    ])
    const wrapper = await mountView()
    expect(wrapper.get(testid('changes-file-bin-note')).text()).toContain('Binary file')
    expect(wrapper.get(testid('changes-file-big-note')).text()).toContain('too large')
    expect(wrapper.get(testid('changes-file-pipe-note')).text()).toContain('symlink')
    expect(wrapper.get(testid('changes-file-gone-note')).text()).toContain('No longer changed')
    expect(wrapper.get(testid('changes-file-bad-note')).text()).toContain('exit 128')
    expect(wrapper.get(testid('changes-file-renamed-note')).text()).toContain('No content changes')

    await wrapper.get(testid('changes-file-big-header')).get('button').trigger('click')
    expect(mocks.OpenSessionInEditor).toHaveBeenCalledWith('s1')
    wrapper.unmount()
  })

  it('hides Branch and says why the list is empty when only uncommitted work applies', async () => {
    mocks.SessionChanges.mockResolvedValue(changes([], { base: 'uncommitted', branchAvailable: false }))
    const wrapper = await mountView()
    await wrapper.get(testid('changes-target')).trigger('click')
    await flushPromises()
    expect(document.querySelector(testid('changes-target-option-branch'))).toBeNull()
    expect(document.querySelector(testid('changes-target-option-uncommitted'))).not.toBeNull()
    expect(wrapper.get(testid('changes-empty')).text()).toBe('No uncommitted changes')
    wrapper.unmount()
  })

  it.each(['uncommitted', 'staged', 'unstaged'])('reads %s when it is picked', async (base) => {
    const wrapper = await mountView()
    await pick(wrapper, 'changes-target', base)
    expect(mocks.SessionChanges).toHaveBeenLastCalledWith('s1', { base, against: '', commit: '' })
    wrapper.unmount()
  })

  it('compares against the branch picked, for this session only', async () => {
    let wrapper = await mountView()
    await pick(wrapper, 'changes-against', 'refs/remotes/origin/release')
    expect(mocks.SessionChanges).toHaveBeenLastCalledWith('s1', {
      base: 'branch',
      against: 'refs/remotes/origin/release',
      commit: '',
    })
    wrapper.unmount()

    wrapper = mount(ChangesView, { props: { sessionId: 's2' }, attachTo: document.body })
    await flushPromises()
    expect(mocks.SessionChanges).toHaveBeenLastCalledWith('s2', { base: 'branch', against: '', commit: '' })
    wrapper.unmount()
  })

  it('offers only the default branch when the branches cannot be listed', async () => {
    mocks.SessionBranches.mockRejectedValue(new Error('daemon gone'))
    const wrapper = await mountView()
    await wrapper.get(testid('changes-against')).trigger('click')
    await flushPromises()
    expect(document.querySelector(testid('changes-against-option-'))!.textContent).toContain('main (default)')
    expect(document.querySelector(testid('changes-against-option-refs/heads/release'))).toBeNull()
    wrapper.unmount()
  })

  it('steps through the commits from the keyboard and stops at each end', async () => {
    mocks.SessionChanges.mockImplementation((_id: string, target: { base: string; commit: string }) =>
      Promise.resolve(
        changes([file('a.go')], {
          base: target.base,
          commits: COMMITS,
          commit: target.base === 'commit' ? target.commit || 'c3' : '',
        }),
      ),
    )
    const wrapper = await mountView()

    await press('[')
    expect(mocks.SessionChanges).toHaveBeenLastCalledWith('s1', { base: 'commit', against: '', commit: '' })
    expect(wrapper.get(testid('changes-commit-position')).text()).toBe('3/3')
    expect(wrapper.get(testid('changes-commit-newer')).attributes('disabled')).toBeDefined()

    await press('[')
    expect(mocks.SessionChanges).toHaveBeenLastCalledWith('s1', { base: 'commit', against: '', commit: 'c2' })
    await press('[')
    expect(wrapper.get(testid('changes-commit-position')).text()).toBe('1/3')
    expect(wrapper.get(testid('changes-commit-older')).attributes('disabled')).toBeDefined()

    mocks.SessionChanges.mockClear()
    await press('[')
    expect(mocks.SessionChanges).not.toHaveBeenCalled()

    await press(']')
    expect(mocks.SessionChanges).toHaveBeenLastCalledWith('s1', { base: 'commit', against: '', commit: 'c2' })
    wrapper.unmount()
  })

  describe('loading', () => {
    const loader = (wrapper: VueWrapper) => wrapper.find(testid('changes-loading')).exists()

    it('covers the tree and the diffs until the first diffs land, so they appear together', async () => {
      let answerList: (value: unknown) => void = () => {}
      let answerDiffs: (value: unknown) => void = () => {}
      mocks.SessionChanges.mockImplementationOnce(() => new Promise((resolve) => (answerList = resolve)))
      mocks.SessionFileDiffs.mockImplementationOnce(() => new Promise((resolve) => (answerDiffs = resolve)))
      const wrapper = await mountView()
      expect(loader(wrapper)).toBe(true)

      answerList(changes([file('a.go')]))
      await flushPromises()
      expect(loader(wrapper)).toBe(true)

      answerDiffs([{ path: 'a.go', text: '@@ -1 +1 @@\n-old\n+new\n', ...noState }])
      await flushPromises()
      expect(loader(wrapper)).toBe(false)
      wrapper.unmount()
    })

    it('covers them again when the target moves', async () => {
      const wrapper = await mountView()
      expect(loader(wrapper)).toBe(false)
      mocks.SessionChanges.mockImplementationOnce(() => new Promise(() => {}))
      await pick(wrapper, 'changes-target', 'staged')
      expect(loader(wrapper)).toBe(true)
      wrapper.unmount()
    })

    it('leaves a poll alone', async () => {
      vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
      try {
        const wrapper = await mountView()
        mocks.SessionChanges.mockClear()
        mocks.SessionChanges.mockImplementationOnce(() => new Promise(() => {}))
        await vi.advanceTimersByTimeAsync(5_000)
        expect(mocks.SessionChanges).toHaveBeenCalledOnce()
        expect(loader(wrapper)).toBe(false)
        wrapper.unmount()
      } finally {
        vi.useRealTimers()
      }
    })

    it('gives way to the error when the file list cannot be read', async () => {
      mocks.SessionChanges.mockRejectedValueOnce(new Error('no checkout'))
      const wrapper = await mountView()
      expect(loader(wrapper)).toBe(false)
      expect(wrapper.find(testid('changes-error')).exists()).toBe(true)
      wrapper.unmount()
    })

    it.each([
      ['a failed read', () => Promise.reject(new Error('git failed'))],
      ['an empty answer', () => Promise.resolve([])],
    ])('stops waiting after %s', async (_name, read) => {
      mocks.SessionFileDiffs.mockImplementationOnce(read)
      const wrapper = await mountView()
      expect(loader(wrapper)).toBe(false)
      wrapper.unmount()
    })

    it('does not wait for diffs when no file changed', async () => {
      mocks.SessionChanges.mockResolvedValue(changes([]))
      const wrapper = await mountView()
      expect(loader(wrapper)).toBe(false)
      expect(wrapper.find(testid('changes-empty')).exists()).toBe(true)
      wrapper.unmount()
    })
  })

  it('reads nothing against a new target until its file list lands', async () => {
    const wrapper = await mountView()
    mocks.SessionFileDiffs.mockClear()
    let answer: (value: unknown) => void = () => {}
    mocks.SessionChanges.mockImplementationOnce(() => new Promise((resolve) => (answer = resolve)))
    await pick(wrapper, 'changes-target', 'staged')
    expect(mocks.SessionFileDiffs).not.toHaveBeenCalled()
    expect(wrapper.find(testid('changes-empty')).exists()).toBe(false)

    answer(changes([file('staged.go')], { base: 'staged' }))
    await flushPromises()
    expect(mocks.SessionFileDiffs).toHaveBeenLastCalledWith('s1', expect.objectContaining({ base: 'staged' }), [
      'staged.go',
    ])
    wrapper.unmount()
  })

  it('applies the current theme on opening, since the pool outlives a closed view', async () => {
    const wrapper = await mountView()
    expect(mocks.setRenderOptions).toHaveBeenCalledWith({ theme: expect.stringMatching(/^code-/) })
    wrapper.unmount()
  })

  it('keeps the last good list beside a failed poll', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      const wrapper = await mountView()
      mocks.SessionChanges.mockRejectedValue(new Error('fatal: bad HEAD'))
      await vi.advanceTimersByTimeAsync(5_000)
      expect(wrapper.get(testid('changes-error')).text()).toContain('fatal: bad HEAD')
      expect(wrapper.find(testid('changes-file-a.go-header')).exists()).toBe(true)
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it("does not read the last target's files against a target that failed to load", async () => {
    const wrapper = await mountView()
    mocks.SessionFileDiffs.mockClear()
    mocks.SessionChanges.mockRejectedValue(new Error('fatal: bad ref'))
    await pick(wrapper, 'changes-target', 'uncommitted')
    expect(wrapper.get(testid('changes-error')).text()).toContain('fatal: bad ref')
    expect(mocks.SessionFileDiffs).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('hides and shows the file list, and remembers it', async () => {
    let wrapper = await mountView()
    await wrapper.get(testid('changes-files-toggle')).trigger('click')
    expect(wrapper.find(testid('changes-files')).exists()).toBe(false)
    expect(wrapper.find(testid('changes-file-a.go-header')).exists()).toBe(true)
    wrapper.unmount()

    wrapper = await mountView()
    expect(wrapper.find(testid('changes-files')).exists()).toBe(false)
    await press('b')
    expect(wrapper.find(testid('changes-files')).exists()).toBe(true)
    wrapper.unmount()
  })

  it('collapses and expands every folder at once', async () => {
    mocks.SessionChanges.mockResolvedValue(changes([file('x/a.go'), file('x/y/b.go'), file('z/c.go')]))
    const wrapper = await mountView()
    const toggle = () => wrapper.get(testid('changes-folders-toggle'))
    expect(toggle().attributes('aria-label')).toBe('Collapse all folders')

    await toggle().trigger('click')
    expect(wrapper.find(testid('changes-row-x/a.go')).exists()).toBe(false)
    expect(wrapper.find(testid('changes-row-z/c.go')).exists()).toBe(false)
    expect(toggle().attributes('aria-label')).toBe('Expand all folders')

    await toggle().trigger('click')
    expect(wrapper.find(testid('changes-row-x/y/b.go')).exists()).toBe(true)
    wrapper.unmount()
  })

  it('pins the header of the file at the top, and the next header pushes it out', async () => {
    mocks.tops.set('a.go', 0).set('b.go', 1000)
    const wrapper = await mountView()
    const pinned = () => wrapper.get(testid('changes-pinned-header'))
    const push = () => pinned().element.parentElement!.style.transform

    mocks.scroll!(500)
    await flushPromises()
    expect(pinned().text()).toContain('a.go')
    expect(push()).toBe('translateY(0px)')

    mocks.scroll!(980)
    await flushPromises()
    expect(pinned().text()).toContain('a.go')
    expect(push()).toBe('translateY(-12px)')

    mocks.scroll!(1000)
    await flushPromises()
    expect(pinned().text()).toContain('b.go')
    expect(push()).toBe('translateY(0px)')

    await wrapper.get(testid('changes-pinned-viewed')).trigger('click')
    expect(wrapper.get(testid('changes-progress')).text()).toBe('1/2 viewed')
    wrapper.unmount()
  })

  it('hides viewed files from the list but not from the diffs', async () => {
    const wrapper = await mountView()
    await press('v')
    expect(wrapper.find(testid('changes-row-a.go')).exists()).toBe(true)
    await wrapper.get(testid('changes-hide-viewed')).trigger('click')
    expect(wrapper.find(testid('changes-row-a.go')).exists()).toBe(false)
    expect(wrapper.find(testid('changes-file-a.go-header')).exists()).toBe(true)
    wrapper.unmount()
  })

  it('names the commit a comment was made on, and returns to it when picked', async () => {
    mocks.SessionChanges.mockImplementation((_id: string, target: { base: string; commit: string }) =>
      Promise.resolve(
        changes([file('a.go')], {
          base: target.base,
          commits: COMMITS,
          commit: target.base === 'commit' ? target.commit || 'c3' : '',
        }),
      ),
    )
    const wrapper = await mountView()
    await press('[')
    await comment('at this commit')
    const [saved] = useChangesReview().comments('s1')
    expect(saved).toMatchObject({ base: 'commit', commit: 'c3' })
    expect(wrapper.find(testid('changes-review')).exists()).toBe(false)

    await press('[')
    expect(mocks.SessionChanges).toHaveBeenLastCalledWith('s1', { base: 'commit', against: '', commit: 'c2' })
    await press('r')
    await wrapper.get(testid('changes-review')).get('button').trigger('click')
    await flushPromises()
    expect(mocks.SessionChanges).toHaveBeenLastCalledWith('s1', { base: 'commit', against: '', commit: 'c3' })
    wrapper.unmount()
  })

  it('leaves keys alone while a modifier is held', async () => {
    const wrapper = await mountView()
    await press('v', { metaKey: true })
    expect(wrapper.get(testid('changes-progress')).text()).toBe('0/2 viewed')
    wrapper.unmount()
  })

  it('takes Ctrl for search and send where there is no Command key', async () => {
    mocks.ChangesTargets.mockResolvedValue([agent('%1', 'ready')])
    mocks.SendChanges.mockResolvedValue(undefined)
    const wrapper = await mountView()
    await press('f', { ctrlKey: true })
    expect(wrapper.find(testid('changes-search')).exists()).toBe(true)

    await comment('note')
    await press('Enter', { ctrlKey: true })
    await flushPromises()
    expect(mocks.SendChanges).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('opens a composer on selected lines and files the comment under the panel', async () => {
    const wrapper = await mountView()
    expect(wrapper.find(testid('changes-review')).exists()).toBe(false)
    expect(wrapper.find(testid('changes-send')).exists()).toBe(false)

    await selectLines('a.go', 1)
    expect(document.querySelector(testid('changes-comment-popover'))!.textContent).toContain('a.go L1')
    expect(mocks.setSelectedLines).toHaveBeenCalled()

    const field = document.querySelector<HTMLTextAreaElement>(testid('changes-comment-text'))!
    field.value = 'rename this'
    field.dispatchEvent(new Event('input'))
    await flushPromises()
    document.querySelector<HTMLButtonElement>(testid('changes-comment-save'))!.click()
    await flushPromises()

    expect(document.querySelector(testid('changes-comment-popover'))).toBeNull()
    expect(wrapper.find(testid('changes-review')).exists()).toBe(false)
    await press('r')
    expect(wrapper.get(testid('changes-review')).text()).toContain('rename this')
    expect(wrapper.get(testid('changes-send-count')).text()).toBe('1')
    expect(wrapper.get(testid('changes-row-a.go-comments')).text()).toBe('1')
    wrapper.unmount()
  })

  it('starts with the panel closed every time, even one left open', async () => {
    let wrapper = await mountView()
    await comment('first')
    await press('r')
    expect(wrapper.find(testid('changes-review')).exists()).toBe(true)
    wrapper.unmount()

    wrapper = await mountView()
    expect(wrapper.find(testid('changes-review')).exists()).toBe(false)
    expect(wrapper.get(testid('changes-send-count')).text()).toBe('1')
    wrapper.unmount()
  })

  it.each(['ready', 'active'])('sends straight to a lone %s agent and clears the review', async (status) => {
    mocks.ChangesTargets.mockResolvedValue([agent('%1', status)])
    mocks.SendChanges.mockResolvedValue(undefined)
    const wrapper = await mountView()
    await comment('rename this')
    await wrapper.get(testid('changes-send')).trigger('click')
    await flushPromises()
    expect(mocks.SendChanges).toHaveBeenCalledWith('s1', '%1', [expect.objectContaining({ text: 'rename this' })])
    expect(wrapper.find(testid('changes-send')).exists()).toBe(false)
    expect(wrapper.find(testid('changes-review')).exists()).toBe(false)
    wrapper.unmount()
  })

  it.each([['several', [agent('%1', 'ready'), agent('%2', 'ready')]]])(
    'asks before sending with %s',
    async (_name, targets) => {
      mocks.ChangesTargets.mockResolvedValue(targets)
      const wrapper = await mountView()
      await comment('rename this')
      await wrapper.get(testid('changes-send')).trigger('click')
      await flushPromises()
      expect(mocks.SendChanges).not.toHaveBeenCalled()
      expect(document.querySelector(testid('changes-send-dialog'))).not.toBeNull()
      wrapper.unmount()
    },
  )

  it('saves with ⌘↵ in the composer without sending, and sends with it outside a field', async () => {
    mocks.ChangesTargets.mockResolvedValue([agent('%1', 'ready')])
    mocks.SendChanges.mockResolvedValue(undefined)
    const wrapper = await mountView()
    await selectLines('a.go', 1)
    const field = document.querySelector<HTMLTextAreaElement>(testid('changes-comment-text'))!
    field.value = 'rename this'
    field.dispatchEvent(new Event('input'))
    await flushPromises()
    field.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', metaKey: true, bubbles: true }))
    await flushPromises()
    expect(wrapper.get(testid('changes-send-count')).text()).toBe('1')
    expect(mocks.ChangesTargets).not.toHaveBeenCalled()

    await press('Enter', { metaKey: true })
    expect(mocks.SendChanges).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  async function failedSend(setup: () => void) {
    setup()
    mocks.ChangesFeedback.mockResolvedValue('# feedback')
    mocks.writeText.mockResolvedValue(undefined)
    const wrapper = await mountView()
    await comment('rename this')
    await wrapper.get(testid('changes-send')).trigger('click')
    await flushPromises()
    return wrapper
  }

  it.each([
    [
      'Go refuses an agent on a prompt',
      () => {
        mocks.ChangesTargets.mockResolvedValue([agent('%1', 'approval')])
        mocks.SendChanges.mockRejectedValue(new Error('waiting on a prompt'))
      },
      'waiting on a prompt',
    ],
    ['no agent is running', () => mocks.ChangesTargets.mockResolvedValue([]), 'No agent is running'],
    ['the agents cannot be listed', () => mocks.ChangesTargets.mockRejectedValue(new Error('tmux down')), 'tmux down'],
  ])('keeps the comments and offers them as text when %s', async (_name, setup, message) => {
    const wrapper = await failedSend(setup)
    expect(wrapper.get(testid('changes-send-count')).text()).toBe('1')
    const [toast] = useToasts().toasts.value
    expect(toast.message).toContain(message)
    expect(toast.duration).toBe(0)

    toast.actions[0].onClick()
    await flushPromises()
    expect(mocks.ChangesFeedback).toHaveBeenCalledWith([expect.objectContaining({ text: 'rename this' })])
    expect(mocks.writeText).toHaveBeenCalledWith('# feedback')
    wrapper.unmount()
  })

  it('totals a collapsed folder comments on its row', async () => {
    mocks.SessionChanges.mockResolvedValue(changes([file('z/b.go'), file('z/c.go')]))
    const wrapper = await mountView()
    for (const path of ['z/b.go', 'z/c.go']) {
      await selectLines(path, 1)
      const field = document.querySelector<HTMLTextAreaElement>(testid('changes-comment-text'))!
      field.value = 'note'
      field.dispatchEvent(new Event('input'))
      await flushPromises()
      document.querySelector<HTMLButtonElement>(testid('changes-comment-save'))!.click()
      await flushPromises()
    }
    expect(wrapper.find(testid('changes-row-z-comments')).exists()).toBe(false)

    await wrapper
      .findAll('[role="treeitem"]')
      .find((row) => row.text() === 'z')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.get(testid('changes-row-z-comments')).text()).toBe('2')
    wrapper.unmount()
  })

  it('lists sent reviews in the panel and restores one', async () => {
    mocks.ChangesTargets.mockResolvedValue([agent('%1', 'ready')])
    mocks.SendChanges.mockResolvedValue(undefined)
    const wrapper = await mountView()
    await comment('rename this')
    await wrapper.get(testid('changes-send')).trigger('click')
    await flushPromises()
    await press('r')
    expect(wrapper.get(testid('changes-sent-0')).text()).toContain('1 comment')

    await wrapper.get(testid('changes-sent-0-restore')).trigger('click')
    await flushPromises()
    expect(wrapper.get(testid('changes-send-count')).text()).toBe('1')
    expect(wrapper.find(testid('changes-sent')).exists()).toBe(false)
    wrapper.unmount()
  })

  it('searches with / and steps through matches, unfolding the file a match is in', async () => {
    const wrapper = await mountView()
    await press('j')
    await press('v')
    expect(wrapper.get('[data-fake="fake-b.go"]').text()).not.toContain('new b.go')

    await press('/')
    const input = wrapper.get(testid('changes-search-input'))
    await input.setValue('new')
    await new Promise((resolve) => setTimeout(resolve, 170))
    await flushPromises()
    expect(wrapper.get(testid('changes-search-count')).text()).toBe('1 of 2')
    expect(mocks.scrollTo).toHaveBeenLastCalledWith(
      expect.objectContaining({ type: 'line', id: 'a.go', lineNumber: 1 }),
    )

    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(wrapper.get(testid('changes-search-count')).text()).toBe('2 of 2')
    expect(mocks.scrollTo).toHaveBeenLastCalledWith(
      expect.objectContaining({ type: 'line', id: 'b.go', lineNumber: 1 }),
    )
    expect(wrapper.get('[data-fake="fake-b.go"]').text()).toContain('new b.go')

    await input.trigger('keydown', { key: 'Enter', shiftKey: true })
    expect(wrapper.get(testid('changes-search-count')).text()).toBe('1 of 2')
    wrapper.unmount()
  })

  it('closes on Escape and from its button', async () => {
    const wrapper = await mountView()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toHaveLength(1)
    await wrapper.get(testid('changes-close')).trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(2)
    wrapper.unmount()
  })
})
