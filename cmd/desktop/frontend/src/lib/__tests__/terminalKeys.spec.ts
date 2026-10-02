import { describe, expect, it } from 'vitest'
import { Terminal } from '@xterm/xterm'
import { claimsShiftEnter } from '../terminalKeys'

// The bytes Pi reads for Shift+Enter (#538): its tui.input.newLine binding is
// the CSI u modified-enter, and a plain CR is Enter.
const SHIFT_ENTER = '\x1b[13;2u'

function harness(): { term: Terminal; sent: () => string } {
  const term = new Terminal({ cols: 20, rows: 5 })
  let sent = ''
  term.onData((data) => {
    sent += data
  })
  return { term, sent: () => sent }
}

function keyEvent(type: 'keydown' | 'keypress' | 'keyup', init: KeyboardEventInit = {}): KeyboardEvent {
  return new KeyboardEvent(type, { key: 'Enter', cancelable: true, ...init })
}

describe('claimsShiftEnter', () => {
  it('writes the CSI u modified-enter for Shift+Enter and prevents the keydown', () => {
    const { term, sent } = harness()
    const event = keyEvent('keydown', { shiftKey: true })

    expect(claimsShiftEnter(term, event)).toBe(true)
    expect(sent()).toBe(SHIFT_ENTER)
    expect(event.defaultPrevented).toBe(true)
  })

  it('declines the keypress of the same chord without writing it twice', () => {
    const { term, sent } = harness()
    claimsShiftEnter(term, keyEvent('keydown', { shiftKey: true }))

    expect(claimsShiftEnter(term, keyEvent('keypress', { shiftKey: true }))).toBe(true)
    expect(sent()).toBe(SHIFT_ENTER)
  })

  it('leaves the keyup to xterm', () => {
    const { term, sent } = harness()

    expect(claimsShiftEnter(term, keyEvent('keyup', { shiftKey: true }))).toBe(false)
    expect(sent()).toBe('')
  })

  it('leaves a bare Enter to xterm, which writes the CR itself', () => {
    const { term, sent } = harness()
    const event = keyEvent('keydown')

    expect(claimsShiftEnter(term, event)).toBe(false)
    expect(sent()).toBe('')
    expect(event.defaultPrevented).toBe(false)
  })

  // ⌘⇧↩ is the pane zoom chord; the rest are xterm's to encode as it does today.
  it.each([
    ['Shift+Ctrl', { shiftKey: true, ctrlKey: true }],
    ['Shift+Alt', { shiftKey: true, altKey: true }],
    ['Shift+Meta', { shiftKey: true, metaKey: true }],
  ])('leaves %s+Enter to xterm', (_name, init) => {
    const { term, sent } = harness()

    expect(claimsShiftEnter(term, keyEvent('keydown', init))).toBe(false)
    expect(sent()).toBe('')
  })

  it('leaves Shift on any other key to xterm', () => {
    const { term, sent } = harness()

    expect(claimsShiftEnter(term, keyEvent('keydown', { key: 'A', shiftKey: true }))).toBe(false)
    expect(sent()).toBe('')
  })

  it('leaves Shift+Enter inside an IME composition to xterm', () => {
    const { term, sent } = harness()

    expect(claimsShiftEnter(term, keyEvent('keydown', { shiftKey: true, isComposing: true }))).toBe(false)
    expect(sent()).toBe('')
  })
})
