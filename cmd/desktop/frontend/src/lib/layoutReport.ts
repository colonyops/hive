import { isPanelLayout, type PanelLayout, type PanelNode, type PanelTab } from './panelLayout'
import { windowIdOf } from './windowTabs'

export interface PanelTabWire {
  id: string
  kind: string
  windowId: string
}

export interface PanelNodeWire {
  kind: string
  id: string
  tabs: PanelTabWire[]
  activeTab: string
  picking: boolean
  axis: string
  ratio: number
  children: PanelNodeWire[]
}

export interface PanelLayoutWire {
  root: PanelNodeWire
  focusedPanel: string
}

/** What Go reads to answer an agent about the layouts the user sees. */
export interface LayoutReport {
  kind: string
  onScreen: string
  layouts: Record<string, PanelLayoutWire>
}

function tabWire(tab: PanelTab): PanelTabWire {
  return { id: tab.id, kind: tab.kind, windowId: windowIdOf(tab) ?? '' }
}

function nodeWire(node: PanelNode): PanelNodeWire {
  if (node.kind === 'panel') {
    return {
      kind: node.kind,
      id: node.id,
      tabs: node.tabs.map(tabWire),
      activeTab: node.activeTab ?? '',
      picking: node.picking,
      axis: '',
      ratio: 0,
      children: [],
    }
  }
  return {
    kind: node.kind,
    id: node.id,
    tabs: [],
    activeTab: '',
    picking: false,
    axis: node.axis,
    ratio: node.ratio,
    children: [nodeWire(node.first), nodeWire(node.second)],
  }
}

/** `onScreen` is the layout key the user is looking at, or '' when none is. */
export function toLayoutReport(
  kind: 'code' | 'chats',
  onScreen: string,
  layouts: Record<string, PanelLayout>,
): LayoutReport {
  return {
    kind,
    onScreen,
    layouts: Object.fromEntries(
      Object.entries(layouts)
        .filter(([, layout]) => isPanelLayout(layout))
        .map(([key, layout]) => [key, { root: nodeWire(layout.root), focusedPanel: layout.focusedPanel }]),
    ),
  }
}
