// Renders untrusted GitHub-flavored markdown (issue / PR bodies) to HTML for
// injection via v-html. markdown-it is a pure string transform (no DOM), and
// it's safe-by-default for attacker-controllable input:
//   - html:false escapes any raw HTML embedded in the markdown, so a body
//     like `<script>…</script>` renders as inert text, never a live element.
//   - its built-in link validator drops dangerous URL schemes (javascript:,
//     vbscript:, and non-image data:), so a `[x](javascript:…)` never becomes
//     a clickable anchor.
// Because escaping happens during parse, no separate sanitizer pass is needed.
//
// GFM coverage comes from markdown-it's defaults (tables, strikethrough) plus
// linkify (autolink bare URLs) and the task-lists plugin (checkbox items).
// breaks:true mirrors how GitHub renders a single newline in a comment body
// as a line break.
//
// This is deliberately separate from pipeline/lib/markdown.ts, a tiny
// hand-rolled renderer for FIRST-PARTY node help.md docs — trusted authors, a
// fixed subset, no need for a full parser.
import MarkdownIt from 'markdown-it'
import taskLists from 'markdown-it-task-lists'

const md = new MarkdownIt({ html: false, linkify: true, breaks: true }).use(taskLists)

export function renderGithubMarkdown(src: string): string {
  if (!src.trim()) return ''
  return md.render(src)
}
