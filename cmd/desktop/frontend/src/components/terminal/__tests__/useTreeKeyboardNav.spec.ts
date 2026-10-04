import { describe, expect, it, vi } from 'vitest'
import { computed, effectScope, ref } from 'vue'
import { paneMayAutoFocus } from '../../../lib/terminalTree'
import type { TerminalSessionGroup, TerminalSessionRow } from '../../../stores/useTerminalSessions'
import type { TreeWindowRow } from '../useTerminalTree'
import { useTreeKeyboardNav } from '../useTreeKeyboardNav'

const running = { id: '1', name: 'running', slug: 'running', repo: 'r', state: 'active', canvasOwner: '' }
const idle = { id: '2', name: 'idle', slug: 'idle', repo: 'r', state: 'active', canvasOwner: '' }

function win(windowId: string, active = false): TreeWindowRow {
  return { windowId, name: windowId, active, live: true, indicator: null }
}

function setup() {
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
    rowRunning: (row: TerminalSessionRow) => row.id === '1',
    renaming: () => false,
    selectSession: vi.fn(),
    startSession: vi.fn().mockResolvedValue(undefined),
    goToWindow: vi.fn().mockResolvedValue(undefined),
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
