// Display-only JS tokenizer for the function node's hand-rolled code overlay
// (fields/CodeField.vue) — colors comments/strings/keywords well enough to
// read at a glance, matching the --hv-code-* tokens. Not a real parser (a
// small regex pass), same posture as lib/markdown.ts and
// ../../lib/yamlHighlight.ts: a tiny hand-rolled renderer rather than a
// dependency (CodeMirror is blocked by the min-release-age policy — see
// fields/CodeField.vue).
//
// Builds an HTML string (one <span>-wrapped line per input line, joined by
// "\n" so a `white-space:pre` <pre> lays them out 1:1 with the line-number
// gutter) for `v-html`, the same technique lib/markdown.ts already uses for
// docs — so, like that file, every raw text segment is escaped *before*
// being spliced into markup. The function node's own execution is
// author-trusted (config.ts's D2 note), but this is a separate display
// surface: nothing here should let source text inject elements into the
// host page.

const KEYWORDS = new Set([
  'const', 'let', 'var', 'function', 'return', 'if', 'else', 'for', 'while', 'do',
  'switch', 'case', 'break', 'continue', 'new', 'typeof', 'instanceof', 'in', 'of',
  'this', 'class', 'extends', 'super', 'try', 'catch', 'finally', 'throw',
  'async', 'await', 'import', 'export', 'from', 'default', 'delete', 'void',
  'yield', 'static', 'true', 'false', 'null', 'undefined',
])

// Ordered alternation: a line comment, a quoted/backtick string (with
// backslash-escapes), or a bare identifier — matched left-to-right so
// earlier alternatives win at any given start position.
const TOKEN_RE = /(\/\/[^\n]*)|(`(?:\\.|[^`\\])*`|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')|(\b[A-Za-z_$][\w$]*\b)/g

function escapeHtml(text: string): string {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function highlightLine(line: string): string {
  let out = ''
  let last = 0
  TOKEN_RE.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = TOKEN_RE.exec(line)) !== null) {
    out += escapeHtml(line.slice(last, m.index))
    if (m[1]) out += `<span class="hv-code-comment">${escapeHtml(m[1])}</span>`
    else if (m[2]) out += `<span class="hv-code-string">${escapeHtml(m[2])}</span>`
    else if (m[3] && KEYWORDS.has(m[3])) out += `<span class="hv-code-key">${escapeHtml(m[3])}</span>`
    else out += escapeHtml(m[0])
    last = TOKEN_RE.lastIndex
  }
  out += escapeHtml(line.slice(last))
  return out
}

/** Renders `src` into a highlighted HTML string for a `white-space:pre` container — every raw character is escaped, only the wrapping <span class="hv-code-*"> markup is real markup. */
export function highlightCode(src: string): string {
  return src.split('\n').map(highlightLine).join('\n')
}

/** Line count for the gutter — always at least 1 (an empty string is one empty line), matching how a <pre> with the same text would lay out. */
export function codeLineCount(src: string): number {
  return src.length === 0 ? 1 : src.split('\n').length
}
