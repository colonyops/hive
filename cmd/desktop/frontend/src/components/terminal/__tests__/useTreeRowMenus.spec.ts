import { describe, expect, it } from 'vitest'
import type { TerminalSessionRow } from '../../../stores/useTerminalSessions'
import { useTreeRowMenus, windowMenuKey } from '../useTreeRowMenus'

const row: TerminalSessionRow = {
  id: '1',
  name: 'parser',
  slug: 'parser',
  repo: 'r',
  state: 'active',
  canvasOwner: '',
  createdAt: '',
}
const scratch: TerminalSessionRow = {
  id: 's',
  name: 'scratch',
  slug: 'scratch',
  repo: '',
  state: 'active',
  canvasOwner: '',
  createdAt: '',
}

function setup() {
  return useTreeRowMenus({ windowMenuAllowed: (candidate) => candidate !== scratch })
}

function rightClick(): MouseEvent {
  return new MouseEvent('contextmenu')
}

describe('useTreeRowMenus', () => {
  it('toggles a row menu from its button, and a right-click always opens it', () => {
    const menus = setup()
    menus.toggleRow(row)
    expect(menus.openRow.value).toBe('1')
    menus.toggleRow(row)
    expect(menus.openRow.value).toBe('')
    menus.toggleRow(row, rightClick())
    menus.toggleRow(row, rightClick())
    expect(menus.openRow.value).toBe('1')
  })

  it('keeps at most one menu open', () => {
    const menus = setup()
    menus.toggleRow(row)
    menus.toggleWindow(row, '@1')
    expect(menus.openRow.value).toBe('')
    expect(menus.openWindow.value).toBe(windowMenuKey(row, '@1'))
    menus.toggleNewWindow(row, rightClick())
    expect(menus.openWindow.value).toBe('')
    expect(menus.openNewWindow.value).toBe('1')
    menus.toggleRow(row)
    expect(menus.openNewWindow.value).toBe('')
  })

  it('opens no window menu where no action applies', () => {
    const menus = setup()
    menus.toggleWindow(scratch, '@1', rightClick())
    expect(menus.openWindow.value).toBe('')
  })

  it('tracks the toggles it anchors to', () => {
    const menus = setup()
    const button = document.createElement('button')
    menus.setRowToggle('1', { $el: button })
    expect(menus.rowToggle('1')).toBe(button)
    menus.setRowToggle('1', null)
    expect(menus.rowToggle('1')).toBeNull()
  })
})
