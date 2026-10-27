// Mirror driveapi.SnippetMatchStart and SnippetMatchEnd.
const matchStart = ''
const matchEnd = ''

export interface SnippetPart {
  text: string
  match: boolean
}

/**
 * Splits a server snippet into plain and matched runs. The text is never
 * parsed as HTML, so the caller renders each part as text.
 */
export function snippetParts(snippet: string): SnippetPart[] {
  const parts: SnippetPart[] = []
  let match = false
  let rest = snippet
  while (rest) {
    const at = rest.indexOf(match ? matchEnd : matchStart)
    const text = at < 0 ? rest : rest.slice(0, at)
    if (text) parts.push({ text, match })
    if (at < 0) break
    rest = rest.slice(at + 1)
    match = !match
  }
  return parts
}
