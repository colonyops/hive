import { describe, expect, it, vi } from 'vitest'
import { computed, effectScope, ref } from 'vue'
import { paneMayAutoFocus } from '../../../lib/terminalTree'
import type { TerminalSessionGroup, TerminalSessionRow } from '../../../stores/useTerminalSessions'
import type { TreeItem, TreeWindowRow } from '../useTerminalTree'
import { useTreeKeyboardNav } from '../useTreeKeyboardNav'

const running = {
  id: '1',
  name: 'running',
  slug: 'running',
  repo: 'r',
  server: '',
  repoIdentity: 'r',
  state: 'active',
  canvasOwner: '',
  tags: [],
  createdAt: '',
}
const idle = {
  id: '2',
  name: 'idle',
  slug: 'idle',
  repo: 'r',
  server: '',
  repoIdentity: 'r',
  state: 'active',
  canvasOwner: '',
  tags: [],
  createdAt: '',
}

function win(windowId: string, active = false): TreeWindowRow {
  return { windowId, name: windowId, active, live: true, indicator: null }
}

const windowItem = (window: TreeWindowRow): TreeItem => ({
  key: window.windowId,
  tabId: `window:${window.windowId}`,
  win: window,
  view: null,
})
const viewItem = (kind: string, panel = 'p0'): TreeItem => ({
  key: `view:${kind}`,
  tabId: kind,
  win: null,
  view: { panel, tab: { id: kind, kind } },
})

function setup(views: Record<string, TreeItem[]> = {}) {
  const windows: Record<string, TreeWindowRow[]> = { '1': [win('@1', true), win('@2')], '2': [win('@9')] }
  const rows = ref<TerminalSessionRow[]>([running, idle])
  const activeSlug = ref('running')
  const options = {
    groups: ref<TerminalSessionGroup[]>([{ key: 'r', name: 'r', kind: 'repo', sessions: [running, idle] }]),
    expanded: () => true,
    rows,
    attachedRow: computed(() => rows.value.find((row) => row.slug === activeSlug.value) ?? null),
    activeSlug,
    windowRowsFor: (row: TerminalSessionRow) => windows[row.id] ?? [],
    treeItemsFor: (row: TerminalSessionRow) => [...(windows[row.id] ?? []).map(windowItem), ...(views[row.id] ?? [])],
    rowRunning: (row: TerminalSessionRow) => row.id === '1',
    renaming: () => false,
    selectSession: vi.fn(),
    startSession: vi.fn().mockResolvedValue(undefined),
    goToWindow: vi.fn().mockResolvedValue(undefined),
    openView: vi.fn(),
  }
  const nav = effectScope().run(() => useTreeKeyboardNav(options))!
  return { nav, options }
}

function key(name: string, init: KeyboardEventInit = {}): KeyboardEvent {
  return new KeyboardEvent('keydown', { key: name, cancelable: true, ...init })
}

describe('useTreeKeyboardNav', () => {
  it("walks a running session's windows in place of its row, and an idle session's row", async () => {
    const { nav, options } = setup()
    expect(nav.tabStopKey.value).toBe('w:1:@1')

    nav.onKeydown(key('ArrowDown'))
    await Promise.resolve()
    expect(options.goToWindow).toHaveBeenLastCalledWith(running, win('@2'))
    expect(nav.tabStopKey.value).toBe('w:1:@2')

    nav.onKeydown(key('j'))
    await Promise.resolve()
    expect(nav.tabStopKey.value).toBe('s:2')
    expect(options.selectSession).toHaveBeenLastCalledWith('idle')
    expect(paneMayAutoFocus.value).toBe(false)
  })

  it('walks a view row between the windows and opens it on Enter', async () => {
    const { nav, options } = setup({ '1': [viewItem('diff', 'p1')] })
    nav.onKeydown(key('ArrowDown'))
    await Promise.resolve()
    nav.onKeydown(key('ArrowDown'))
    await Promise.resolve()

    expect(nav.tabStopKey.value).toBe('v:1:diff')
    expect(options.openView).toHaveBeenLastCalledWith(running, 'p1', 'diff')
    expect(options.selectSession).not.toHaveBeenCalled()

    nav.clickView(idle, 'p0', 'tasks')
    expect(options.openView).toHaveBeenLastCalledWith(idle, 'p0', 'tasks')
  })

  it("reaches a server's rows, whose ids carry a colon", async () => {
    const remote = { ...running, id: 'box:9', slug: 'box:remote', server: 'box' }
    const windows = [win('@4', true)]
    const options = {
      groups: ref<TerminalSessionGroup[]>([{ key: 'r', name: 'r', kind: 'repo', sessions: [remote, idle] }]),
      expanded: () => true,
      rows: ref<TerminalSessionRow[]>([remote, idle]),
      attachedRow: computed(() => null),
      activeSlug: ref(''),
      windowRowsFor: (row: TerminalSessionRow) => (row.id === 'box:9' ? windows : []),
      treeItemsFor: (row: TerminalSessionRow) => (row.id === 'box:9' ? windows.map(windowItem) : []),
      rowRunning: (row: TerminalSessionRow) => row.id === 'box:9',
      renaming: () => false,
      selectSession: vi.fn(),
      startSession: vi.fn().mockResolvedValue(undefined),
      goToWindow: vi.fn().mockResolvedValue(undefined),
      openView: vi.fn(),
    }
    const nav = effectScope().run(() => useTreeKeyboardNav(options))!

    nav.onKeydown(key('ArrowDown'))
    await Promise.resolve()

    expect(nav.tabStopKey.value).toBe('w:box:9:@4')
    expect(options.goToWindow).toHaveBeenCalledWith(remote, windows[0])
  })

  it('clamps at either end rather than wrapping', () => {
    const { nav, options } = setup()
    nav.onKeydown(key('ArrowUp'))
    expect(nav.tabStopKey.value).toBe('w:1:@1')
    expect(options.goToWindow).not.toHaveBeenCalled()
  })

  it('leaves modified arrows to the keymap', () => {
    const { nav } = setup()
    const event = key('ArrowDown', { metaKey: true })
    nav.onKeydown(event)
    expect(event.defaultPrevented).toBe(false)
    expect(nav.tabStopKey.value).toBe('w:1:@1')
  })

  it('starts a stopped session on Enter, and lets the pane take focus', () => {
    const { nav, options } = setup()
    paneMayAutoFocus.value = false
    nav.enterSession(idle)
    expect(options.startSession).toHaveBeenCalledWith('idle')
    expect(paneMayAutoFocus.value).toBe(true)
    nav.enterSession(running)
    expect(options.selectSession).toHaveBeenCalledWith('running')
  })

  it('moves DOM focus to the cursor row', async () => {
    const { nav } = setup()
    const root = document.createElement('div')
    root.innerHTML = '<div data-tree-key="w:1:@1" tabindex="0"></div>'
    document.body.append(root)
    nav.root.value = root
    nav.focusTree()
    await Promise.resolve()
    expect(document.activeElement?.getAttribute('data-tree-key')).toBe('w:1:@1')
    root.remove()
  })
})
