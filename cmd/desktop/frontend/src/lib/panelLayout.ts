export type SplitAxis = 'row' | 'column'

export const MIN_SPLIT_RATIO = 0.2
export const MAX_SPLIT_RATIO = 0.8

/**
 * What a tab shows. The layout places tabs and reads nothing else about them:
 * `kind` names the view, and `params` is that view's own state, carried so a
 * restart brings it back. A view that is open once at most uses its kind as the
 * tab id, so opening it again finds the tab it already has.
 */
export interface PanelTab {
  id: string
  kind: string
  params?: Readonly<Record<string, unknown>>
}

export interface Panel {
  kind: 'panel'
  id: string
  tabs: PanelTab[]
  activeTab: string | null
  /** An empty panel asking what goes in it. */
  picking: boolean
}

export interface PanelSplit {
  kind: 'split'
  id: string
  axis: SplitAxis
  /** The first child's share of the split. */
  ratio: number
  first: PanelNode
  second: PanelNode
}

export type PanelNode = Panel | PanelSplit

/**
 * A tree of splits with tabbed panels at its leaves, as in tmux. Every rule
 * here returns a new layout, or the same one when nothing changed, so a caller
 * can tell a no-op from a write.
 */
export interface PanelLayout {
  root: PanelNode
  focusedPanel: string
  nextId: number
}

/** A box as shares of the whole layout. */
export interface PanelBox {
  x: number
  y: number
  width: number
  height: number
}

export function newPanelLayout(): PanelLayout {
  return {
    root: { kind: 'panel', id: 'p0', tabs: [], activeTab: null, picking: false },
    focusedPanel: 'p0',
    nextId: 1,
  }
}

function isTab(value: unknown): value is PanelTab {
  const tab = value as { id?: unknown; kind?: unknown } | null
  return typeof tab?.id === 'string' && typeof tab.kind === 'string'
}

function isNode(value: unknown): value is PanelNode {
  const node = value as {
    kind?: unknown
    id?: unknown
    tabs?: unknown
    ratio?: unknown
    axis?: unknown
    first?: unknown
    second?: unknown
  } | null
  if (node?.kind === 'panel') return typeof node.id === 'string' && Array.isArray(node.tabs) && node.tabs.every(isTab)
  if (node?.kind !== 'split') return false
  return (
    typeof node.ratio === 'number' &&
    (node.axis === 'row' || node.axis === 'column') &&
    isNode(node.first) &&
    isNode(node.second)
  )
}

/** Storage can hold anything, so a layout that breaks the rules above starts over. */
export function isPanelLayout(value: unknown): value is PanelLayout {
  const layout = value as Partial<PanelLayout> | null | undefined
  return isNode(layout?.root) && typeof layout?.focusedPanel === 'string' && typeof layout?.nextId === 'number'
}

/** Every panel, first to last: left to right and top to bottom through the tree. */
export function panelsOf(layout: PanelLayout): Panel[] {
  const walk = (node: PanelNode): Panel[] =>
    node.kind === 'panel' ? [node] : [...walk(node.first), ...walk(node.second)]
  return walk(layout.root)
}

export function panelById(layout: PanelLayout, id: string): Panel | undefined {
  return panelsOf(layout).find((panel) => panel.id === id)
}

export function panelWithTab(layout: PanelLayout, tabId: string): Panel | undefined {
  return panelsOf(layout).find((panel) => panel.tabs.some((tab) => tab.id === tabId))
}

export function tabById(layout: PanelLayout, tabId: string): PanelTab | undefined {
  return panelWithTab(layout, tabId)?.tabs.find((tab) => tab.id === tabId)
}

/** Every tab in the order the panels list them, with where each one lives. */
export function orderedTabs(layout: PanelLayout): { panel: string; tab: PanelTab }[] {
  return panelsOf(layout).flatMap((panel) => panel.tabs.map((tab) => ({ panel: panel.id, tab })))
}

/** The tab a panel shows, or nothing while it is asking. */
export function shownTab(panel: Panel): PanelTab | undefined {
  return panel.picking ? undefined : panel.tabs.find((tab) => tab.id === panel.activeTab)
}

/** What each panel shows, by panel id. A panel showing nothing is left out. */
export function shownTabs(layout: PanelLayout): Map<string, PanelTab> {
  const shown = new Map<string, PanelTab>()
  for (const panel of panelsOf(layout)) {
    const tab = shownTab(panel)
    if (tab) shown.set(panel.id, tab)
  }
  return shown
}

/** The tab the focused panel shows, or '' when it shows nothing. */
export function focusedTab(layout: PanelLayout): string {
  const panel = panelById(layout, layout.focusedPanel)
  return panel ? (shownTab(panel)?.id ?? '') : ''
}

function firstPanel(node: PanelNode): Panel {
  return node.kind === 'panel' ? node : firstPanel(node.first)
}

function mapPanel(node: PanelNode, id: string, change: (panel: Panel) => PanelNode): PanelNode {
  if (node.kind === 'panel') return node.id === id ? change(node) : node
  const first = mapPanel(node.first, id, change)
  const second = mapPanel(node.second, id, change)
  return first === node.first && second === node.second ? node : { ...node, first, second }
}

function withPanel(layout: PanelLayout, id: string, change: Partial<Panel>): PanelLayout {
  return { ...layout, root: mapPanel(layout.root, id, (panel) => ({ ...panel, ...change })) }
}

export function focusPanel(layout: PanelLayout, id: string): PanelLayout {
  if (layout.focusedPanel === id || !panelById(layout, id)) return layout
  return { ...layout, focusedPanel: id }
}

export function selectTab(layout: PanelLayout, panelId: string, tabId: string): PanelLayout {
  if (!panelById(layout, panelId)?.tabs.some((tab) => tab.id === tabId)) return layout
  return { ...withPanel(layout, panelId, { activeTab: tabId, picking: false }), focusedPanel: panelId }
}

function appendTab(layout: PanelLayout, panelId: string, tab: PanelTab): PanelLayout {
  const panel = panelById(layout, panelId)!
  return selectTab(withPanel(layout, panelId, { tabs: [...panel.tabs, tab] }), panelId, tab.id)
}

/** Opens a tab in `target`, the focused panel by default. One already open anywhere is focused where it is. */
export function openTab(layout: PanelLayout, tab: PanelTab, target = layout.focusedPanel): PanelLayout {
  const existing = panelWithTab(layout, tab.id)
  if (existing) return selectTab(layout, existing.id, tab.id)
  return appendTab(layout, panelById(layout, target) ? target : firstPanel(layout.root).id, tab)
}

/**
 * For a tab nobody at the keyboard asked for, such as a document an agent opened.
 * It joins the last panel and takes no focus, so typing in a terminal carries on.
 */
export function addTabQuietly(layout: PanelLayout, tab: PanelTab): PanelLayout {
  if (panelWithTab(layout, tab.id)) return layout
  const panel = panelsOf(layout).at(-1)!
  return withPanel(layout, panel.id, { tabs: [...panel.tabs, tab], activeTab: panel.activeTab ?? tab.id })
}

/** The picker names a place, so a tab open in another panel moves here. */
export function placeTab(layout: PanelLayout, tab: PanelTab, target: string): PanelLayout {
  if (!panelById(layout, target)) return layout
  const from = panelWithTab(layout, tab.id)
  if (from?.id === target) return selectTab(layout, target, tab.id)
  if (from) return moveTab(layout, tab.id, target, panelById(layout, target)?.tabs.length ?? 0)
  return appendTab(layout, target, tab)
}

/** Replaces a tab's `params`. The same layout comes back for a tab that is not open. */
export function withTabParams(layout: PanelLayout, tabId: string, params: PanelTab['params']): PanelLayout {
  const panel = panelWithTab(layout, tabId)
  if (!panel) return layout
  return withPanel(layout, panel.id, { tabs: panel.tabs.map((tab) => (tab.id === tabId ? { ...tab, params } : tab)) })
}

function removeTab(layout: PanelLayout, panelId: string, tabId: string): PanelLayout {
  const panel = panelById(layout, panelId)!
  const at = panel.tabs.findIndex((tab) => tab.id === tabId)
  const tabs = panel.tabs.filter((tab) => tab.id !== tabId)
  const activeTab = panel.activeTab === tabId ? ((tabs[at] ?? tabs[at - 1])?.id ?? null) : panel.activeTab
  return withPanel(layout, panelId, { tabs, activeTab })
}

function removePanel(node: PanelNode, id: string): PanelNode | null {
  if (node.kind === 'panel') return node.id === id ? null : node
  const first = removePanel(node.first, id)
  const second = removePanel(node.second, id)
  if (!first) return second
  if (!second) return first
  return first === node.first && second === node.second ? node : { ...node, first, second }
}

/** The panel that takes over when `id` goes: the nearest one on the other side of its split. */
function neighbourOf(node: PanelNode, id: string): Panel | undefined {
  if (node.kind === 'panel') return undefined
  if (node.first.kind === 'panel' && node.first.id === id) return firstPanel(node.second)
  if (node.second.kind === 'panel' && node.second.id === id) return firstPanel(node.first)
  return neighbourOf(node.first, id) ?? neighbourOf(node.second, id)
}

/** A panel with nothing in it closes, wherever it is. The last panel stays. */
function dropEmptyPanels(layout: PanelLayout): PanelLayout {
  let next = layout
  for (const panel of panelsOf(layout)) {
    if (panel.tabs.length || panel.picking || panelsOf(next).length < 2) continue
    next = withoutPanel(next, panel.id)
  }
  return next
}

function withoutPanel(layout: PanelLayout, id: string): PanelLayout {
  const neighbour = neighbourOf(layout.root, id)
  const root = removePanel(layout.root, id)
  if (!root || !neighbour) return layout
  return { ...layout, root, focusedPanel: layout.focusedPanel === id ? neighbour.id : layout.focusedPanel }
}

/** Removes a tab. A panel left with nothing closes with it. */
export function closeTab(layout: PanelLayout, panelId: string, tabId: string): PanelLayout {
  if (!panelById(layout, panelId)?.tabs.some((tab) => tab.id === tabId)) return layout
  return dropEmptyPanels(removeTab(layout, panelId, tabId))
}

/** Splits a panel the way tmux does: it keeps its tabs and a new panel asks what goes beside it. */
export function splitPanel(layout: PanelLayout, panelId: string, axis: SplitAxis): PanelLayout {
  if (!panelById(layout, panelId)) return layout
  const added: Panel = { kind: 'panel', id: `p${layout.nextId}`, tabs: [], activeTab: null, picking: true }
  const root = mapPanel(layout.root, panelId, (panel) => ({
    kind: 'split',
    id: `s${layout.nextId}`,
    axis,
    ratio: 0.5,
    first: panel,
    second: added,
  }))
  return { ...layout, root, nextId: layout.nextId + 1, focusedPanel: added.id }
}

/**
 * For a tab nobody at the keyboard asked for that should still be seen at once.
 * It opens in a new panel beside the focused one, and focus stays where it was.
 */
export function addTabBeside(layout: PanelLayout, tab: PanelTab): PanelLayout {
  if (panelWithTab(layout, tab.id)) return layout
  const focused = panelById(layout, layout.focusedPanel) ?? firstPanel(layout.root)
  const split = splitPanel(layout, focused.id, 'row')
  const added = `p${layout.nextId}`
  return { ...withPanel(split, added, { tabs: [tab], activeTab: tab.id, picking: false }), focusedPanel: focused.id }
}

/**
 * Like `addTabBeside`, for a tab that may be open already. One open in a panel
 * other than the focused one is beside something, so it stays. One in the
 * focused panel moves out into the new split, unless it is that panel's only
 * tab, which would leave nothing to be beside.
 */
export function placeTabBeside(layout: PanelLayout, tab: PanelTab): PanelLayout {
  const from = panelWithTab(layout, tab.id)
  if (from && (from.id !== layout.focusedPanel || from.tabs.length === 1)) return layout
  const focused = panelById(layout, layout.focusedPanel) ?? firstPanel(layout.root)
  const split = splitPanel(layout, focused.id, 'row')
  const added = `p${layout.nextId}`
  const cleared = from ? removeTab(split, from.id, tab.id) : split
  return { ...withPanel(cleared, added, { tabs: [tab], activeTab: tab.id, picking: false }), focusedPanel: focused.id }
}

/** Closes a panel without losing anything: its tabs join the neighbour. */
export function closePanel(layout: PanelLayout, panelId: string): PanelLayout {
  const panel = panelById(layout, panelId)
  if (!panel) return layout
  const neighbour = neighbourOf(layout.root, panelId)
  if (!neighbour) return withPanel(layout, panelId, { picking: false })
  const merged = withPanel(withoutPanel(layout, panelId), neighbour.id, {
    tabs: [...neighbour.tabs, ...panel.tabs],
    activeTab: neighbour.activeTab ?? panel.activeTab,
    picking: neighbour.picking && !panel.tabs.length,
  })
  return { ...merged, focusedPanel: neighbour.id }
}

function mapSplit(node: PanelNode, id: string, change: (split: PanelSplit) => PanelSplit): PanelNode {
  if (node.kind === 'panel') return node
  if (node.id === id) return change(node)
  const first = mapSplit(node.first, id, change)
  const second = mapSplit(node.second, id, change)
  return first === node.first && second === node.second ? node : { ...node, first, second }
}

export function resizeSplit(layout: PanelLayout, splitId: string, ratio: number): PanelLayout {
  const clamped = Math.min(MAX_SPLIT_RATIO, Math.max(MIN_SPLIT_RATIO, ratio))
  const root = mapSplit(layout.root, splitId, (split) =>
    split.ratio === clamped ? split : { ...split, ratio: clamped },
  )
  return root === layout.root ? layout : { ...layout, root }
}

/** Turns one split between side by side and stacked, with its contents in place. */
export function turnSplit(layout: PanelLayout, splitId: string): PanelLayout {
  const root = mapSplit(layout.root, splitId, (split) => ({
    ...split,
    axis: split.axis === 'row' ? 'column' : 'row',
  }))
  return root === layout.root ? layout : { ...layout, root }
}

/** `gap` counts tabs in the target panel as they stand before the move. */
export function moveTab(layout: PanelLayout, tabId: string, target: string, gap: number): PanelLayout {
  const from = panelWithTab(layout, tabId)
  const destination = panelById(layout, target)
  if (!from || !destination) return layout
  const tab = from.tabs.find((candidate) => candidate.id === tabId)!
  const position = from.id === target ? destination.tabs.indexOf(tab) : -1
  const adjusted = position >= 0 && position < gap ? gap - 1 : gap
  const cleared = removeTab(layout, from.id, tabId)
  const remaining = panelById(cleared, target)!.tabs
  const at = Math.min(remaining.length, Math.max(0, adjusted))
  const placed = withPanel(cleared, target, { tabs: [...remaining.slice(0, at), tab, ...remaining.slice(at)] })
  return dropEmptyPanels(selectTab(placed, target, tabId))
}

/** A drop beside another tab, as a list of rows names it. */
export function moveTabBeside(
  layout: PanelLayout,
  tabId: string,
  besideId: string,
  edge: 'before' | 'after',
): PanelLayout {
  const panel = panelWithTab(layout, besideId)
  if (!panel) return layout
  const at = panel.tabs.findIndex((tab) => tab.id === besideId)
  return moveTab(layout, tabId, panel.id, edge === 'after' ? at + 1 : at)
}

/**
 * Makes the tabs of one kind mirror a set of ids: a tab whose id is gone goes,
 * and every id without a tab gets one in `target`, the focused panel by default.
 * The same layout comes back when nothing changed, so a poll cannot cost a
 * storage write. A panel with nothing active takes `active` when it holds it,
 * and its first tab otherwise.
 */
export function syncTabs(
  layout: PanelLayout,
  kind: string,
  ids: readonly string[],
  make: (id: string) => PanelTab,
  options: { active?: string; target?: string } = {},
): PanelLayout {
  const wanted = ids.map(make)
  const keep = new Set(wanted.map((tab) => tab.id))
  let next = layout
  for (const panel of panelsOf(layout)) {
    for (const tab of panel.tabs) {
      if (tab.kind === kind && !keep.has(tab.id)) next = removeTab(next, panel.id, tab.id)
    }
  }
  const missing = wanted.filter((tab) => !panelWithTab(next, tab.id))
  if (missing.length) {
    const target = panelById(next, options.target ?? next.focusedPanel) ?? firstPanel(next.root)
    // Only a tab this panel asked for answers its picker. One that arrived on
    // its own joins behind it, and the question stays on screen.
    const picking = target.picking && target.id !== options.target
    next = withPanel(next, target.id, { tabs: [...target.tabs, ...missing], picking })
  }
  for (const panel of panelsOf(next)) {
    if (panel.activeTab || !panel.tabs.length) continue
    const active = options.active && panel.tabs.some((tab) => tab.id === options.active) ? options.active : null
    next = withPanel(next, panel.id, { activeTab: active ?? panel.tabs[0].id })
  }
  return next === layout ? layout : dropEmptyPanels(next)
}

const WHOLE: PanelBox = { x: 0, y: 0, width: 1, height: 1 }

function childBoxes(split: PanelSplit, box: PanelBox): [PanelBox, PanelBox] {
  if (split.axis === 'row') {
    const width = box.width * split.ratio
    return [
      { ...box, width },
      { ...box, x: box.x + width, width: box.width - width },
    ]
  }
  const height = box.height * split.ratio
  return [
    { ...box, height },
    { ...box, y: box.y + height, height: box.height - height },
  ]
}

/** Where each panel sits, as shares of the layout. */
export function panelBoxes(layout: PanelLayout): Map<string, PanelBox> {
  const boxes = new Map<string, PanelBox>()
  const walk = (node: PanelNode, box: PanelBox): void => {
    if (node.kind === 'panel') {
      boxes.set(node.id, box)
      return
    }
    const [first, second] = childBoxes(node, box)
    walk(node.first, first)
    walk(node.second, second)
  }
  walk(layout.root, WHOLE)
  return boxes
}

/** Each split with the box it divides, for placing and dragging its boundary. */
export function splitBoxes(layout: PanelLayout): { split: PanelSplit; box: PanelBox }[] {
  const found: { split: PanelSplit; box: PanelBox }[] = []
  const walk = (node: PanelNode, box: PanelBox): void => {
    if (node.kind === 'panel') return
    found.push({ split: node, box })
    const [first, second] = childBoxes(node, box)
    walk(node.first, first)
    walk(node.second, second)
  }
  walk(layout.root, WHOLE)
  return found
}
