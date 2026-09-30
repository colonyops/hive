import { describe, expect, it } from 'vitest'
import { commandCatalog, defaultCombosFor } from '../catalog'
import { comboFromEvent, terminalEscapeCombo } from '../../composables/useKeybindings'

// Where `mod` is Ctrl, Ctrl+Shift is the pane escape and terminalEscapeCombo
// drops the Shift before resolving, so a shifted default on an escapesPane
// command is unreachable from a pane there and lands on whichever command owns
// the unshifted spelling: Ctrl+Shift+W on `mod+shift+w` would close the window.
describe('commandCatalog defaults', () => {
  it('gives every escapesPane command unshifted defaults where mod is Ctrl', () => {
    for (const command of commandCatalog.filter((c) => c.escapesPane)) {
      for (const combo of defaultCombosFor(command, false)) {
        expect(combo.split('+'), `${command.id}: ${combo}`).not.toContain('shift')
      }
    }
  })

  it.each([
    ['=', false],
    ['+', true],
    ['+', false],
  ])('binds the %s spelling (shift: %s) of the increase chord on macOS', (key, shiftKey) => {
    const increase = commandCatalog.find((c) => c.id === 'terminal.text-size-increase')!
    const press = new KeyboardEvent('keydown', { key, shiftKey, metaKey: true })

    expect(defaultCombosFor(increase, true)).toContain(comboFromEvent(press))
  })

  it.each([
    ['terminal.text-size-increase', '+'],
    ['terminal.text-size-decrease', '_'],
    ['terminal.text-size-reset', ')'],
  ])('reaches %s from a pane where mod is Ctrl', (id, key) => {
    const command = commandCatalog.find((c) => c.id === id)!
    const press = new KeyboardEvent('keydown', { key, ctrlKey: true, shiftKey: true })

    expect(defaultCombosFor(command, false)).toContain(terminalEscapeCombo(press))
  })

  it('keeps item selection bindable without claiming a default chord', () => {
    const selection = commandCatalog.find((command) => command.id === 'feed.toggle-selection')

    expect(selection?.title).toBe('Toggle item selection')
    expect(selection?.defaultCombos).toEqual([])
    expect(selection?.context).toBe('feed')
  })

  it('applies the persisted sidebar command to any view with a sidebar', () => {
    const sidebar = commandCatalog.find((command) => command.id === 'terminal.toggle-sidebar')

    expect(sidebar?.title).toBe('Toggle sidebar')
    expect(sidebar?.group).toBe('View')
    expect(sidebar?.context).toBe('sidebar')
    expect(sidebar?.defaultCombos).toEqual(['mod+b'])
  })

  it.each([
    ['macOS', true],
    ['a Ctrl platform', false],
  ])('binds each default to one command on %s', (_, mac) => {
    const owners = new Map<string, string>()
    for (const command of commandCatalog) {
      for (const combo of defaultCombosFor(command, mac)) {
        expect(owners.get(combo), `${combo} already belongs to ${owners.get(combo)}`).toBeUndefined()
        owners.set(combo, command.id)
      }
    }
  })
})
