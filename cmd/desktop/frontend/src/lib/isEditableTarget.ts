export function isEditableTarget(target: EventTarget | null): boolean {
  return target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    target instanceof HTMLSelectElement ||
    (target instanceof HTMLElement && target.isContentEditable)
}

/**
 * True inside a live terminal. Unlike an editable target, which only claims
 * bare keys, a terminal claims every combination — Ctrl-b belongs to the pane,
 * not to a Hive shortcut.
 */
export function isTerminalTarget(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest('[data-terminal-input-scope]') !== null
}
