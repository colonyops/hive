import { errorText } from '@hive/ui/lib/errorText'
import type {
  Anchor,
  Comment,
  CreatedToken,
  CreateTokenRequest,
  DriveDocument,
  Me,
  PresenceUpdate,
  PublicDocument,
  SearchResults,
  Share,
  ShareExpiry,
  SpaceSummary,
  Thread,
  ThreadList,
  Token,
} from './types'

export class ApiError extends Error {
  readonly status: number
  readonly kind: string
  /** Seconds from the Retry-After header of a 429. */
  readonly retryAfter?: number

  constructor(status: number, kind: string, message: string, retryAfter?: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.kind = kind
    this.retryAfter = retryAfter
  }
}

// These answer 401 for a wrong code or a dead link, not for an expired session.
// Presence names its stream by a secret, so a 401 there says nothing about the session.
const sessionless = ['/api/v1/auth/', '/api/v1/public/', '/api/v1/presence']

let unauthorized: (() => void) | undefined

/** `handler` runs when a call that needs the session answers 401. */
export function onUnauthorized(handler: () => void): void {
  unauthorized = handler
}

/** The text to show for a failed call: a rate limit names its wait. */
export function describeApiError(error: unknown, fallback: string): string {
  if (error instanceof ApiError && error.status === 429) {
    const wait = error.retryAfter ? `Try again in ${error.retryAfter} seconds.` : 'Try again later.'
    return `Too many attempts. ${wait}`
  }
  return errorText(error, fallback)
}

async function fail(path: string, res: Response): Promise<never> {
  if (res.status === 401 && !sessionless.some((prefix) => path.startsWith(prefix))) unauthorized?.()
  let kind = 'unknown'
  let message = `Request failed (${res.status}).`
  try {
    const body = (await res.json()) as { kind?: string; message?: string }
    kind = body.kind ?? kind
    message = body.message ?? message
  } catch {
    // A proxy or a plain-text error carries no JSON body; the status text above stands.
  }
  const header = Number(res.headers.get('Retry-After'))
  throw new ApiError(res.status, kind, message, Number.isFinite(header) && header > 0 ? header : undefined)
}

async function send(method: string, path: string, body?: unknown): Promise<Response> {
  const res = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) await fail(path, res)
  return res
}

async function json<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await send(method, path, body)
  return (await res.json()) as T
}

async function empty(method: string, path: string, body?: unknown): Promise<void> {
  await send(method, path, body)
}

export function encodePath(path: string): string {
  return path.split('/').map(encodeURIComponent).join('/')
}

/** `apiPrefix` is a space's route prefix from lib/space.ts. */
export const docUrl = (apiPrefix: string, path: string) => `${apiPrefix}/docs/${encodePath(path)}`

export const api = {
  me: () => json<Me>('GET', '/api/v1/me'),
  startSignIn: (email: string) => empty('POST', '/api/v1/auth/code', { email }),
  verifySignIn: (email: string, code: string) => empty('POST', '/api/v1/auth/verify', { email, code }),
  signOut: () => empty('POST', '/api/v1/auth/sign-out'),

  spaces: () => json<SpaceSummary[]>('GET', '/api/v1/spaces'),
  search: (query: string, space?: string, limit = 20) => {
    const params = new URLSearchParams({ q: query, limit: String(limit) })
    if (space) params.set('space', space)
    return json<SearchResults>('GET', `/api/v1/search?${params.toString()}`)
  },
  recent: (limit = 100) => json<DriveDocument[]>('GET', `/api/v1/recent?limit=${limit}`),
  sharedLinks: () => json<DriveDocument[]>('GET', '/api/v1/shared-links'),
  documents: (apiPrefix: string) => json<DriveDocument[]>('GET', apiPrefix),
  /** The body and its sha256 from the ETag, the version a new thread is checked against. */
  documentBody: async (apiPrefix: string, path: string): Promise<{ body: string; sha: string }> => {
    const res = await send('GET', docUrl(apiPrefix, path))
    return { body: await res.text(), sha: (res.headers.get('ETag') ?? '').replace(/^(?:W\/)?"|"$/g, '') }
  },
  deleteDocument: (apiPrefix: string, path: string) => empty('DELETE', docUrl(apiPrefix, path)),
  share: (apiPrefix: string, path: string, expiresIn: ShareExpiry) =>
    json<Share>('POST', `${apiPrefix}/shares/${encodePath(path)}`, { expires_in: expiresIn || 'none' }),
  unshare: (apiPrefix: string, path: string) => empty('DELETE', `${apiPrefix}/shares/${encodePath(path)}`),

  tokens: () => json<Token[]>('GET', '/api/v1/tokens'),
  createToken: (req: CreateTokenRequest) => json<CreatedToken>('POST', '/api/v1/tokens', req),
  revokeToken: (id: string) => empty('DELETE', `/api/v1/tokens/${encodeURIComponent(id)}`),

  publicDocument: (slug: string) => json<PublicDocument>('GET', `/api/v1/public/${encodeURIComponent(slug)}`),
  updateMe: (displayName: string) => json<Me>('PATCH', '/api/v1/me', { display_name: displayName }),

  // Collab routes take the URLs lib/collab.ts builds for an owner or a link target.
  threads: (url: string) => json<ThreadList>('GET', url),
  createThread: (url: string, documentSha: string, anchor: Anchor | null, body: string) =>
    json<Thread>('POST', url, { document_sha256: documentSha, anchor: anchor ?? undefined, body }),
  reply: (url: string, body: string) => json<Comment>('POST', url, { body }),
  setResolved: (url: string, resolved: boolean) => json<Thread>('PATCH', url, { resolved }),
  editComment: (url: string, body: string) => json<Comment>('PATCH', url, { body }),
  deleteComment: (url: string) => empty('DELETE', url),
  presence: (update: PresenceUpdate) => empty('POST', '/api/v1/presence', update),
}
