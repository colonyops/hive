import type { DriveScope } from './drive'

export type SessionViewKind = 'docs' | 'diff' | 'tasks'

export type SplitAxis = 'row' | 'column'

export const MIN_SPLIT_RATIO = 0.2
export const MAX_SPLIT_RATIO = 0.8

export type SessionTab = { id: string; kind: 'window'; windowId: string } | { id: string; kind: SessionViewKind }

export interface SessionPane {
  kind: 'pane'
  id: string
  tabs: SessionTab[]
  activeTab: string | null
  /** An empty pane asking what goes in it. */
  picking: boolean
}

export interface SessionSplit {
  kind: 'split'
  id: string
  axis: SplitAxis
  /** The first child's share of the split. */
  ratio: number
  first: LayoutNode
  second: LayoutNode
}

export type LayoutNode = SessionPane | SessionSplit

/**
 * What a session shows in Code: a tree of splits with panes at its leaves, as in
 * tmux. Every tmux window has exactly one tab, and the session tree lists these
 * tabs in this order, so the two always mirror each other. Tab ids derive from
 * the content, so opening a view twice finds the tab it already has.
 */
export interface SessionLayout {
  root: LayoutNode
  focusedPane: string
  nextId: number
  /** Where the Docs tab is reading; null follows the session's own space. */
  docScope: DriveScope | null
}

/** A box as shares of the whole session. */
export interface LayoutBox {
  x: number
  y: number
  width: number
  height: number
}

export function newSessionLayout(): SessionLayout {
  return {
    root: { kind: 'pane', id: 'p0', tabs: [], activeTab: null, picking: false },
    focusedPane: 'p0',
    nextId: 1,
    docScope: null,
  }
}

function isNode(value: unknown): value is LayoutNode {
  const node = value as {
    kind?: unknown
    id?: unknown
    tabs?: unknown
    ratio?: unknown
    axis?: unknown
    first?: unknown
    second?: unknown
  } | null
  if (node?.kind === 'pane') return Array.isArray(node.tabs) && typeof node.id === 'string'
  if (node?.kind !== 'split') return false
  return (
    typeof node.ratio === 'number' &&
    (node.axis === 'row' || node.axis === 'column') &&
    isNode(node.first) &&
    isNode(node.second)
  )
}

/** Storage can hold anything, so a layout that breaks the rules above starts over. */
export function isSessionLayout(value: unknown): value is SessionLayout {
  const layout = value as Partial<SessionLayout> | null | undefined
  return isNode(layout?.root) && typeof layout?.focusedPane === 'string' && typeof layout?.nextId === 'number'
}

/** Every pane, first to last: left to right and top to bottom through the tree. */
export function panesOf(layout: SessionLayout): SessionPane[] {
  const walk = (node: LayoutNode): SessionPane[] =>
    node.kind === 'pane' ? [node] : [...walk(node.first), ...walk(node.second)]
  return walk(layout.root)
}

export function paneById(layout: SessionLayout, id: string): SessionPane | undefined {
  return panesOf(layout).find((pane) => pane.id === id)
}

function firstPane(node: LayoutNode): SessionPane {
  return node.kind === 'pane' ? node : firstPane(node.first)
}

function mapPane(node: LayoutNode, id: string, change: (pane: SessionPane) => LayoutNode): LayoutNode {
  if (node.kind === 'pane') return node.id === id ? change(node) : node
  const first = mapPane(node.first, id, change)
  const second = mapPane(node.second, id, change)
  return first === node.first && second === node.second ? node : { ...node, first, second }
}

function withPane(layout: SessionLayout, id: string, change: Partial<SessionPane>): SessionLayout {
  return { ...layout, root: mapPane(layout.root, id, (pane) => ({ ...pane, ...change })) }
}

function paneWithTab(layout: SessionLayout, tabId: string): SessionPane | undefined {
  return panesOf(layout).find((pane) => pane.tabs.some((tab) => tab.id === tabId))
}

export function windowTabId(windowId: string): string {
  return `window:${windowId}`
}

export function focusPane(layout: SessionLayout, id: string): SessionLayout {
  if (layout.focusedPane === id || !paneById(layout, id)) return layout
  return { ...layout, focusedPane: id }
}

export function selectTab(layout: SessionLayout, paneId: string, tabId: string): SessionLayout {
  if (!paneById(layout, paneId)?.tabs.some((tab) => tab.id === tabId)) return layout
  return { ...withPane(layout, paneId, { activeTab: tabId, picking: false }), focusedPane: paneId }
}

function appendTab(layout: SessionLayout, paneId: string, tab: SessionTab): SessionLayout {
  const pane = paneById(layout, paneId)!
  return selectTab(withPane(layout, paneId, { tabs: [...pane.tabs, tab] }), paneId, tab.id)
}

/** A view that is already open anywhere is focused where it is. */
export function openView(layout: SessionLayout, kind: SessionViewKind, target = layout.focusedPane): SessionLayout {
  const existing = paneWithTab(layout, kind)
  if (existing) return selectTab(layout, existing.id, kind)
  return appendTab(layout, paneById(layout, target) ? target : firstPane(layout.root).id, { id: kind, kind })
}

/**
 * For a tab nobody at the keyboard asked for, such as a document an agent opened.
 * It joins the last pane and takes no focus, so typing in a terminal carries on.
 */
export function addViewQuietly(layout: SessionLayout, kind: SessionViewKind): SessionLayout {
  if (paneWithTab(layout, kind)) return layout
  const pane = panesOf(layout).at(-1)!
  return withPane(layout, pane.id, {
    tabs: [...pane.tabs, { id: kind, kind }],
    activeTab: pane.activeTab ?? kind,
  })
}

/**
 * Windows in the order the tabs and the tree show them. tmux's own can differ, as
 * when a closed pane's tabs join the end of its neighbour's, and one move cannot
 * always repair it. A window with no tab yet goes last.
 */
export function inTabOrder<T extends { windowId: string }>(windows: readonly T[], layout: SessionLayout): T[] {
  const order = windowOrder(layout)
  const position = (window: T): number => {
    const at = order.indexOf(window.windowId)
    return at < 0 ? order.length : at
  }
  return [...windows].sort((a, b) => position(a) - position(b))
}

/** The picker names a place, so a view open in another pane moves here. */
export function placeView(layout: SessionLayout, kind: SessionViewKind, target: string): SessionLayout {
  const from = paneWithTab(layout, kind)
  if (from?.id === target) return selectTab(layout, target, kind)
  if (from) return moveTab(layout, kind, target, paneById(layout, target)?.tabs.length ?? 0)
  return appendTab(layout, target, { id: kind, kind })
}

/** A window has one tab, so placing one moves that tab here. */
export function placeWindow(layout: SessionLayout, windowId: string, target: string): SessionLayout {
  const id = windowTabId(windowId)
  const from = paneWithTab(layout, id)
  if (from?.id === target) return selectTab(layout, target, id)
  if (from) return moveTab(layout, id, target, paneById(layout, target)?.tabs.length ?? 0)
  return appendTab(layout, target, { id, kind: 'window', windowId })
}

function removeTab(layout: SessionLayout, paneId: string, tabId: string): SessionLayout {
  const pane = paneById(layout, paneId)!
  const at = pane.tabs.findIndex((tab) => tab.id === tabId)
  const tabs = pane.tabs.filter((tab) => tab.id !== tabId)
  const activeTab = pane.activeTab === tabId ? ((tabs[at] ?? tabs[at - 1])?.id ?? null) : pane.activeTab
  return withPane(layout, paneId, { tabs, activeTab })
}

function removePane(node: LayoutNode, id: string): LayoutNode | null {
  if (node.kind === 'pane') return node.id === id ? null : node
  const first = removePane(node.first, id)
  const second = removePane(node.second, id)
  if (!first) return second
  if (!second) return first
  return first === node.first && second === node.second ? node : { ...node, first, second }
}

/** The pane that takes over when `id` goes: the nearest one on the other side of its split. */
function neighbourOf(node: LayoutNode, id: string): SessionPane | undefined {
  if (node.kind === 'pane') return undefined
  if (node.first.kind === 'pane' && node.first.id === id) return firstPane(node.second)
  if (node.second.kind === 'pane' && node.second.id === id) return firstPane(node.first)
  return neighbourOf(node.first, id) ?? neighbourOf(node.second, id)
}

/** A pane with nothing in it closes, wherever it is. The last pane stays. */
function dropEmptyPanes(layout: SessionLayout): SessionLayout {
  let next = layout
  for (const pane of panesOf(layout)) {
    if (pane.tabs.length || pane.picking || panesOf(next).length < 2) continue
    next = withoutPane(next, pane.id)
  }
  return next
}

function withoutPane(layout: SessionLayout, id: string): SessionLayout {
  const neighbour = neighbourOf(layout.root, id)
  const root = removePane(layout.root, id)
  if (!root || !neighbour) return layout
  return { ...layout, root, focusedPane: layout.focusedPane === id ? neighbour.id : layout.focusedPane }
}

/** Removes a tab from the layout. A window's tab goes when its tmux window goes: see reconcileWindows. */
export function closeTab(layout: SessionLayout, paneId: string, tabId: string): SessionLayout {
  if (!paneById(layout, paneId)?.tabs.some((tab) => tab.id === tabId)) return layout
  return dropEmptyPanes(removeTab(layout, paneId, tabId))
}

/** Splits a pane the way tmux does: it keeps its tabs and a new pane asks what goes beside it. */
export function splitPane(layout: SessionLayout, paneId: string, axis: SplitAxis): SessionLayout {
  if (!paneById(layout, paneId)) return layout
  const newPane: SessionPane = { kind: 'pane', id: `p${layout.nextId}`, tabs: [], activeTab: null, picking: true }
  const root = mapPane(layout.root, paneId, (pane) => ({
    kind: 'split',
    id: `s${layout.nextId}`,
    axis,
    ratio: 0.5,
    first: pane,
    second: newPane,
  }))
  return { ...layout, root, nextId: layout.nextId + 1, focusedPane: newPane.id }
}

/** Closes a pane without losing anything: its tabs join the neighbour, and a window keeps its tab. */
export function closePane(layout: SessionLayout, paneId: string): SessionLayout {
  const pane = paneById(layout, paneId)
  if (!pane) return layout
  const neighbour = neighbourOf(layout.root, paneId)
  if (!neighbour) return withPane(layout, paneId, { picking: false })
  const merged = withPane(withoutPane(layout, paneId), neighbour.id, {
    tabs: [...neighbour.tabs, ...pane.tabs],
    activeTab: neighbour.activeTab ?? pane.activeTab,
    picking: neighbour.picking && !pane.tabs.length,
  })
  return { ...merged, focusedPane: neighbour.id }
}

function mapSplit(node: LayoutNode, id: string, change: (split: SessionSplit) => SessionSplit): LayoutNode {
  if (node.kind === 'pane') return node
  if (node.id === id) return change(node)
  const first = mapSplit(node.first, id, change)
  const second = mapSplit(node.second, id, change)
  return first === node.first && second === node.second ? node : { ...node, first, second }
}

export function resizeSplit(layout: SessionLayout, splitId: string, ratio: number): SessionLayout {
  const clamped = Math.min(MAX_SPLIT_RATIO, Math.max(MIN_SPLIT_RATIO, ratio))
  const root = mapSplit(layout.root, splitId, (split) =>
    split.ratio === clamped ? split : { ...split, ratio: clamped },
  )
  return root === layout.root ? layout : { ...layout, root }
}

/** Turns one split between side by side and stacked, with its contents in place. */
export function turnSplit(layout: SessionLayout, splitId: string): SessionLayout {
  const root = mapSplit(layout.root, splitId, (split) => ({
    ...split,
    axis: split.axis === 'row' ? 'column' : 'row',
  }))
  return root === layout.root ? layout : { ...layout, root }
}

/** `gap` counts tabs in the target pane as they stand before the move. */
export function moveTab(layout: SessionLayout, tabId: string, target: string, gap: number): SessionLayout {
  const from = paneWithTab(layout, tabId)
  const destination = paneById(layout, target)
  if (!from || !destination) return layout
  const tab = from.tabs.find((candidate) => candidate.id === tabId)!
  const position = from.id === target ? destination.tabs.indexOf(tab) : -1
  const adjusted = position >= 0 && position < gap ? gap - 1 : gap
  const cleared = removeTab(layout, from.id, tabId)
  const remaining = paneById(cleared, target)!.tabs
  const at = Math.min(remaining.length, Math.max(0, adjusted))
  const placed = withPane(cleared, target, { tabs: [...remaining.slice(0, at), tab, ...remaining.slice(at)] })
  return dropEmptyPanes(selectTab(placed, target, tabId))
}

/** The tree names a neighbour rather than a pane and a gap. */
export function moveTabBeside(
  layout: SessionLayout,
  tabId: string,
  besideId: string,
  edge: 'before' | 'after',
): SessionLayout {
  const pane = paneWithTab(layout, besideId)
  if (!pane) return layout
  const at = pane.tabs.findIndex((tab) => tab.id === besideId)
  return moveTab(layout, tabId, pane.id, edge === 'after' ? at + 1 : at)
}

/**
 * The tree or a chord asked for a window: focus its tab, wherever it lives. A
 * session still attaching has no tab for it yet. Making one here would put that
 * window ahead of the others, so the active window brings it forward once it lands.
 */
export function revealWindow(layout: SessionLayout, windowId: string): SessionLayout {
  const id = windowTabId(windowId)
  const pane = paneWithTab(layout, id)
  return pane ? selectTab(layout, pane.id, id) : layout
}

/**
 * Gives every window tmux has a tab, and takes the tab away from one it no
 * longer has. The same layout comes back when nothing changed, so a poll cannot
 * cost a storage write. A pane with nothing active takes `active` when it holds
 * that window, and its first tab otherwise.
 */
export function reconcileWindows(
  layout: SessionLayout,
  windowIds: readonly string[],
  options: { active?: string; target?: string } = {},
): SessionLayout {
  let next = layout
  for (const pane of panesOf(layout)) {
    for (const tab of pane.tabs) {
      if (tab.kind === 'window' && !windowIds.includes(tab.windowId)) next = removeTab(next, pane.id, tab.id)
    }
  }
  const missing = windowIds.filter((id) => !paneWithTab(next, windowTabId(id)))
  if (missing.length) {
    const wanted = options.target ?? next.focusedPane
    const target = paneById(next, wanted) ?? firstPane(next.root)
    const added = missing.map((windowId) => ({ id: windowTabId(windowId), kind: 'window' as const, windowId }))
    // Only a window this pane asked for answers its picker. One that an agent or
    // another client opened joins behind it, and the question stays on screen.
    const picking = target.picking && target.id !== options.target
    next = withPane(next, target.id, { tabs: [...target.tabs, ...added], picking })
  }
  for (const pane of panesOf(next)) {
    if (pane.activeTab || !pane.tabs.length) continue
    const wanted = options.active ? windowTabId(options.active) : ''
    next = withPane(next, pane.id, {
      activeTab: pane.tabs.some((tab) => tab.id === wanted) ? wanted : pane.tabs[0].id,
    })
  }
  return next === layout ? layout : dropEmptyPanes(next)
}

function shownTab(pane: SessionPane): SessionTab | undefined {
  return pane.picking ? undefined : pane.tabs.find((tab) => tab.id === pane.activeTab)
}

/** Every tab in the order the tree lists it, with where each one lives. */
export function orderedTabs(layout: SessionLayout): { pane: string; tab: SessionTab }[] {
  return panesOf(layout).flatMap((pane) => pane.tabs.map((tab) => ({ pane: pane.id, tab })))
}

export function windowOrder(layout: SessionLayout): string[] {
  return orderedTabs(layout).flatMap(({ tab }) => (tab.kind === 'window' ? [tab.windowId] : []))
}

/** The tab the focused pane shows, when nothing is asking there. */
export function focusedTab(layout: SessionLayout): string {
  const pane = paneById(layout, layout.focusedPane)
  return pane ? (shownTab(pane)?.id ?? '') : ''
}

/** The window the keyboard belongs to, or '' with a view in the focused pane. */
export function focusedWindow(layout: SessionLayout): string {
  const pane = paneById(layout, layout.focusedPane)
  const tab = pane && shownTab(pane)
  return tab?.kind === 'window' ? tab.windowId : ''
}

/** Which pane shows each window on screen. */
export function windowsOnScreen(layout: SessionLayout): Map<string, string> {
  const shown = new Map<string, string>()
  for (const pane of panesOf(layout)) {
    const tab = shownTab(pane)
    if (tab?.kind === 'window') shown.set(tab.windowId, pane.id)
  }
  return shown
}

/** Windows behind another tab: what a new pane can bring forward. */
export function offscreenWindows(layout: SessionLayout, windowIds: readonly string[]): string[] {
  const shown = windowsOnScreen(layout)
  return windowIds.filter((id) => !shown.has(id))
}

/** The pane a session's terminal states show in: one holding a window, or nothing at all. */
export function terminalPane(layout: SessionLayout): string {
  const pane = panesOf(layout).find((candidate) => {
    if (candidate.picking) return false
    const tab = shownTab(candidate)
    return !tab || tab.kind === 'window'
  })
  return pane?.id ?? ''
}

const WHOLE: LayoutBox = { x: 0, y: 0, width: 1, height: 1 }

function childBoxes(split: SessionSplit, box: LayoutBox): [LayoutBox, LayoutBox] {
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

/** Where each pane sits, as shares of the session. */
export function paneBoxes(layout: SessionLayout): Map<string, LayoutBox> {
  const boxes = new Map<string, LayoutBox>()
  const walk = (node: LayoutNode, box: LayoutBox): void => {
    if (node.kind === 'pane') {
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
export function splitBoxes(layout: SessionLayout): { split: SessionSplit; box: LayoutBox }[] {
  const found: { split: SessionSplit; box: LayoutBox }[] = []
  const walk = (node: LayoutNode, box: LayoutBox): void => {
    if (node.kind === 'pane') return
    found.push({ split: node, box })
    const [first, second] = childBoxes(node, box)
    walk(node.first, first)
    walk(node.second, second)
  }
  walk(layout.root, WHOLE)
  return found
}
