import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

// The durable overrides live in settings.yaml, read and written through
// SettingsService. The effective keymap is a module singleton hydrated from
// that read, so each test seeds the stubbed service, resets modules, and
// imports fresh.
const settings = vi.hoisted(() => ({ get: vi.fn(), set: vi.fn() }))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  KeybindingSettings: settings.get,
  SetKeybindingSettings: settings.set,
}))

/** Seeds what settings.yaml holds before useKeybindings hydrates. */
function seedStoredOverrides(overrides: unknown): void {
  settings.get.mockResolvedValue({ overrides })
}

/** The override map most recently written through to settings.yaml. */
function lastPersisted(): Record<string, string[]> | undefined {
  const calls = settings.set.mock.calls
  return calls.length ? (calls[calls.length - 1]![0] as { overrides: Record<string, string[]> }).overrides : undefined
}

beforeEach(() => {
  localStorage.clear()
  settings.get.mockReset()
  settings.set.mockReset()
  settings.get.mockResolvedValue({ overrides: {} })
  settings.set.mockResolvedValue(undefined)
  vi.resetModules()
})

function ev(init: Partial<KeyboardEvent>): KeyboardEvent {
  return {
    isComposing: false,
    keyCode: 0,
    key: '',
    code: '',
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

  // macOS composes with Option: Option+G emits `©`, so reading event.key would
  // produce a combo no user could think to write.
  it('takes an alt combo from the physical key, not the composed character', async () => {
    const { comboFromEvent } = await import('../useKeybindings')
    expect(comboFromEvent(ev({ key: '©', code: 'KeyG', altKey: true }))).toBe('alt+g')
    expect(comboFromEvent(ev({ key: 'Ω', code: 'KeyZ', altKey: true, metaKey: true }))).toBe('mod+alt+z')
    expect(comboFromEvent(ev({ key: '¡', code: 'Digit1', altKey: true }))).toBe('alt+1')
    expect(comboFromEvent(ev({ key: '÷', code: 'Slash', altKey: true }))).toBe('alt+/')
    expect(comboFromEvent(ev({ key: 'ArrowDown', code: 'ArrowDown', altKey: true }))).toBe('alt+arrowdown')
    // Option+E/U/I/N/` are dead keys on macOS: event.key is `Dead`, which the
    // key path drops outright.
    expect(comboFromEvent(ev({ key: 'Dead', code: 'KeyE', altKey: true }))).toBe('alt+e')
  })

  // A modifier held alone is still not a keystroke: its code is not one
  // baseFromCode spells, so the key path answers and rejects it.
  it('still ignores Option held on its own', async () => {
    const { comboFromEvent } = await import('../useKeybindings')
    expect(comboFromEvent(ev({ key: 'Alt', code: 'AltLeft', altKey: true }))).toBeNull()
    expect(comboFromEvent(ev({ key: 'Shift', code: 'ShiftLeft', altKey: true, shiftKey: true }))).toBeNull()
  })

  // Linux and Windows do not compose, so the character already is the key —
  // and both platforms must agree on the spelling for one settings.yaml to
  // mean the same thing on each.
  it('spells an alt combo the same way when the platform does not compose', async () => {
    const { comboFromEvent } = await import('../useKeybindings')
    expect(comboFromEvent(ev({ key: 'g', code: 'KeyG', altKey: true }))).toBe('alt+g')
  })

  // Shift still reads the character, which is what makes `?` a combo rather
  // than `shift+/`.
  it('leaves non-alt combos reading the produced character', async () => {
    const { comboFromEvent } = await import('../useKeybindings')
    expect(comboFromEvent(ev({ key: '?', code: 'Slash', shiftKey: true }))).toBe('?')
  })

  // A code this module has no spelling for must not swallow the keystroke.
  it('falls back to the character for an unmapped code', async () => {
    const { comboFromEvent } = await import('../useKeybindings')
    expect(comboFromEvent(ev({ key: 'f13', code: 'F13', altKey: true }))).toBe('alt+f13')
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

describe('terminalEscapeCombo', () => {
  // A pane owns every key it can use; only modifiers a terminal never wants
  // may claim one back.
  it('claims Cmd chords and leaves bare Ctrl to the pane', async () => {
    const { terminalEscapeCombo } = await import('../useKeybindings')
    expect(terminalEscapeCombo(ev({ key: 'k', metaKey: true }))).toBe('mod+k')
    // Ctrl+K is readline's kill-to-end-of-line.
    expect(terminalEscapeCombo(ev({ key: 'k', ctrlKey: true }))).toBeNull()
    // Bare keys and tmux prefixes are the pane's outright.
    expect(terminalEscapeCombo(ev({ key: 'j' }))).toBeNull()
    expect(terminalEscapeCombo(ev({ key: 'b', ctrlKey: true }))).toBeNull()
  })

  // Ctrl+Shift is how a terminal emulator spells an app chord on a platform
  // with no Cmd, so the Shift stands in for it and one configured `mod+k`
  // matches on both.
  it('drops the stand-in Shift from the Ctrl form', async () => {
    const { terminalEscapeCombo } = await import('../useKeybindings')
    expect(terminalEscapeCombo(ev({ key: 'k', ctrlKey: true, shiftKey: true }))).toBe('mod+k')
  })

  // ⌘⇧] and Ctrl+Shift+] are the same chord in two dialects, and Shift already
  // changed the character — so both land on `mod+}`, which is what lets the
  // tab-walk defaults be spelled once (keybindings/catalog).
  it('spells a shifted-punctuation chord the same way on both platforms', async () => {
    const { terminalEscapeCombo } = await import('../useKeybindings')
    expect(terminalEscapeCombo(ev({ key: '}', metaKey: true, shiftKey: true }))).toBe('mod+}')
    expect(terminalEscapeCombo(ev({ key: '}', ctrlKey: true, shiftKey: true }))).toBe('mod+}')
  })

  // Option composes on macOS, so an alt chord is not a reliable escape and is
  // left to the pane, which may want it as a meta prefix.
  it('does not claim alt chords', async () => {
    const { terminalEscapeCombo } = await import('../useKeybindings')
    expect(terminalEscapeCombo(ev({ key: '©', code: 'KeyG', altKey: true, metaKey: true }))).toBeNull()
  })
})

// terminalEscapeCombo drops Ctrl+Shift's stand-in Shift on Ctrl platforms, so
// pane-escaping commands need unshifted defaults. happy-dom's Darwin user agent
// lacks "Mac", so tests default to Ctrl unless overridden.
describe('pane chords per platform', () => {
  function fakePlatform(mac: boolean): void {
    Object.defineProperty(navigator, 'userAgent', {
      value: mac ? 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)' : 'Mozilla/5.0 (X11; Linux x86_64)',
      configurable: true,
    })
  }

  afterEach(() => {
    Reflect.deleteProperty(navigator, 'userAgent')
  })

  it('answers each Ctrl+Shift chord from a pane with one command where mod is Ctrl', async () => {
    fakePlatform(false)
    const { terminalEscapeCombo, useKeybindings } = await import('../useKeybindings')
    const kb = useKeybindings()
    const fromPane = (key: string) => kb.resolve(terminalEscapeCombo(ev({ key, ctrlKey: true, shiftKey: true })) ?? '')

    expect(fromPane('D')).toBe('terminal.split-right')
    expect(fromPane('W')).toBe('terminal.close-window')
    expect(fromPane('O')).toBe('terminal.split-down')
    expect(fromPane('Q')).toBe('terminal.close-pane')
    expect(fromPane('M')).toBe('terminal.zoom-pane')
    // Outside a pane the full combo resolves, so the macOS spellings must not
    // also be bound or one physical chord would do two things by focus.
    expect(kb.resolve('mod+shift+d')).toBeNull()
    expect(kb.resolve('mod+shift+w')).toBeNull()
    expect(kb.resolve('mod+shift+enter')).toBeNull()
  })

  it('keeps the Command chords on macOS', async () => {
    fakePlatform(true)
    const { terminalEscapeCombo, useKeybindings } = await import('../useKeybindings')
    const kb = useKeybindings()
    const fromPane = (init: Partial<KeyboardEvent>) =>
      kb.resolve(terminalEscapeCombo(ev({ metaKey: true, ...init })) ?? '')

    expect(fromPane({ key: 'd' })).toBe('terminal.split-right')
    expect(fromPane({ key: 'D', shiftKey: true })).toBe('terminal.split-down')
    expect(fromPane({ key: 'w' })).toBe('terminal.close-window')
    expect(fromPane({ key: 'W', shiftKey: true })).toBe('terminal.close-pane')
    expect(fromPane({ key: 'Enter', shiftKey: true })).toBe('terminal.zoom-pane')
    expect(kb.resolve('mod+o')).toBeNull()
    expect(kb.resolve('mod+q')).toBeNull()
    expect(kb.resolve('mod+m')).toBeNull()
  })

  it('seeds, lists and restores the platform default', async () => {
    fakePlatform(false)
    const { useKeybindings } = await import('../useKeybindings')
    const kb = useKeybindings()
    expect(kb.bindings.value['terminal.close-pane']).toEqual(['mod+q'])

    kb.addBinding('terminal.close-pane', 'mod+shift+w')
    kb.resetToDefault('terminal.close-pane')
    expect(kb.bindings.value['terminal.close-pane']).toEqual(['mod+q'])
    expect(kb.isOverridden('terminal.close-pane')).toBe(false)
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

  it('renders named keys with symbols and bare letters lowercase', async () => {
    const { formatCombo } = await import('../useKeybindings')
    expect(formatCombo('arrowdown', true)).toBe('↓')
    expect(formatCombo('j', true)).toBe('j')
    expect(formatCombo('enter', false)).toBe('↵')
  })

  it('renders a sequence step by step, space-joined', async () => {
    const { formatCombo } = await import('../useKeybindings')
    expect(formatCombo('g i', true)).toBe('g i')
    expect(formatCombo('mod+k mod+s', true)).toBe('⌘K ⌘S')
    expect(formatCombo('mod+k mod+s', false)).toBe('Ctrl+K Ctrl+S')
  })

  it("returns '' for an invalid binding", async () => {
    const { formatCombo } = await import('../useKeybindings')
    expect(formatCombo('shift')).toBe('')
    expect(formatCombo('g shift')).toBe('')
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

  // A key containing whitespace is a sequence, not a single step — the
  // spelling canonicalizeBinding relies on to reject a bad step.
  it('rejects a key containing whitespace', async () => {
    const { canonicalizeCombo } = await import('../useKeybindings')
    expect(canonicalizeCombo('mod+page up')).toBe('')
  })
})

describe('canonicalizeBinding', () => {
  it('canonicalizes each step and rejoins with a single space', async () => {
    const { canonicalizeBinding } = await import('../useKeybindings')
    expect(canonicalizeBinding('g i')).toBe('g i')
    expect(canonicalizeBinding('Meta+K')).toBe('mod+k')
    expect(canonicalizeBinding('ctrl+k   meta+shift+s')).toBe('mod+k mod+shift+s')
  })

  it("returns '' when any step is invalid, or the binding is empty", async () => {
    const { canonicalizeBinding } = await import('../useKeybindings')
    expect(canonicalizeBinding('g shift')).toBe('')
    expect(canonicalizeBinding('')).toBe('')
    expect(canonicalizeBinding('   ')).toBe('')
  })

  // Shift+/ produces '?' on a US layout; the binding string must keep that
  // spelling rather than reconstituting it as 'shift+/'.
  it('keeps the shifted `?` spelling rather than `shift+/`', async () => {
    const { canonicalizeBinding, formatCombo } = await import('../useKeybindings')
    expect(canonicalizeBinding('?')).toBe('?')
    expect(formatCombo('?')).toBe('?')
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
    expect(lastPersisted()).toMatchObject({ 'feed.next': ['j', 'arrowdown', 'g'] })
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

// These tests exercise the pure stepSequence mechanics in isolation, so they
// seed synthetic commands through setLauncherCommands — the same seam the
// 'launcher commands' tests below use to extend the catalog at runtime — on a
// leader ('z') the real catalog does not bind, so its own 'g …' sequences
// (view.go-inbox and friends) cannot collide with the fixture.
describe('sequences', () => {
  const BASE_COMMANDS = [
    { id: 'test.goto-inbox', title: 'Go to Inbox', group: 'Test', defaultCombos: ['z i'], context: 'global' as const },
    { id: 'test.goto-code', title: 'Go to Code', group: 'Test', defaultCombos: ['z c'], context: 'global' as const },
    // A three-step binding whose middle step carries the primary modifier, so
    // a matched continuation can be shown extending despite it.
    { id: 'test.deep', title: 'Deep', group: 'Test', defaultCombos: ['z mod+shift+x y'], context: 'global' as const },
  ]
  const BARE_LEADER_COMMAND = {
    id: 'test.bare-leader',
    title: 'Bare leader',
    group: 'Test',
    defaultCombos: ['z'],
    context: 'global' as const,
  }

  /** withBareLeader seeds a command bound to plain 'z' alongside the base fixture, for the deferred-command (Zed prefix) cases. */
  async function seedSequenceCommands({ withBareLeader = false } = {}) {
    const keybindings = await import('../useKeybindings')
    const { setLauncherCommands } = await import('../../keybindings/catalog')
    setLauncherCommands(withBareLeader ? [...BASE_COMMANDS, BARE_LEADER_COMMAND] : BASE_COMMANDS)
    return keybindings
  }

  it("does not resolve a sequence's first step on its own", async () => {
    const { useKeybindings } = await seedSequenceCommands()
    const kb = useKeybindings()
    expect(kb.resolve('z')).toBeNull()
    expect(kb.resolve('i')).toBeNull()
  })

  it('extends a sequence start that prefixes bindings, listing continuations', async () => {
    const { stepSequence } = await seedSequenceCommands()
    const transition = stepSequence(null, 'z')
    if (transition.kind !== 'extend') throw new Error(`expected extend, got ${transition.kind}`)
    expect(transition.pending.steps).toEqual(['z'])
    expect(transition.pending.continuations.sort((a, b) => a.step.localeCompare(b.step))).toEqual([
      { step: 'c', commandId: 'test.goto-code' },
      { step: 'i', commandId: 'test.goto-inbox' },
      { step: 'mod+shift+x', commandId: 'test.deep' },
    ])
    expect(transition.deferredCommandId).toBeNull()
  })

  it("sets deferredCommandId when the pending steps are also a complete binding (Zed's prefix rule)", async () => {
    const { stepSequence } = await seedSequenceCommands({ withBareLeader: true })
    const transition = stepSequence(null, 'z')
    if (transition.kind !== 'extend') throw new Error(`expected extend, got ${transition.kind}`)
    expect(transition.deferredCommandId).toBe('test.bare-leader')
  })

  it('passes a combo with no sequence involvement', async () => {
    const { stepSequence } = await seedSequenceCommands()
    expect(stepSequence(null, 'q')).toEqual({ kind: 'pass' })
  })

  it('runs when a pending sequence is completed by the next combo', async () => {
    const { stepSequence } = await seedSequenceCommands()
    const pending = { steps: ['z'], continuations: [] }
    expect(stepSequence(pending, 'i')).toEqual({ kind: 'run', commandId: 'test.goto-inbox' })
  })

  it('extends when the next combo matches a continuation, even carrying the primary modifier', async () => {
    const { stepSequence } = await seedSequenceCommands()
    const pending = { steps: ['z'], continuations: [] }
    const transition = stepSequence(pending, 'mod+shift+x')
    if (transition.kind !== 'extend') throw new Error(`expected extend, got ${transition.kind}`)
    expect(transition.pending.steps).toEqual(['z', 'mod+shift+x'])
    expect(transition.pending.continuations).toEqual([{ step: 'y', commandId: 'test.deep' }])
    expect(transition.deferredCommandId).toBeNull()
  })

  it('swallows an unmatched bare or modifier-less combo while a sequence is pending', async () => {
    const { stepSequence } = await seedSequenceCommands()
    const pending = { steps: ['z'], continuations: [] }
    expect(stepSequence(pending, 'q')).toEqual({ kind: 'swallow' })
    expect(stepSequence(pending, 'shift+q')).toEqual({ kind: 'swallow' })
  })

  it('passes an unmatched combo carrying the primary modifier, so it reaches normal dispatch', async () => {
    const { stepSequence } = await seedSequenceCommands()
    const pending = { steps: ['z'], continuations: [] }
    // mod+k is not a continuation of `z` — normal dispatch resolves it (e.g. to palette.toggle).
    expect(stepSequence(pending, 'mod+k')).toEqual({ kind: 'pass' })
  })

  it('does not treat a prefix relationship as a conflict, only an exact duplicate binding', async () => {
    const { useKeybindings } = await seedSequenceCommands({ withBareLeader: true })
    const kb = useKeybindings()
    // 'z' (test.bare-leader) prefixes 'z i' (test.goto-inbox) — functional per the
    // Zed rule, so excluding each binding's own owner leaves no conflict.
    expect(kb.conflicts('z', 'test.bare-leader')).toEqual([])
    expect(kb.conflicts('z i', 'test.goto-inbox')).toEqual([])

    kb.addBinding('feed.refresh', 'z i')
    expect(kb.conflicts('z i').sort()).toEqual(['feed.refresh', 'test.goto-inbox'])
  })

  it('sanitizes a stored sequence override, formalizing the round-trip', async () => {
    seedStoredOverrides({ 'feed.next': ['Meta+Shift+K g'], 'feed.prev': ['g shift'] })
    const { useKeybindings, initializeKeybindings } = await import('../useKeybindings')
    initializeKeybindings()
    await vi.waitFor(() => expect(useKeybindings().bindings.value['feed.next']).toEqual(['mod+shift+k g']))
    // 'shift' alone is not a valid step, so the whole binding is dropped.
    expect(useKeybindings().bindings.value['feed.prev']).toEqual([])
  })
})

describe('launcher commands', () => {
  // A launcher is a terminal-popup action, so it joins the catalog only once
  // actions.yml has been read — which is after settings.yaml has.
  it('binds a launcher registered after the overrides were read', async () => {
    seedStoredOverrides({ 'launcher.lazygit': ['alt+g'] })
    const { useKeybindings, initializeKeybindings } = await import('../useKeybindings')
    const { setLauncherCommands } = await import('../../keybindings/catalog')
    initializeKeybindings()
    await vi.waitFor(() => expect(settings.get).toHaveBeenCalled())

    const kb = useKeybindings()
    expect(kb.resolve('alt+g')).toBeNull()

    setLauncherCommands([
      { id: 'launcher.lazygit', title: 'lazygit', group: 'Quick terminals', defaultCombos: [], context: 'global' },
    ])
    expect(kb.resolve('alt+g')).toBe('launcher.lazygit')
  })

  // Persisting drops whatever the override map does not hold, so a launcher
  // binding erased at load would be erased from settings.yaml by the next
  // unrelated rebind.
  it('keeps a launcher override through a rebind made before its catalog loaded', async () => {
    seedStoredOverrides({ 'launcher.lazygit': ['alt+g'] })
    const { useKeybindings, initializeKeybindings } = await import('../useKeybindings')
    initializeKeybindings()
    await vi.waitFor(() => expect(settings.get).toHaveBeenCalled())

    useKeybindings().addBinding('feed.next', 'g')
    await vi.waitFor(() =>
      expect(lastPersisted()).toEqual({
        'launcher.lazygit': ['alt+g'],
        'feed.next': ['j', 'arrowdown', 'g'],
      }),
    )
  })

  // Catalog order is what resolves a shared combo, and launchers come last, so
  // a config file cannot take a built-in shortcut away from the app.
  it('does not let a launcher shadow a built-in combo', async () => {
    const { useKeybindings } = await import('../useKeybindings')
    const { setLauncherCommands } = await import('../../keybindings/catalog')
    setLauncherCommands([
      { id: 'launcher.thief', title: 'Thief', group: 'Quick terminals', defaultCombos: ['mod+k'], context: 'global' },
    ])
    expect(useKeybindings().resolve('mod+k')).toBe('palette.toggle')
  })
})

describe('override storage sanitization', () => {
  // settings.yaml is hand-editable, so anything can arrive here.
  it('drops non-arrays and non-string combos on load, and never binds an unknown id', async () => {
    seedStoredOverrides({
      'feed.next': ['g'],
      'unknown.id': ['x'],
      'feed.prev': 'not-an-array',
      'feed.refresh': [123, 'r', 'r'],
    })
    const { useKeybindings, initializeKeybindings } = await import('../useKeybindings')
    initializeKeybindings()
    await vi.waitFor(() => expect(useKeybindings().bindings.value['feed.next']).toEqual(['g']))

    const kb = useKeybindings()
    expect(kb.bindings.value['feed.refresh']).toEqual(['r']) // 123 dropped, dupe collapsed
    expect(kb.bindings.value['feed.prev']).toEqual(['k', 'arrowup']) // invalid → default
    // Kept in the override map — it may be a launcher whose catalog has not
    // arrived — but it resolves to nothing until something claims the id.
    expect(kb.resolve('x')).toBeNull()
  })

  it('falls back to defaults for a malformed or non-object section', async () => {
    seedStoredOverrides('not an object')
    const { useKeybindings, initializeKeybindings } = await import('../useKeybindings')
    initializeKeybindings()
    await nextTick()
    expect(useKeybindings().bindings.value['feed.next']).toEqual(['j', 'arrowdown'])
  })

  // An unreadable settings.yaml must leave the app usable rather than
  // shortcut-less.
  it('keeps catalog defaults when the settings read fails', async () => {
    settings.get.mockRejectedValue(new Error('unavailable'))
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { useKeybindings, initializeKeybindings } = await import('../useKeybindings')
    initializeKeybindings()
    await vi.waitFor(() => expect(warn).toHaveBeenCalled())
    expect(useKeybindings().bindings.value['feed.next']).toEqual(['j', 'arrowdown'])
    warn.mockRestore()
  })

  // Hydration happens after mount; a rebind made while that read is in flight
  // is newer than its result and must survive.
  it('does not let a late hydrate clobber a rebind made while it was in flight', async () => {
    let resolveRead: (value: { overrides: Record<string, string[]> }) => void = () => {}
    settings.get.mockReturnValue(
      new Promise((resolve) => {
        resolveRead = resolve
      }),
    )

    const { useKeybindings, initializeKeybindings } = await import('../useKeybindings')
    initializeKeybindings()

    const kb = useKeybindings()
    kb.addBinding('feed.next', 'g')
    resolveRead({ overrides: { 'feed.next': ['z'] } })
    await nextTick()

    expect(kb.bindings.value['feed.next']).toEqual(['j', 'arrowdown', 'g'])
  })

  it('clears the section entirely on reset-all', async () => {
    const { useKeybindings } = await import('../useKeybindings')
    const kb = useKeybindings()
    kb.addBinding('feed.next', 'g')
    kb.clearAll()

    expect(kb.bindings.value['feed.next']).toEqual(['j', 'arrowdown'])
    // Writes are chained so two quick edits cannot land out of order, so the
    // clear lands a tick behind the add.
    await vi.waitFor(() => expect(lastPersisted()).toEqual({}))
  })
})
