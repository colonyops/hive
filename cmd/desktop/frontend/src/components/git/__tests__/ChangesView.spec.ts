import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import ChangesView from '../ChangesView.vue'
import { resetStores } from '../../../stores/defineStore'
import { resetToastsForTests, useToasts } from '../../../composables/useToasts'

const mocks = vi.hoisted(() => ({
  SessionChanges: vi.fn(),
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
    subscribeToScroll(): void {}
    getTopForItem(): number {
      return 0
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
    truncated: false,
    files,
    ...extra,
  }
}
const noState = { binary: false, truncated: false, unavailable: false, gone: false, error: '' }

async function mountView() {
  const wrapper = mount(ChangesView, { props: { sessionId: 's1' }, attachTo: document.body })
  await flushPromises()
  await flushPromises()
  return wrapper
}
const testid = (id: string) => `[data-testid="${id}"]`
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
    mocks.OpenSessionInEditor.mockResolvedValue(undefined)
    mocks.SessionChanges.mockResolvedValue(changes([file('a.go'), file('b.go')]))
    mocks.SessionFileDiffs.mockImplementation((_id: string, _base: string, paths: string[]) =>
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
    expect(mocks.SessionFileDiffs).toHaveBeenCalledWith('s1', 'branch', ['a.go', 'b.go'])
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
    expect(wrapper.get(testid('changes-file-b.go-header')).classes()).toContain('changes-file-current')
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
    const wrapper = await mountView()
    mocks.SessionFileDiffs.mockClear()
    await press('o')
    mocks.SessionChanges.mockResolvedValue(changes([file('a.go', { stamp: '2' }), file('b.go')]))
    await flushPromises()
    expect(mocks.SessionFileDiffs).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('shows each state a diff can have', async () => {
    mocks.SessionChanges.mockResolvedValue(
      changes(['bin', 'big', 'pipe', 'gone', 'bad', 'renamed'].map((path) => file(path))),
    )
    mocks.SessionFileDiffs.mockResolvedValue([
      { path: 'bin', text: '', ...noState, binary: true },
      { path: 'big', text: '', ...noState, truncated: true },
      { path: 'pipe', text: '', ...noState, unavailable: true },
      { path: 'gone', text: '', ...noState, gone: true },
      { path: 'bad', text: '', ...noState, error: 'exit 128' },
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
    expect(wrapper.find(testid('changes-base-branch')).exists()).toBe(false)
    expect(wrapper.get(testid('changes-base-uncommitted')).attributes('aria-pressed')).toBe('true')
    expect(wrapper.get(testid('changes-empty')).text()).toBe('No uncommitted changes')
    wrapper.unmount()
  })

  it('reads the other base when it is picked', async () => {
    const wrapper = await mountView()
    await wrapper.get(testid('changes-base-uncommitted')).trigger('click')
    await flushPromises()
    expect(mocks.SessionChanges).toHaveBeenLastCalledWith('s1', 'uncommitted')
    wrapper.unmount()
  })

  it('keeps the last good list beside a failed read', async () => {
    const wrapper = await mountView()
    mocks.SessionChanges.mockRejectedValue(new Error('fatal: bad HEAD'))
    await wrapper.get(testid('changes-base-uncommitted')).trigger('click')
    await flushPromises()
    expect(wrapper.get(testid('changes-error')).text()).toContain('fatal: bad HEAD')
    expect(wrapper.find(testid('changes-file-a.go-header')).exists()).toBe(true)
    wrapper.unmount()
  })

  it('leaves keys alone while a modifier is held', async () => {
    const wrapper = await mountView()
    await press('v', { metaKey: true })
    expect(wrapper.get(testid('changes-progress')).text()).toBe('0/2 viewed')
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
    expect(wrapper.get(testid('changes-review')).text()).toContain('rename this')
    expect(wrapper.get(testid('changes-send-count')).text()).toBe('1')
    expect(wrapper.get(testid('changes-row-a.go-comments')).text()).toBe('1')
    wrapper.unmount()
  })

  it('remembers a closed panel and leaves it closed for later comments', async () => {
    let wrapper = await mountView()
    await comment('first')
    await press('r')
    expect(wrapper.find(testid('changes-review')).exists()).toBe(false)
    await comment('second')
    expect(wrapper.find(testid('changes-review')).exists()).toBe(false)
    wrapper.unmount()

    wrapper = await mountView()
    expect(wrapper.find(testid('changes-review')).exists()).toBe(false)
    expect(wrapper.get(testid('changes-send-count')).text()).toBe('2')
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
