import type { UseResizablePanelOptions } from '@hive/ui/composables/useResizablePanel'

export interface ModeSidebar {
  panel: UseResizablePanelOptions
  collapsedKey: string
}

export const TERMINAL_SIDEBAR: ModeSidebar = {
  panel: { storageKey: 'hive.panel.terminal.sidebar', defaultSize: 250, min: 180, max: 400, edge: 'right' },
  collapsedKey: 'hive.panel.terminal.sidebar.collapsed',
}

export const AGENTS_SIDEBAR: ModeSidebar = {
  panel: { storageKey: 'hive.panel.agents.sidebar', defaultSize: 260, min: 180, max: 400, edge: 'right' },
  collapsedKey: 'hive.panel.agents.sidebar.collapsed',
}
