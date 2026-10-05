/** Who wrote a canvas: a chat by its record id, or a hive session by its id. */
export type CanvasAuthor = number | string

/**
 * Which canvas a canvas surface is pointed at. `workspace` is the owner key: a
 * workspace's directory name, or `owner/repo` for a repository. A null name
 * lets the default win: the most recent canvas of `session`, else the owner's
 * most recent.
 */
export interface CanvasScope {
  workspace: string
  name: string | null
  session: CanvasAuthor | null
}

/** The owner key every agent outside a chat and a hive session writes to. */
export const globalCanvasOwner = '@global'
export const globalCanvasOwnerLabel = 'Global'

/** A repository's owner key holds a slash, which a workspace directory cannot. */
export function isRepositoryCanvasOwner(owner: string): boolean {
  return owner.includes('/')
}

/**
 * The author a canvas:updated or canvas:toggle event names, or null when it
 * names none. A chat and a hive session never share an event.
 */
export function canvasEventAuthor(data: unknown): CanvasAuthor | null {
  const payload = (Array.isArray(data) ? data[0] : data) as { session?: unknown; hiveSession?: unknown } | undefined
  if (typeof payload?.hiveSession === 'string' && payload.hiveSession) return payload.hiveSession
  const session = Number(payload?.session)
  return Number.isInteger(session) && session > 0 ? session : null
}

export type CanvasLinkTarget = { kind: 'url'; url: string } | { kind: 'canvas'; name: string }

/**
 * Where a link on a canvas leads. A web or mail address opens outside the app,
 * and a relative reference that names a canvas in the same workspace opens that
 * canvas. Anything else leads nowhere, because the webview must never navigate.
 */
export function canvasLinkTarget(href: string, canvasNames: ReadonlySet<string>): CanvasLinkTarget | null {
  if (/^(https?:|mailto:)/i.test(href)) return { kind: 'url', url: href }
  const name = href.replace(/^\.\//, '')
  return canvasNames.has(name) ? { kind: 'canvas', name } : null
}
