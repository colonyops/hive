import { ref } from 'vue'
import type { TerminalSessionRow } from '../../stores/useTerminalSessions'

// The separator is escaped, never typed: a literal NUL in the source makes the
// whole file read as binary, and every grep over it comes back empty.
export function windowMenuKey(row: TerminalSessionRow, windowId: string): string {
  return `${row.slug}\u0000${windowId}`
}

// The sidebar is a scroll container, so an overflowing menu is clipped rather
// than allowed to hang outside it: open upward near the bottom of the window.
function flipsUp(toggle: HTMLElement | null | undefined): boolean {
  const rect = toggle?.getBoundingClientRect()
  return rect != null && window.innerHeight - rect.bottom < 200 && rect.top > 200
}

function eventTarget(event: MouseEvent | undefined): HTMLElement | undefined {
  return event?.currentTarget instanceof HTMLElement ? event.currentTarget : undefined
}

/**
 * Which of the session tree's row menus is open: a session row's, a window
 * row's, or a row's new-window menu. At most one is open at a time. A
 * right-click (an event) always opens; a click on the toggle (no event) also
 * closes the menu it opened.
 */
export function useTreeRowMenus(options: { windowMenuAllowed: (row: TerminalSessionRow) => boolean }) {
  const openRow = ref('')
  const rowFlip = ref(false)
  const rowToggles = new Map<string, HTMLElement>()

  const openWindow = ref('')
  const windowFlip = ref(false)
  const windowToggles = new Map<string, HTMLElement>()

  const openNewWindow = ref('')
  const newWindowFlip = ref(false)
  const newWindowToggle = ref<HTMLElement | null>(null)

  // A toggle is an IconButton, so the ref is its instance; the menu wants its element.
  function setToggle(toggles: Map<string, HTMLElement>, key: string, toggle: unknown): void {
    const el = (toggle as { $el?: unknown } | null)?.$el
    if (el instanceof HTMLElement) toggles.set(key, el)
    else toggles.delete(key)
  }

  function toggleRow(row: TerminalSessionRow, event?: MouseEvent): void {
    openNewWindow.value = ''
    if (openRow.value === row.id && !event) {
      openRow.value = ''
      return
    }
    openWindow.value = ''
    rowFlip.value = flipsUp(eventTarget(event) ?? rowToggles.get(row.id))
    openRow.value = row.id
  }

  function toggleWindow(row: TerminalSessionRow, windowId: string, event?: MouseEvent): void {
    if (!options.windowMenuAllowed(row)) return
    const key = windowMenuKey(row, windowId)
    if (openWindow.value === key && !event) {
      openWindow.value = ''
      return
    }
    openRow.value = ''
    windowFlip.value = flipsUp(eventTarget(event) ?? windowToggles.get(key))
    openWindow.value = key
  }

  function toggleNewWindow(row: TerminalSessionRow, event: MouseEvent): void {
    if (openNewWindow.value === row.id) {
      openNewWindow.value = ''
      return
    }
    openRow.value = ''
    openWindow.value = ''
    newWindowToggle.value = eventTarget(event) ?? null
    newWindowFlip.value = flipsUp(newWindowToggle.value)
    openNewWindow.value = row.id
  }

  return {
    openRow,
    rowFlip,
    rowToggle: (id: string) => rowToggles.get(id) ?? null,
    setRowToggle: (id: string, el: unknown) => setToggle(rowToggles, id, el),
    toggleRow,
    openWindow,
    windowFlip,
    windowToggle: (key: string) => windowToggles.get(key) ?? null,
    setWindowToggle: (key: string, el: unknown) => setToggle(windowToggles, key, el),
    toggleWindow,
    openNewWindow,
    newWindowFlip,
    newWindowToggle,
    toggleNewWindow,
  }
}
