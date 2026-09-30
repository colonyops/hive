import { ref, type Ref } from 'vue'
import type { PaneDirection, SplitDirection } from './terminalClient'

// The seam between the global keymap and terminal mode's session tree.
//
// The `terminal.*` commands are dispatched by App.vue like every other bindable
// command, but they act on state that lives inside TerminalMode — the tree's
// DOM rows, the attached session's window list, its xterm. This module is where
// the two meet: the sidebar publishes its handles, and the dispatcher calls
// whatever is published.
//
// A plain module registry rather than a composable: it holds no lifecycle of
// its own, so the sidebar sets and clears it from its own mount hooks instead
// of inheriting an effect scope from whoever happens to call in.

export interface TerminalTreeHandles {
  /** Move DOM focus to the tree, on the cursor row. */
  focusTree(): void
  /** Move DOM focus to the attached session's active pane. */
  focusPane(): void
  /** Move DOM focus to the sidebar's filter field, selecting what is in it. */
  focusFilter(): void
  /**
   * Select the `position`-th window of the attached session, 1-based, and go to
   * work in it. A session with fewer windows ignores the call.
   */
  selectWindow(position: number): void
  /** Open a window in the attached session and go to work in it. */
  newWindow(): void
  /** Close the attached session's active window. */
  closeWindow(): void
  /**
   * Go to the window `delta` places along the list from the active one,
   * wrapping at either end.
   */
  stepWindow(delta: number): void
  /** Split the active window's active pane; horizontal puts the new pane to the right, vertical below. */
  splitPane(direction: SplitDirection): void
  /** Close the active window's active pane, asking first when it is running something. */
  closePane(): void
  zoomPane(): void
  focusPaneDirection(direction: PaneDirection): void
}

let handles: TerminalTreeHandles | null = null

/** Whether focus is currently inside the session tree. Drives the hint bar. */
export const terminalTreeFocused: Ref<boolean> = ref(false)

// Whether a pane may take focus on its own when a session or window becomes
// active — the mouse's behaviour, and the wrong one for the arrows: a pane that
// grabs focus mid-walk sends the next arrow to tmux.
//
// It records the last input to move the selection rather than bracketing one
// activation, because the calls it gates are spread across an attach that may
// not paint for a second. Explicit requests to enter a pane (Enter, the focus
// chord, the scroll pill) call focusActive() directly and do not consult it.
export const paneMayAutoFocus: Ref<boolean> = ref(true)

export function setTerminalTreeHandles(next: TerminalTreeHandles | null): void {
  handles = next
}

export function focusTerminalTree(): void {
  handles?.focusTree()
}

export function focusTerminalPane(): void {
  handles?.focusPane()
}

export function focusTerminalFilter(): void {
  handles?.focusFilter()
}

export function selectTerminalWindow(position: number): void {
  handles?.selectWindow(position)
}

export function newTerminalWindow(): void {
  handles?.newWindow()
}

export function closeTerminalWindow(): void {
  handles?.closeWindow()
}

export function stepTerminalWindow(delta: number): void {
  handles?.stepWindow(delta)
}

export function splitTerminalPane(direction: SplitDirection): void {
  handles?.splitPane(direction)
}

export function closeTerminalPane(): void {
  handles?.closePane()
}

export function zoomTerminalPane(): void {
  handles?.zoomPane()
}

export function focusTerminalPaneDirection(direction: PaneDirection): void {
  handles?.focusPaneDirection(direction)
}
