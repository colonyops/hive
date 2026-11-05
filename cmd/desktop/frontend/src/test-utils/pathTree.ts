import { flushPromises } from '@vue/test-utils'
import { vi } from 'vitest'

// @pierre/trees renders its rows in a shadow root under PathTree's host, and
// commits them in Preact effects after Vue's own tick.

export async function settleTree(): Promise<void> {
  await flushPromises()
  if (vi.isFakeTimers()) await vi.advanceTimersByTimeAsync(0)
  else await new Promise((resolve) => setTimeout(resolve, 0))
  await flushPromises()
}

export function treeShadow(scope: Element | Document = document): ShadowRoot {
  const host = scope.querySelector('.path-tree')
  if (!host?.shadowRoot) throw new Error('no mounted path tree in scope')
  return host.shadowRoot
}

export function treeRows(scope: Element | Document = document): HTMLElement[] {
  return [...treeShadow(scope).querySelectorAll<HTMLElement>('[role="treeitem"]')]
}

/** The visible rows' paths; a folder ends in a slash. */
export function treePaths(scope: Element | Document = document): string[] {
  return treeRows(scope).map((row) => row.dataset.itemPath ?? '')
}

export function treeRow(scope: Element | Document, path: string): HTMLElement {
  const row = treeRows(scope).find((candidate) => candidate.dataset.itemPath === path)
  if (!row) throw new Error(`no row ${path} in ${treePaths(scope).join(', ')}`)
  return row
}

export function treeKey(row: HTMLElement, key: string): void {
  row.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, composed: true, cancelable: true }))
}
