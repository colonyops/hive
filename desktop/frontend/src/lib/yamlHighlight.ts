// Display-only YAML tokenizer: enough to make keys, strings, and comments
// scan like the settings mock, nothing more. Shared by the feeds-as-code
// sheet and the feed editor's live preview; nothing here edits YAML.

export interface Token {
  text: string
  kind: 'key' | 'string' | 'comment' | 'plain'
}

export function tokenizeLine(line: string): Token[] {
  const commentAt = line.indexOf('#')
  const beforeComment = commentAt >= 0 ? line.slice(0, commentAt) : line
  const comment = commentAt >= 0 ? line.slice(commentAt) : ''

  const tokens: Token[] = []
  const keyMatch = /^(\s*-?\s*)([\w-]+)(:)(.*)$/.exec(beforeComment)
  if (keyMatch) {
    const [, indent, key, colon, rest] = keyMatch
    if (indent) tokens.push({ text: indent, kind: 'plain' })
    tokens.push({ text: key + colon, kind: 'key' })
    if (rest) tokens.push({ text: rest, kind: /["'[]/.test(rest.trimStart()[0] ?? '') ? 'string' : 'plain' })
  } else if (beforeComment) {
    tokens.push({ text: beforeComment, kind: 'plain' })
  }
  if (comment) tokens.push({ text: comment, kind: 'comment' })
  return tokens
}

/** Tokenize a whole YAML document into per-line token lists. */
export function tokenizeYaml(text: string): Token[][] {
  return text.replace(/\n$/, '').split('\n').map(tokenizeLine)
}
