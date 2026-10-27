const base = 'http://drive.invalid'

/**
 * The in-app path a `redirect` query names, or null. Anything that would
 * leave the app (`//host`, `https:`, `/\host`) or loop back into sign-in is
 * refused, because the query comes from a link anyone can craft.
 */
export function safeRedirect(value: unknown): string | null {
  if (typeof value !== 'string' || !value.startsWith('/') || value.includes('\\')) return null
  let url: URL
  try {
    url = new URL(value, base)
  } catch {
    return null
  }
  if (url.origin !== base || url.pathname.startsWith('/sign-in')) return null
  return url.pathname + url.search + url.hash
}

/** The query that carries `path` through sign-in; empty for the default landing page or an unsafe path. */
export function redirectQuery(path: unknown): { redirect?: string } {
  const safe = safeRedirect(path)
  return safe && safe !== '/' ? { redirect: safe } : {}
}
