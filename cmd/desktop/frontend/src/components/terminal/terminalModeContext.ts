import { inject, provide, proxyRefs, type InjectionKey, type ShallowUnwrapRef } from 'vue'
import type { useSessionActions } from '../../composables/useSessionActions'
import type { SessionTreeView } from './useSessionTreeView'
import type { TerminalAttach } from './useTerminalAttach'
import type { TerminalPool } from './useTerminalPool'
import type { TerminalSessionOps } from './useTerminalSessionOps'
import type { TerminalTree } from './useTerminalTree'
import type { TreeKeyboardNav } from './useTreeKeyboardNav'
import type { WindowRename } from './useWindowRename'

/**
 * The Code view's per-instance state, built by TerminalMode and read by its
 * sidebar and pane column. Provided rather than passed as props because both
 * halves read most of it, and the state has to outlive a collapsed sidebar.
 */
export interface TerminalModeContext {
  pool: TerminalPool
  tree: TerminalTree
  view: SessionTreeView
  nav: TreeKeyboardNav
  attach: TerminalAttach
  ops: TerminalSessionOps
  rename: WindowRename
  sessions: ReturnType<typeof useSessionActions>
}

/** The context as the children see it: each part's refs unwrapped one level, as in a template. */
export type TerminalModeState = { [K in keyof TerminalModeContext]: ShallowUnwrapRef<TerminalModeContext[K]> }

const terminalModeKey: InjectionKey<TerminalModeState> = Symbol('terminalMode')

export function provideTerminalMode(context: TerminalModeContext): void {
  provide(
    terminalModeKey,
    Object.fromEntries(Object.entries(context).map(([part, refs]) => [part, proxyRefs(refs)])) as TerminalModeState,
  )
}

export function useTerminalModeContext(): TerminalModeState {
  const context = inject(terminalModeKey)
  if (!context) throw new Error('useTerminalModeContext needs a TerminalMode ancestor')
  return context
}
