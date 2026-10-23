import { entryTime, type ReaderDocument } from '@hive/ui/lib/document'
import type {
  DocumentWire,
  EntryWire,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

/** Who wrote a document through an agent: a chat by its record id, or a hive session by its id. */
export type DriveAuthor = string

export type DriveKind = 'dir' | 'markdown' | 'html' | 'canvas' | 'file'

export type SpaceGroup = 'Repositories' | 'Workspaces' | 'Shared'

/** The order the space picker lists its groups in. */
export const spaceGroups: readonly SpaceGroup[] = ['Repositories', 'Workspaces', 'Shared']

export const sharedSpaceKey = 'shared'

/**
 * What a drive surface is pointed at. A null path lets the default win: the
 * newest document `session` wrote, else the newest in the space.
 */
export interface DriveScope {
  space: string
  path: string | null
  session: DriveAuthor | null
}

export function workspaceSpaceKey(dir: string): string {
  return `workspaces/${dir}`
}

/**
 * Maps the owner key a hive://canvas link and the hive-canvas MCP results
 * still use onto a space key.
 */
export function ownerToSpaceKey(owner: string): string {
  if (owner === '@global') return sharedSpaceKey
  return owner.includes('/') ? owner : workspaceSpaceKey(owner)
}

export function spaceGroup(key: string): SpaceGroup {
  if (key === sharedSpaceKey) return 'Shared'
  return key.startsWith('workspaces/') ? 'Workspaces' : 'Repositories'
}

/** The canvas document a bare canvas name is filed under, as drive.CanvasPath does in Go. */
export function canvasDocumentPath(name: string): string {
  return name.includes('/') ? `${name}.canvas.json` : `canvases/${name}.canvas.json`
}

export function isDocument(entry: EntryWire): boolean {
  return entry.kind !== 'dir'
}

export function entryLabel(entry: EntryWire): string {
  return entry.title || entry.name
}

/** Whether the tree filter keeps entry: its name or title contains the query, ignoring case. */
export function entryMatches(entry: EntryWire, query: string): boolean {
  const q = query.trim().toLowerCase()
  return entry.name.toLowerCase().includes(q) || entry.title.toLowerCase().includes(q)
}

/** Every document under root, depth first. */
export function documentsOf(root: EntryWire | undefined | null): EntryWire[] {
  const out: EntryWire[] = []
  const walk = (entry: EntryWire) => {
    for (const child of entry.children ?? []) {
      if (child.kind === 'dir') walk(child)
      else out.push(child)
    }
  }
  if (root) walk(root)
  return out
}

export function newestDocument(root: EntryWire | undefined | null): EntryWire | null {
  let newest: EntryWire | null = null
  for (const entry of documentsOf(root)) {
    if (!newest || entryTime(entry.modified) > entryTime(newest.modified)) newest = entry
  }
  return newest
}

/**
 * Where path lives after from moved to to: itself or anything under it
 * follows, anything else stays put.
 */
export function movedPath(path: string, from: string, to: string): string {
  if (path === from) return to
  return path.startsWith(`${from}/`) ? to + path.slice(from.length) : path
}

/** The folders that hold path, outermost first. */
export function ancestorPaths(path: string): string[] {
  const parts = path.split('/')
  return parts.slice(0, -1).map((_, index) => parts.slice(0, index + 1).join('/'))
}

/**
 * The author a drive:updated or drive:toggle event names, or null when it
 * names none: a change the watcher found on disk has no author.
 */
export function driveEventAuthor(data: unknown): DriveAuthor | null {
  const payload = (Array.isArray(data) ? data[0] : data) as { session?: unknown; hiveSession?: unknown } | undefined
  if (typeof payload?.hiveSession === 'string' && payload.hiveSession) return payload.hiveSession
  return typeof payload?.session === 'string' && payload.session ? payload.session : null
}

function isoOrUndefined(unixMs: number): string | undefined {
  return unixMs > 0 ? new Date(unixMs).toISOString() : undefined
}

export function toReaderDocument(doc: DocumentWire): ReaderDocument | null {
  const { entry, canvas } = doc
  if (entry.kind !== 'markdown' && entry.kind !== 'html' && entry.kind !== 'canvas') return null
  return {
    entry: {
      path: entry.path,
      kind: entry.kind,
      front: entry.front ?? undefined,
      created: entry.created,
      modified: entry.modified,
    },
    markdown: doc.markdown,
    html: doc.html,
    canvas: canvas
      ? {
          blocks: (canvas.blocks ?? []).map(({ id, kind, title, body, url }) => ({ id, kind, title, body, url })),
          frontmatter: canvas.frontmatter ?? undefined,
          createdAt: isoOrUndefined(canvas.createdAt),
          updatedAt: isoOrUndefined(canvas.updatedAt),
        }
      : undefined,
  }
}
