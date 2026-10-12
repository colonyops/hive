import type { EntryWire } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

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

export type FrontmatterScalar = string | number | boolean | null
export type FrontmatterValue = FrontmatterScalar | FrontmatterScalar[] | Record<string, unknown>

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

export function entryTime(iso: string): number {
  const ms = Date.parse(iso)
  return Number.isFinite(ms) && ms > 0 ? ms : 0
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

/** A markdown body without its YAML front matter, which the reader shows as properties. */
export function markdownBody(source: string): string {
  const match = /^---\r?\n[\s\S]*?\r?\n---[ \t]*(?:\r?\n|$)/.exec(source)
  return match ? source.slice(match[0].length) : source
}

export type LinkTarget = { kind: 'url'; url: string } | { kind: 'document'; path: string }

function resolvePath(base: string[], relative: string): string | null {
  const parts = [...base]
  for (const segment of relative.split('/')) {
    if (segment === '' || segment === '.') continue
    if (segment === '..') {
      if (!parts.length) return null
      parts.pop()
    } else parts.push(segment)
  }
  return parts.join('/')
}

/**
 * Where a link in a document leads. A web or mail address opens outside the
 * app. A path that names a document in the same space opens it: `.hive/...`
 * from the space root, as a checkout's link reads, and anything else from the
 * linking document's folder. A bare canvas name still finds its canvas.
 * Anything else leads nowhere, because the webview must never navigate.
 */
export function linkTarget(href: string, fromPath: string, paths: ReadonlySet<string>): LinkTarget | null {
  if (/^(https?:|mailto:)/i.test(href)) return { kind: 'url', url: href }
  if (/^[a-z][a-z0-9+.-]*:/i.test(href) || href.startsWith('/') || href.startsWith('#')) return null
  let reference = href.replace(/[?#].*$/, '')
  try {
    reference = decodeURIComponent(reference)
  } catch {
    return null
  }
  const fromRoot = reference.startsWith('.hive/')
  const base = fromRoot ? [] : fromPath.split('/').slice(0, -1)
  const resolved = resolvePath(base, fromRoot ? reference.slice('.hive/'.length) : reference)
  if (!resolved) return null
  if (paths.has(resolved)) return { kind: 'document', path: resolved }
  const canvas = `${resolved}.canvas.json`
  return paths.has(canvas) ? { kind: 'document', path: canvas } : null
}
