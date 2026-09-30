// The seam between the global keymap and the Agents area's two-level list,
// mirroring lib/terminalTree.ts. `agents.focus-sidebar` / `agents.focus-pane`
// are dispatched by App.vue like every other bindable command, but they act on
// DOM elements that live inside AgentsMode — the workspace list and the
// session pane. A plain module registry rather than a composable: it holds no
// lifecycle of its own, so the list publishes its handles from its own mount
// hooks instead of inheriting an effect scope from whoever happens to call in.

export interface AgentsTreeHandles {
  /** Move DOM focus to the workspace list. */
  focusList(): void
  /** Move DOM focus to the list's filter field. */
  focusFilter(): void
  /** Move DOM focus to the open session's pane. */
  focusPane(): void
}

let handles: AgentsTreeHandles | null = null

export function setAgentsTreeHandles(next: AgentsTreeHandles | null): void {
  handles = next
}

export function focusAgentsList(): void {
  handles?.focusList()
}

export function focusAgentsFilter(): void {
  handles?.focusFilter()
}

export function focusAgentsPane(): void {
  handles?.focusPane()
}
