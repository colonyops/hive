import { nextTick } from 'vue'

/** The tab row's height. The row draws it, and the host places floating content under it from the same number. */
export const TAB_ROW_PX = 30

/**
 * Moves the keyboard onto a tab that a key brought to the front, as a click on it would. Left alone,
 * focus stays in the terminal the tab now hides, and keys go to that terminal.
 */
export function focusShownTab(tabId: string): void {
  void nextTick(() => {
    const focused = document.activeElement
    if (focused instanceof HTMLElement && focused.getClientRects().length > 0) return
    const tabButtons = document.querySelectorAll<HTMLElement>(
      `[data-panel-tab][data-tab-id="${CSS.escape(tabId)}"] [role="tab"]`,
    )
    ;[...tabButtons].find((button) => button.getClientRects().length > 0)?.focus()
  })
}
