import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import ChangesView from '../ChangesView.vue'
import { resetStores } from '../../../stores/defineStore'

const mocks = vi.hoisted(() => ({
  SessionChanges: vi.fn(),
  SessionFileDiffs: vi.fn(),
  OpenSessionInEditor: vi.fn(),
}))
vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice', () => ({
  SessionChanges: mocks.SessionChanges,
  SessionFileDiffs: mocks.SessionFileDiffs,
  OpenSessionInEditor: mocks.OpenSessionInEditor,
}))
vi.mock('../../../stores/useWindowFocus', () => ({ useWindowFocus: () => ({ focused: ref(true) }) }))
vi.mock('@hive/ui/lib/highlight', () => ({ useCodeHighlight: () => ({ highlight: () => null }) }))

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

// jsdom has no layout, so every section the observer watches counts as near.
class EverythingNear {
  constructor(private callback: IntersectionObserverCallback) {}
  observe(target: Element): void {
    this.callback(
      [{ target, isIntersecting: true } as unknown as IntersectionObserverEntry],
      this as unknown as IntersectionObserver,
    )
  }
  disconnect(): void {}
}

async function mountView() {
  const wrapper = mount(ChangesView, { props: { sessionId: 's1' }, attachTo: document.body })
  await flushPromises()
  await flushPromises()
  return wrapper
}
const testid = (id: string) => `[data-testid="${id}"]`
const press = async (key: string, init: KeyboardEventInit = {}) => {
  window.dispatchEvent(new KeyboardEvent('keydown', { key, ...init }))
  await flushPromises()
}

describe('ChangesView', () => {
  beforeEach(() => {
    localStorage.clear()
    resetStores()
    vi.clearAllMocks()
    vi.stubGlobal('IntersectionObserver', EverythingNear)
    mocks.OpenSessionInEditor.mockResolvedValue(undefined)
    mocks.SessionChanges.mockResolvedValue(changes([file('a.go'), file('b.go')]))
    mocks.SessionFileDiffs.mockImplementation((_id: string, _base: string, paths: string[]) =>
      Promise.resolve(paths.map((path) => ({ path, text: `@@ -1 +1 @@\n-old ${path}\n+new ${path}\n`, ...noState }))),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('orders the stream as the rail does, folders first', async () => {
    mocks.SessionChanges.mockResolvedValue(changes([file('a.go'), file('z/b.go')]))
    const wrapper = await mountView()
    const order = wrapper.findAll('section[data-file]').map((section) => section.attributes('data-file'))
    expect(order).toEqual(['z/b.go', 'a.go'])
    wrapper.unmount()
  })

  it('shows every diff in one scroll with its totals', async () => {
    const wrapper = await mountView()
    expect(wrapper.get(testid('changes-branch')).text()).toBe('feat/x')
    expect(wrapper.get(testid('changes-total')).text()).toBe('+2-2')
    expect(wrapper.get(testid('changes-file-a.go')).text()).toContain('new a.go')
    expect(wrapper.get(testid('changes-file-b.go')).text()).toContain('old b.go')
    expect(mocks.SessionFileDiffs).toHaveBeenCalledWith('s1', 'branch', ['a.go', 'b.go'])
    wrapper.unmount()
  })

  it('renders both sides of a line in side by side mode', async () => {
    const wrapper = await mountView()
    await press('s')
    expect(wrapper.findAll('.diff-pair').length).toBeGreaterThan(0)
    await wrapper.get(testid('changes-mode')).trigger('click')
    expect(wrapper.findAll('.diff-pair')).toHaveLength(0)
    wrapper.unmount()
  })

  it('folds a viewed file, counts it, and tracks the current file', async () => {
    const wrapper = await mountView()
    await press('v')
    expect(wrapper.get(testid('changes-progress')).text()).toBe('1/2 viewed')
    expect(wrapper.get(testid('changes-file-a.go')).text()).not.toContain('new a.go')
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
    expect(wrapper.get(testid('changes-file-a.go')).text()).not.toContain('new a.go')
    expect(wrapper.get(testid('changes-progress')).text()).toBe('0/2 viewed')
    await header.trigger('click')
    expect(wrapper.get(testid('changes-file-a.go')).text()).toContain('new a.go')
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
    expect(wrapper.get(testid('changes-file-gone-note')).text()).toContain('no longer changed')
    expect(wrapper.get(testid('changes-file-bad')).text()).toContain('exit 128')
    expect(wrapper.get(testid('changes-file-renamed-note')).text()).toContain('No content changes')

    await wrapper.get(testid('changes-file-big-note')).get('button').trigger('click')
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
    expect(wrapper.find(testid('changes-file-a.go')).exists()).toBe(true)
    wrapper.unmount()
  })

  it('leaves keys alone while a modifier is held', async () => {
    const wrapper = await mountView()
    await press('v', { metaKey: true })
    expect(wrapper.get(testid('changes-progress')).text()).toBe('0/2 viewed')
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
