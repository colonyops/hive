import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref, type EffectScope } from 'vue'
import type { CommandContext } from '../../keybindings/catalog'

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  KeybindingSettings: vi.fn().mockResolvedValue({ overrides: {} }),
  SetKeybindingSettings: vi.fn().mockResolvedValue(undefined),
}))

// The keymap, the pending sequence, and the modal registry are module
// singletons, so each test imports them fresh.
type Modules = {
  useGlobalKeymap: typeof import('../useGlobalKeymap').useGlobalKeymap
  useKeybindings: typeof import('../../composables/useKeybindings').useKeybindings
  useRegisterOpenModal: typeof import('../../composables/useOpenModalCount').useRegisterOpenModal
  SEQUENCE_TIMEOUT_MS: number
}
let m: Modules
let scope: EffectScope
const runCommand = vi.fn<(id: string) => void>()
const activeContexts = new Set<CommandContext>()
const paletteOpen = ref(false)
const activityOpen = ref(false)
const tasksOpen = ref(false)

beforeEach(async () => {
  vi.resetModules()
  const keymap = await import('../useGlobalKeymap')
  const keybindings = await import('../../composables/useKeybindings')
  const modals = await import('../../composables/useOpenModalCount')
  m = {
    useGlobalKeymap: keymap.useGlobalKeymap,
    useKeybindings: keybindings.useKeybindings,
    useRegisterOpenModal: modals.useRegisterOpenModal,
    SEQUENCE_TIMEOUT_MS: keybindings.SEQUENCE_TIMEOUT_MS,
  }
  runCommand.mockReset()
  activeContexts.clear()
  activeContexts.add('global')
  paletteOpen.value = false
  activityOpen.value = false
  tasksOpen.value = false
  scope = effectScope()
  scope.run(() =>
    m.useGlobalKeymap({
      runCommand,
      contextActive: (context) => activeContexts.has(context),
      paletteOpen,
      activityOpen,
      tasksOpen,
    }),
  )
})

afterEach(() => {
  scope.stop()
  vi.useRealTimers()
  document.body.innerHTML = ''
})

function press(key: string, init: KeyboardEventInit = {}, target: EventTarget = window): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...init })
  target.dispatchEvent(event)
  return event
}

function openModal(): () => void {
  const modalScope = effectScope()
  modalScope.run(() => m.useRegisterOpenModal())
  return () => modalScope.stop()
}

const tasksChord = { metaKey: true, shiftKey: true }

describe('useGlobalKeymap dispatch', () => {
  it('runs a bound command and claims the keystroke', () => {
    const event = press('k', { metaKey: true })

    expect(runCommand).toHaveBeenCalledWith('palette.toggle')
    expect(event.defaultPrevented).toBe(true)
  })

  it('leaves a command alone outside its context', () => {
    const event = press('j')
    expect(runCommand).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)

    activeContexts.add('feed')
    press('j')
    expect(runCommand).toHaveBeenCalledWith('feed.next')
  })

  it('ignores a bare key typed into an editable field but not a modifier chord', () => {
    activeContexts.add('feed')
    const input = document.body.appendChild(document.createElement('input'))

    press('j', {}, input)
    expect(runCommand).not.toHaveBeenCalled()

    press('k', { metaKey: true }, input)
    expect(runCommand).toHaveBeenCalledWith('palette.toggle')
  })
})

describe('useGlobalKeymap overlay gate', () => {
  it.each([
    ['the palette', () => (paletteOpen.value = true)],
    ['Activity', () => (activityOpen.value = true)],
    ['Tasks', () => (tasksOpen.value = true)],
    ['a modal', () => void openModal()],
  ])('suppresses everything but the palette toggle while %s is open', (_name, open) => {
    activeContexts.add('feed')
    open()

    press('j')
    expect(runCommand).not.toHaveBeenCalled()

    press('k', { metaKey: true })
    expect(runCommand).toHaveBeenCalledWith('palette.toggle')
  })

  it('lets tasks.toggle close the Tasks overlay when nothing else is open', () => {
    tasksOpen.value = true
    press('t', tasksChord)
    expect(runCommand).toHaveBeenCalledWith('tasks.toggle')
  })

  it.each([
    ['the palette', () => (paletteOpen.value = true)],
    ['Activity', () => (activityOpen.value = true)],
    ['a modal stacked over it', () => void openModal()],
  ])('swallows tasks.toggle over Tasks while %s is also open', (_name, open) => {
    tasksOpen.value = true
    open()

    press('t', tasksChord)
    expect(runCommand).not.toHaveBeenCalled()
  })

  it('reopens the gate once the modal closes', () => {
    activeContexts.add('feed')
    const close = openModal()
    press('j')
    close()
    press('j')

    expect(runCommand).toHaveBeenCalledTimes(1)
    expect(runCommand).toHaveBeenCalledWith('feed.next')
  })
})

describe('useGlobalKeymap chord sequences', () => {
  it('dispatches once a two-step sequence completes', () => {
    const kb = m.useKeybindings()

    const first = press('g')
    expect(first.defaultPrevented).toBe(true)
    expect(kb.pendingSequence.value?.steps).toEqual(['g'])
    expect(runCommand).not.toHaveBeenCalled()

    press('t')
    expect(runCommand).toHaveBeenCalledWith('tasks.toggle')
    expect(kb.pendingSequence.value).toBeNull()
  })

  it('swallows an unmatched bare key mid-sequence', () => {
    activeContexts.add('feed')
    press('g')
    const stray = press('j')

    expect(stray.defaultPrevented).toBe(true)
    expect(runCommand).not.toHaveBeenCalled()
    expect(m.useKeybindings().pendingSequence.value).toBeNull()
  })

  it('does not start a sequence under an overlay or in an editable field', () => {
    const kb = m.useKeybindings()
    activityOpen.value = true
    press('g')
    expect(kb.pendingSequence.value).toBeNull()

    activityOpen.value = false
    const input = document.body.appendChild(document.createElement('input'))
    press('g', {}, input)
    expect(kb.pendingSequence.value).toBeNull()
  })

  it('clears a pending sequence when the palette opens or focus enters a field', async () => {
    const kb = m.useKeybindings()
    press('g')
    paletteOpen.value = true
    await nextTick()
    expect(kb.pendingSequence.value).toBeNull()

    paletteOpen.value = false
    await nextTick()
    press('g')
    const input = document.body.appendChild(document.createElement('input'))
    input.dispatchEvent(new FocusEvent('focusin', { bubbles: true }))
    expect(kb.pendingSequence.value).toBeNull()
  })

  it('fires a prefix that is also a complete binding after the timeout', () => {
    vi.useFakeTimers()
    const kb = m.useKeybindings()
    kb.addBinding('palette.toggle', 'q')
    kb.addBinding('tasks.toggle', 'q w')

    press('q')
    expect(runCommand).not.toHaveBeenCalled()

    vi.advanceTimersByTime(m.SEQUENCE_TIMEOUT_MS)
    expect(runCommand).toHaveBeenCalledWith('palette.toggle')
    expect(kb.pendingSequence.value).toBeNull()
  })

  it('re-applies the gate when the deferred command fires', () => {
    vi.useFakeTimers()
    const kb = m.useKeybindings()
    kb.addBinding('feed.next', 'q')
    kb.addBinding('tasks.toggle', 'q w')
    activeContexts.add('feed')

    press('q')
    activeContexts.delete('feed')
    vi.advanceTimersByTime(m.SEQUENCE_TIMEOUT_MS)

    expect(runCommand).not.toHaveBeenCalled()
  })

  it('cancels the deferred command when the sequence continues', () => {
    vi.useFakeTimers()
    const kb = m.useKeybindings()
    kb.addBinding('palette.toggle', 'q')
    kb.addBinding('tasks.toggle', 'q w')

    press('q')
    press('w')
    vi.advanceTimersByTime(m.SEQUENCE_TIMEOUT_MS)

    expect(runCommand).toHaveBeenCalledTimes(1)
    expect(runCommand).toHaveBeenCalledWith('tasks.toggle')
  })
})

describe('useGlobalKeymap over a focused terminal', () => {
  function terminal(): HTMLElement {
    const scopeEl = document.body.appendChild(document.createElement('div'))
    scopeEl.setAttribute('data-terminal-input-scope', '')
    return scopeEl.appendChild(document.createElement('textarea'))
  }

  it('hands the pane ordinary keys and clears a pending sequence', () => {
    activeContexts.add('feed')
    const kb = m.useKeybindings()
    press('g')

    const event = press('j', {}, terminal())

    expect(runCommand).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)
    expect(kb.pendingSequence.value).toBeNull()
  })

  it('lets a piercing command through, but not under an overlay', () => {
    const pane = terminal()
    press('t', tasksChord, pane)
    expect(runCommand).toHaveBeenCalledWith('tasks.toggle')

    runCommand.mockReset()
    activityOpen.value = true
    press('t', tasksChord, pane)
    expect(runCommand).not.toHaveBeenCalled()
  })
})
