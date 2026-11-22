import type MarkdownIt from 'markdown-it'
import type StateInline from 'markdown-it/lib/rules_inline/state_inline.mjs'

export type WikilinkEnv = { links?: { docPath: string; paths: ReadonlySet<string> } }

/**
 * The space path a `[[target]]` names, or null. An exact space-root path
 * wins (with `.md` added when the target lacks it), then a path that ends in
 * the target when exactly one does, which makes `[[foo]]` find `plans/foo.md`.
 */
export function resolveWikilink(target: string, paths: ReadonlySet<string>): string | null {
  const wanted = target.trim().replace(/^\/+/, '')
  if (!wanted) return null
  const candidates = wanted.toLowerCase().endsWith('.md') ? [wanted] : [wanted, `${wanted}.md`]
  for (const candidate of candidates) if (paths.has(candidate)) return candidate

  const matches = new Set<string>()
  for (const path of paths) {
    if (candidates.some((candidate) => path.endsWith(`/${candidate}`))) matches.add(path)
  }
  return matches.size === 1 ? [...matches][0] : null
}

function wikilinkHref(path: string): string {
  return `.hive/${encodeURI(path).replace(/#/g, '%23').replace(/\?/g, '%3F')}`
}

// A table cell has already lost the backslash of `\|` by the time inline
// rules run, and outside a table Obsidian writes a bare `|`, so both split.
function splitAlias(inner: string): [string, string | null] {
  const match = /\\?\|/.exec(inner)
  if (!match) return [inner, null]
  return [inner.slice(0, match.index), inner.slice(match.index + match[0].length)]
}

function parseWikilink(state: StateInline, silent: boolean): boolean {
  const env = state.env as WikilinkEnv
  if (!env.links) return false
  const start = state.pos
  const src = state.src
  if (src.charCodeAt(start) !== 0x5b || src.charCodeAt(start + 1) !== 0x5b) return false

  const end = src.indexOf(']]', start + 2)
  if (end < 0) return false
  const inner = src.slice(start + 2, end)
  if (inner.includes('\n') || inner.includes('[')) return false
  const [rawTarget, rawLabel] = splitAlias(inner)
  const target = rawTarget.trim()
  if (!target) return false

  if (!silent) {
    const token = state.push('wikilink', '', 0)
    token.content = rawLabel?.trim() || target
    token.meta = { target }
  }
  state.pos = end + 2
  return true
}

/** Inert unless the render env carries `links`, so only Drive documents turn `[[...]]` into links. */
export function wikilinks(md: MarkdownIt): void {
  md.inline.ruler.before('link', 'wikilink', parseWikilink)
  md.renderer.rules.wikilink = (tokens, index, _options, env) => {
    const token = tokens[index]
    const label = md.utils.escapeHtml(token.content)
    const links = (env as WikilinkEnv).links
    const path = links ? resolveWikilink((token.meta as { target: string }).target, links.paths) : null
    if (!path) return `<span class="wikilink is-broken">${label}</span>`
    return `<a class="wikilink" href="${md.utils.escapeHtml(wikilinkHref(path))}">${label}</a>`
  }
}
