/**
 * The href a click inside rendered untrusted markdown should open externally,
 * or null when the click was not on a safely-openable link. Once an anchor is
 * involved the default navigation is always prevented, whatever the href:
 * rendered bodies must never navigate the webview away from the app. Shared
 * by every .markdown-body surface so a new one cannot silently skip the
 * guard.
 */
export function externalMarkdownHref(event: MouseEvent): string | null {
  const anchor = (event.target as HTMLElement | null)?.closest('a')
  if (!anchor) return null
  event.preventDefault()
  const href = anchor.getAttribute('href') ?? ''
  return /^(https?:|mailto:)/i.test(href) ? href : null
}
