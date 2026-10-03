import { computed, nextTick, ref, type Ref } from 'vue'
import { isEditableTarget } from '../../lib/isEditableTarget'
import { paneMayAutoFocus, terminalTreeFocused } from '../../lib/terminalTree'
import type { TerminalSessionGroup, TerminalSessionRow } from '../../stores/useTerminalSessions'
import type { TreeWindowRow } from './useTerminalTree'

export type TreeKeyboardNav = ReturnType<typeof useTreeKeyboardNav>

export const sessionKey = (row: TerminalSessionRow): string => `s:${row.id}`
export const windowKey = (row: TerminalSessionRow, windowId: string): string => `w:${row.id}:${windowId}`

/**
 * The session tree's arrow-key walk. An arrow is a click on the next row, not
 * a cursor that moves ahead of the selection, so there is one selected row
 * rather than a selection and a pending cursor to reconcile.
 *
 * Only rows a click does something to are walked: a repo header collapses on
 * click, so it stays mouse- and Tab-reachable. A running session is its
 * windows, so its own row drops out unless it has no windows to stand in for
 * it (window listing off). Rows are keyed rather than indexed, because the tree
 * re-sorts under a poll.
 */
export function useTreeKeyboardNav(options: {
  groups: Ref<TerminalSessionGroup[]>
  expanded: (group: TerminalSessionGroup) => boolean
  rows: Ref<TerminalSessionRow[]>
  attachedRow: Ref<TerminalSessionRow | null>
  activeSlug: Ref<string>
  windowRowsFor: (row: TerminalSessionRow) => TreeWindowRow[]
  rowRunning: (row: TerminalSessionRow) => boolean
  renaming: () => boolean
  selectSession: (slug: string) => void
  startSession: (slug: string) => Promise<void>
  goToWindow: (row: TerminalSessionRow, win: TreeWindowRow) => Promise<void>
}) {
  const root = ref<HTMLElement | null>(null)
  const filterInput = ref<{ select: () => void } | null>(null)

  const keys = computed<string[]>(() => {
    const keys: string[] = []
    for (const group of options.groups.value) {
      if (!options.expanded(group)) continue
      for (const row of group.sessions) {
        const windows = options.windowRowsFor(row)
        if (!options.rowRunning(row) || !windows.length) keys.push(sessionKey(row))
        for (const win of windows) keys.push(windowKey(row, win.windowId))
      }
    }
    return keys
  })

  const selectedKey = computed(() => {
    const row = options.attachedRow.value
    if (!row) return ''
    const active = options.windowRowsFor(row).find((win) => win.active)
    return active ? windowKey(row, active.windowId) : sessionKey(row)
  })

  // The row last landed on, by either input. It cannot be derived from the
  // selection: attaching a session also selects its active window, so a
  // position read back off the selection would sit one row below the session
  // row the arrow just picked, and the next arrow would skip that window.
  const cursorRequest = ref('')

  const cursorKey = computed(() => {
    if (keys.value.includes(cursorRequest.value)) return cursorRequest.value
    if (keys.value.includes(selectedKey.value)) return selectedKey.value
    return ''
  })

  // Nothing is selected on the way in, so the first row carries the tab stop.
  const tabStopKey = computed(() => cursorKey.value || keys.value[0] || '')

  function rowElement(key: string): HTMLElement | null {
    if (!key) return null
    const rows = root.value?.querySelectorAll<HTMLElement>('[data-tree-key]') ?? []
    return Array.from(rows).find((row) => row.dataset.treeKey === key) ?? null
  }

  // The click handlers, reached by key. Re-picking the attached session is where
  // the two part company: a click there means "put me back in the pane", and an
  // arrow that did the same would hand the next keystroke to tmux.
  async function activate(key: string): Promise<void> {
    const [kind, rowID, windowId] = key.split(':')
    const row = options.rows.value.find((candidate) => candidate.id === rowID)
    if (!row) return
    if (kind === 's') {
      if (row.slug !== options.activeSlug.value) options.selectSession(row.slug)
      return
    }
    const win = options.windowRowsFor(row).find((candidate) => candidate.windowId === windowId)
    if (win) await options.goToWindow(row, win)
  }

  async function move(delta: number): Promise<void> {
    const all = keys.value
    if (!all.length) return
    const at = all.indexOf(cursorKey.value)
    // Clamped, not wrapped: running off the end and landing at the top reads as a jump.
    const next = all[Math.min(all.length - 1, Math.max(0, at + delta))]
    if (!next || next === cursorKey.value) return
    cursorRequest.value = next
    // Walking past a session is not an intent to type in it; the row still
    // takes DOM focus so the ring and the tab stop follow the walk.
    paneMayAutoFocus.value = false
    rowElement(next)?.focus()
    await activate(next)
  }

  function onKeydown(event: KeyboardEvent): void {
    // Modified arrows belong to the keymap: Cmd+Left is how focus got here.
    if (event.metaKey || event.ctrlKey || event.altKey) return
    if (options.renaming() || isEditableTarget(event.target)) return
    const delta = { ArrowDown: 1, j: 1, ArrowUp: -1, k: -1 }[event.key]
    if (delta === undefined) return
    event.preventDefault()
    void move(delta)
  }

  function onFocusOut(event: FocusEvent): void {
    const next = event.relatedTarget
    if (next instanceof Node && root.value?.contains(next)) return
    terminalTreeFocused.value = false
  }

  // Nothing to land on while the list is still loading, so take the panel
  // itself and let the first arrow seed the cursor.
  function focusTree(): void {
    void nextTick(() => (rowElement(cursorKey.value) ?? root.value)?.focus())
  }

  // Selected, not just focused: `/` on a field with a query means a new search
  // far more often than an edit of the old one.
  function focusFilter(): void {
    void nextTick(() => filterInput.value?.select())
  }

  // The mouse's path, shared with Enter: both mean "go to work in this", so the
  // pane may take focus as it comes up.
  function openWindow(row: TerminalSessionRow, win: TreeWindowRow): void {
    cursorRequest.value = windowKey(row, win.windowId)
    paneMayAutoFocus.value = true
    void options.goToWindow(row, win)
  }

  function clickSession(row: TerminalSessionRow): void {
    cursorRequest.value = sessionKey(row)
    paneMayAutoFocus.value = true
    options.selectSession(row.slug)
  }

  // Enter on a row the walk only stops at because no terminal is behind it
  // starts one; a click still just opens it, next to the pane's Start button.
  function enterSession(row: TerminalSessionRow): void {
    cursorRequest.value = sessionKey(row)
    paneMayAutoFocus.value = true
    if (options.rowRunning(row)) options.selectSession(row.slug)
    else void options.startSession(row.slug)
  }

  return {
    root,
    filterInput,
    cursorRequest,
    tabStopKey,
    onKeydown,
    onFocusOut,
    focusTree,
    focusFilter,
    openWindow,
    clickSession,
    enterSession,
  }
}
