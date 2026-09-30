import { ref, type Ref } from 'vue'

/** One tmux window of the attached session, as shown in the Code view's strip. */
export interface AttachedWindow {
  windowId: string
  name: string
  active: boolean
}

interface Attached {
  slug: string
  name: string
  windows: AttachedWindow[]
}

// The seam between TerminalMode's live window strip and the App-level palette
// rows that jump to a window from anywhere in the app (mirrors lib/terminalTree.ts's
// setTerminalTreeHandles: one writer, published as a module singleton so the
// reader does not have to be a descendant of the writer). Null until a session
// has actually been attached — correct, since there are then no windows to go
// to — and left alone rather than cleared when the mode goes off-screen: it
// names the attached session, not the visible one, so a window row must
// survive a trip back to the hub.
const attached: Ref<Attached | null> = ref(null)

export function useAttachedTerminalWindows(): {
  /** Written by TerminalMode as the attached session's tabs change. */
  attached: Ref<Attached | null>
} {
  return { attached }
}

/** Written by TerminalMode as the attached session's tabs change. */
export function setAttachedTerminalWindows(next: Attached | null): void {
  attached.value = next
}

export function resetAttachedTerminalWindowsForTests(): void {
  attached.value = null
}
