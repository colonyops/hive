import type { Terminal } from '@xterm/xterm'
import { comboFromEvent, terminalEscapeCombo, useKeybindings } from '../composables/useKeybindings'
import { commandPiercesPane } from '../keybindings/catalog'

// The keymap is a module singleton with no lifecycle of its own, so it is read
// once here rather than per keystroke.
const keymap = useKeybindings()

// Enter is code point 13 and the modifier field is 1 plus the Shift bit: the
// kitty keyboard protocol's form for a modified key that has no legacy
// sequence, and the one Pi binds tui.input.newLine to.
const SHIFT_ENTER = '\x1b[13;2u'

/**
 * Writes Shift+Enter to the pane as a CSI u modified-enter and reports whether
 * it claimed the event, so a custom key handler can decline it to xterm. A
 * bare Enter, or one with any other modifier, is left to xterm untouched.
 *
 * xterm writes the same CR for Shift+Enter as for Enter, so the key has to be
 * claimed on keydown. Preventing that keydown's default keeps the browser from
 * firing a keypress and from putting a newline in xterm's hidden textarea; the
 * keypress is declined as well because xterm only skips its own keypress path,
 * which would still write the CR, when it handled the keydown itself.
 */
export function claimsShiftEnter(term: Terminal, event: KeyboardEvent): boolean {
  if (!isShiftEnter(event)) return false
  if (event.type === 'keydown') {
    event.preventDefault()
    term.input(SHIFT_ENTER)
    return true
  }
  return event.type === 'keypress'
}

function isShiftEnter(event: KeyboardEvent): boolean {
  if (event.key !== 'Enter' || !event.shiftKey || event.isComposing) return false
  return !event.ctrlKey && !event.altKey && !event.metaKey
}

/**
 * Whether App.vue's dispatcher takes this key over a focused pane, so a custom
 * key handler can decline it and xterm does not also write it: every key while
 * a sequence the pane started is pending, the escape chord when it is bound or
 * starts a binding, and a binding of a command that pierces the pane. Resolved
 * against the live keymap, so a rebind moves both sides together.
 */
export function appClaimsPaneKey(event: KeyboardEvent): boolean {
  if (keymap.paneSequencePending()) return true
  const escaped = terminalEscapeCombo(event)
  if (escaped && (keymap.resolve(escaped) || keymap.stepSequence(null, escaped).kind === 'extend')) return true
  // Ctrl+2 through Ctrl+7 are control characters on a platform without
  // Command, and an alt chord is a meta escape the shell would read as a
  // readline command.
  const id = keymap.resolve(comboFromEvent(event) ?? '')
  return !!id && commandPiercesPane(id)
}
