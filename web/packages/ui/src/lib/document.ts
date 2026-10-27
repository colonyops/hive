export type FrontmatterScalar = string | number | boolean | null
export type FrontmatterValue = FrontmatterScalar | FrontmatterScalar[] | Record<string, unknown>

export function entryTime(iso: string): number {
  const ms = Date.parse(iso)
  return Number.isFinite(ms) && ms > 0 ? ms : 0
}

const frontmatterPattern = /^---\r?\n[\s\S]*?\r?\n---[ \t]*(?:\r?\n|$)/

/** A markdown body without its YAML front matter, which the reader shows as properties. */
export function markdownBody(source: string): string {
  const match = frontmatterPattern.exec(source)
  return match ? source.slice(match[0].length) : source
}

/** How many source lines markdownBody cuts off the top. */
export function frontmatterLineCount(source: string): number {
  const match = frontmatterPattern.exec(source)
  return match ? match[0].split('\n').length - 1 : 0
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

export interface ReaderEntry {
  path: string
  kind: 'markdown' | 'html' | 'canvas'
  front?: Record<string, FrontmatterValue>
  /** ISO 8601. */
  created?: string
  modified?: string
}

export interface ReaderBlock {
  id: string
  kind: string
  title?: string
  body?: string
  url?: string
}

export interface ReaderCanvas {
  blocks: ReaderBlock[]
  frontmatter?: Record<string, FrontmatterValue>
  /** ISO 8601. */
  createdAt?: string
  updatedAt?: string
}

export interface ReaderDocument {
  entry: ReaderEntry
  markdown?: string
  html?: string
  canvas?: ReaderCanvas
}
