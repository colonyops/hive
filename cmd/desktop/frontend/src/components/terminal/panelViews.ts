import type { Component } from 'vue'
import IconFileDiff from '~icons/lucide/file-diff'
import IconFolderTree from '~icons/lucide/folder-tree'
import IconListTodo from '~icons/lucide/list-todo'
import type { DriveScope } from '../../lib/drive'
import { tabById, withTabParams, type PanelLayout, type PanelTab } from '../../lib/panelLayout'
import type { TerminalSessionRow } from '../../stores/useTerminalSessions'

export type SessionViewKind = 'docs' | 'diff' | 'tasks'

/** The questions a view's availability asks about a session; the tree answers them. */
export interface SessionKinds {
  isChat(row: TerminalSessionRow): boolean
  isHiveSession(row: TerminalSessionRow): boolean
  isServerSession(row: TerminalSessionRow): boolean
}

export interface SessionViewSpec {
  kind: SessionViewKind
  label: string
  icon: Component
  offeredTo(row: TerminalSessionRow, kinds: SessionKinds): boolean
}

// A pinned chat and the scratch terminal have no session checkout, and a
// server's session has none on this machine.
const hasLocalCheckout = (row: TerminalSessionRow, kinds: SessionKinds): boolean =>
  kinds.isHiveSession(row) && !kinds.isServerSession(row)

/**
 * The views a session can open beside its windows, declared once: the picker,
 * the add menus, the tab row and the tree all read this list. Each is open once
 * per session at most, so a view's tab id is its kind.
 */
export const SESSION_VIEWS: readonly SessionViewSpec[] = [
  {
    kind: 'docs',
    label: 'Drive',
    icon: IconFolderTree,
    offeredTo: (row, kinds) => !!row.canvasOwner || kinds.isChat(row),
  },
  { kind: 'diff', label: 'Diff', icon: IconFileDiff, offeredTo: hasLocalCheckout },
  { kind: 'tasks', label: 'Tasks', icon: IconListTodo, offeredTo: hasLocalCheckout },
]

export function isSessionViewKind(kind: string): kind is SessionViewKind {
  return SESSION_VIEWS.some((view) => view.kind === kind)
}

export function sessionView(kind: SessionViewKind): SessionViewSpec {
  return SESSION_VIEWS.find((view) => view.kind === kind)!
}

/** The view a tab shows, or nothing for a window. */
export function viewOfTab(tab: PanelTab): SessionViewSpec | undefined {
  return isSessionViewKind(tab.kind) ? sessionView(tab.kind) : undefined
}

export function viewTab(kind: SessionViewKind): PanelTab {
  return { id: kind, kind }
}

export function viewsFor(row: TerminalSessionRow, kinds: SessionKinds): SessionViewKind[] {
  return SESSION_VIEWS.filter((view) => view.offeredTo(row, kinds)).map((view) => view.kind)
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
