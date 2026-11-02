import type { Component } from 'vue'
import IconFileDiff from '~icons/lucide/file-diff'
import IconFolderTree from '~icons/lucide/folder-tree'
import IconListTodo from '~icons/lucide/list-todo'
import type { DriveScope } from '../../lib/drive'
import { tabById, withTabParams, type PanelLayout, type PanelTab } from '../../lib/panelLayout'

/**
 * What a view's availability asks about the session hosting it. Each host
 * answers from what it has: Code from the tree's row, Chats from its pane.
 */
export interface SessionViewHost {
  /** A pinned chat in Code, or the open chat in Chats. */
  chat: boolean
  /** A hive session with its checkout on this machine. */
  localCheckout: boolean
  /** The session has a Drive space of its own to read. */
  drive: boolean
}

const VIEWS = [
  {
    kind: 'docs',
    wire: 'drive',
    label: 'Drive',
    icon: IconFolderTree,
    ownsKeys: false,
    offeredTo: (host: SessionViewHost) => host.drive || host.chat,
  },
  {
    kind: 'diff',
    wire: 'diff',
    label: 'Diff',
    icon: IconFileDiff,
    ownsKeys: true,
    offeredTo: (host: SessionViewHost) => host.localCheckout,
  },
  {
    kind: 'tasks',
    wire: 'tasks',
    label: 'Tasks',
    icon: IconListTodo,
    ownsKeys: true,
    offeredTo: (host: SessionViewHost) => host.localCheckout,
  },
] as const

/** The layout's name for a view, which is also its tab id. */
export type SessionViewKind = (typeof VIEWS)[number]['kind']
/** The name an agent and the MCP tools use for a view. */
export type SessionViewWire = (typeof VIEWS)[number]['wire']

export interface SessionViewSpec {
  kind: SessionViewKind
  wire: SessionViewWire
  label: string
  icon: Component
  /** The view reads bare keys off the window while it is in front, so the keymap must leave them to it. */
  ownsKeys: boolean
  offeredTo(host: SessionViewHost): boolean
}

/**
 * The views a session can open beside its windows, declared once: the picker,
 * the add menus, the tab row, the tree and the wire all read this list. Each is
 * open once per session at most, so a view's tab id is its kind.
 */
export const SESSION_VIEWS: readonly SessionViewSpec[] = VIEWS

export function isSessionViewKind(kind: string): kind is SessionViewKind {
  return SESSION_VIEWS.some((view) => view.kind === kind)
}

export function sessionView(kind: SessionViewKind): SessionViewSpec {
  return SESSION_VIEWS.find((view) => view.kind === kind)!
}

/** The view a wire name stands for, or nothing for a name the wire does not know. */
export function viewKindOfWire(wire: string | undefined): SessionViewKind | undefined {
  return SESSION_VIEWS.find((view) => view.wire === wire)?.kind
}

/** The view a tab shows, or nothing for a window. */
export function viewOfTab(tab: PanelTab): SessionViewSpec | undefined {
  return isSessionViewKind(tab.kind) ? sessionView(tab.kind) : undefined
}

export function viewTab(kind: SessionViewKind): PanelTab {
  return { id: kind, kind }
}

export function viewsFor(host: SessionViewHost): SessionViewKind[] {
  return SESSION_VIEWS.filter((view) => view.offeredTo(host)).map((view) => view.kind)
}

function isDriveScope(value: unknown): value is DriveScope {
  const scope = value as Partial<DriveScope> | null
  return typeof scope?.space === 'string' && 'path' in scope && 'session' in scope
}

/** Where the Drive tab is reading, carried on the tab; null follows the session's own space. */
export function docsScopeOf(layout: PanelLayout): DriveScope | null {
  const scope = tabById(layout, 'docs')?.params?.scope
  return isDriveScope(scope) ? scope : null
}

export function withDocsScope(layout: PanelLayout, scope: DriveScope | null): PanelLayout {
  return withTabParams(layout, 'docs', { scope })
}
