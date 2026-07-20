import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

// The effective keymap is a module singleton whose overrides load from
// localStorage (via VueUse useStorage) at import time, so — like useTheme —
// each test clears storage, resets modules, and imports fresh.
beforeEach(() => {
  localStorage.clear()
  vi.resetModules()
})

function ev(init: Partial<KeyboardEvent>): KeyboardEvent {
  return {
    isComposing: false,
    keyCode: 0,
    key: '',
    metaKey: false,
    ctrlKey: false,
    altKey: false,
    shiftKey: false,
    ...init,
  } as KeyboardEvent
}

describe('comboFromEvent', () => {
  it('collapses Meta and Ctrl to a single mod token', async () => {
    const { comboFromEvent } = await import('../useKeybindings')
    expect(comboFromEvent(ev({ key: 'k', metaKey: true }))).toBe('mod+k')
    expect(comboFromEvent(ev({ key: 'k', ctrlKey: true }))).toBe('mod+k')
  })

  it('lowercases letters and records Shift as an explicit token', async () => {
    const { comboFromEvent } = await import('../useKeybindings')
    expect(comboFromEvent(ev({ key: 'G', shiftKey: true }))).toBe('shift+g')
    expect(comboFromEvent(ev({ key: 'j' }))).toBe('j')
  })

  it('does not double-count Shift for symbols whose character already changed', async () => {
    const { comboFromEvent } = await import('../useKeybindings')
    // Shift+/ produces '?'; the character encodes the shift already.
    expect(comboFromEvent(ev({ key: '?', shiftKey: true }))).toBe('?')
  })

  it('names arrow, space, and enter keys', async () => {
    const { comboFromEvent } = await import('../useKeybindings')
    expect(comboFromEvent(ev({ key: 'ArrowDown' }))).toBe('arrowdown')
    expect(comboFromEvent(ev({ key: 'ArrowUp' }))).toBe('arrowup')
    expect(comboFromEvent(ev({ key: ' ' }))).toBe('space')
    expect(comboFromEvent(ev({ key: 'Enter' }))).toBe('enter')
  })

  it('orders modifiers canonically regardless of which are held', async () => {
    const { comboFromEvent } = await import('../useKeybindings')
    expect(comboFromEvent(ev({ key: 'K', metaKey: true, altKey: true, shiftKey: true }))).toBe('mod+alt+shift+k')
  })

  it('returns null for lone modifiers, IME composition, and unidentified keys', async () => {
    const { comboFromEvent } = await import('../useKeybindings')
    expect(comboFromEvent(ev({ key: 'Shift', shiftKey: true }))).toBeNull()
    expect(comboFromEvent(ev({ key: 'Meta', metaKey: true }))).toBeNull()
    expect(comboFromEvent(ev({ key: 'a', isComposing: true }))).toBeNull()
    expect(comboFromEvent(ev({ key: 'a', keyCode: 229 }))).toBeNull()
    expect(comboFromEvent(ev({ key: 'Unidentified' }))).toBeNull()
  })
})

describe('formatCombo', () => {
  it('renders mac symbols and non-mac words', async () => {
    const { formatCombo } = await import('../useKeybindings')
    expect(formatCombo('mod+k', true)).toBe('⌘K')
    expect(formatCombo('mod+k', false)).toBe('Ctrl+K')
    expect(formatCombo('shift+g', true)).toBe('⇧G')
    expect(formatCombo('shift+g', false)).toBe('Shift+G')
    expect(formatCombo('mod+shift+k', true)).toBe('⌘⇧K')
    expect(formatCombo('mod+shift+k', false)).toBe('Ctrl+Shift+K')
  })

  it('renders named keys with symbols and bare letters uppercased', async () => {
    const { formatCombo } = await import('../useKeybindings')
    expect(formatCombo('arrowdown', true)).toBe('↓')
    expect(formatCombo('j', true)).toBe('J')
    expect(formatCombo('enter', false)).toBe('↵')
  })
})

describe('canonicalizeCombo', () => {
  it('collapses and orders modifiers into the single spelling', async () => {
    const { canonicalizeCombo } = await import('../useKeybindings')
    expect(canonicalizeCombo('k+mod')).toBe('mod+k')
    expect(canonicalizeCombo('ctrl+k')).toBe('mod+k')
    expect(canonicalizeCombo('Meta+Shift+K')).toBe('mod+shift+k')
    expect(canonicalizeCombo('shift')).toBe('')
  })
})

describe('effective keymap', () => {
  it('resolves default combos to their command ids', async () => {
    const { useKeybindings } = await import('../useKeybindings')
    const kb = useKeybindings()
    expect(kb.bindings.value['feed.next']).toEqual(['j', 'arrowdown'])
    expect(kb.resolve('j')).toBe('feed.next')
    expect(kb.resolve('arrowdown')).toBe('feed.next')
    expect(kb.resolve('mod+k')).toBe('palette.toggle')
    expect(kb.resolve('nope')).toBeNull()
  })

  it('adds an override that shadows the default and persists it', async () => {
    const { useKeybindings } = await import('../useKeybindings')
    const kb = useKeybindings()
    kb.addBinding('feed.next', 'g')

    expect(kb.bindings.value['feed.next']).toEqual(['j', 'arrowdown', 'g'])
    expect(kb.resolve('g')).toBe('feed.next')
    expect(kb.isOverridden('feed.next')).toBe(true)
    await nextTick()
    expect(JSON.parse(localStorage.getItem('hive.keybindings') ?? '{}')).toMatchObject({
      'feed.next': ['j', 'arrowdown', 'g'],
    })
  })

  it('removes a binding, leaving an empty array as an explicit unbind', async () => {
    const { useKeybindings } = await import('../useKeybindings')
    const kb = useKeybindings()
    kb.removeBinding('feed.next', 'j')
    expect(kb.bindings.value['feed.next']).toEqual(['arrowdown'])
    expect(kb.resolve('j')).toBeNull()

    kb.removeBinding('feed.next', 'arrowdown')
    expect(kb.bindings.value['feed.next']).toEqual([])
    expect(kb.isOverridden('feed.next')).toBe(true)
  })

  it('restores catalog defaults on reset', async () => {
    const { useKeybindings } = await import('../useKeybindings')
    const kb = useKeybindings()
    kb.removeBinding('feed.next', 'j')
    kb.resetToDefault('feed.next')
    expect(kb.bindings.value['feed.next']).toEqual(['j', 'arrowdown'])
    expect(kb.isOverridden('feed.next')).toBe(false)
    expect(kb.resolve('j')).toBe('feed.next')
  })

  it('reports conflicts, honoring the excluded id', async () => {
    const { useKeybindings } = await import('../useKeybindings')
    const kb = useKeybindings()
    kb.addBinding('feed.refresh', 'j')
    expect(kb.conflicts('j').sort()).toEqual(['feed.next', 'feed.refresh'])
    expect(kb.conflicts('j', 'feed.refresh')).toEqual(['feed.next'])
  })
})

describe('override storage sanitization', () => {
  it('drops unknown ids, non-arrays, and non-string combos on load', async () => {
    localStorage.setItem('hive.keybindings', JSON.stringify({
      'feed.next': ['g'],
      'unknown.id': ['x'],
      'feed.prev': 'not-an-array',
      'feed.refresh': [123, 'r', 'r'],
    }))
    const { useKeybindings } = await import('../useKeybindings')
    const kb = useKeybindings()
    expect(kb.bindings.value['feed.next']).toEqual(['g'])
    expect(kb.bindings.value['feed.refresh']).toEqual(['r']) // 123 dropped, dupe collapsed
    expect(kb.bindings.value['feed.prev']).toEqual(['k', 'arrowup']) // invalid → default
    expect(kb.resolve('x')).toBeNull() // unknown id never bound
  })

  it('falls back to defaults for malformed or non-object storage', async () => {
    localStorage.setItem('hive.keybindings', 'not json')
    const { useKeybindings } = await import('../useKeybindings')
    expect(useKeybindings().bindings.value['feed.next']).toEqual(['j', 'arrowdown'])
  })
})
