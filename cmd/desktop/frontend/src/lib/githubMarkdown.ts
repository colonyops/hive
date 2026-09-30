// Renders untrusted GitHub-flavored markdown (issue / PR bodies) to HTML for
// injection via v-html. markdown-it is a pure string transform (no DOM), and
// it's safe-by-default for attacker-controllable input:
//   - Raw HTML is tokenized but always escaped by custom renderer rules. The
//     plugin below recognizes only exact <details>, <summary>, and closing tags,
//     and emits those tags itself.
//   - HTML comments are removed from parsed HTML tokens, while comment-like
//     text in code spans and fences remains visible.
//   - markdown-it's built-in link validator drops dangerous URL schemes
//     (javascript:, vbscript:, and non-image data:).
//
// GFM coverage comes from markdown-it's defaults (tables, strikethrough) plus
// linkify, task lists, GitHub alerts, and collapsible <details> sections.
// breaks:true mirrors how GitHub renders a single newline in a comment body
// as a line break.
//
// This is deliberately separate from pipeline/lib/markdown.ts, a tiny
// hand-rolled renderer for FIRST-PARTY node help.md docs.
import MarkdownIt from 'markdown-it'
import type StateBlock from 'markdown-it/lib/rules_block/state_block.mjs'
import type Token from 'markdown-it/lib/token.mjs'
import taskLists from 'markdown-it-task-lists'

const alertTypes = new Map([
  ['NOTE', 'Note'],
  ['TIP', 'Tip'],
  ['IMPORTANT', 'Important'],
  ['WARNING', 'Warning'],
  ['CAUTION', 'Caution'],
])

function lineText(state: StateBlock, line: number): string {
  return state.src.slice(state.bMarks[line] + state.tShift[line], state.eMarks[line]).trim()
}

// GitHub permits markdown inside a details element. Raw HTML remains escaped;
// this rule admits only the exact, attribute-free tags plus GitHub's `open`
// boolean attribute and lets markdown-it parse all inner content normally.
function githubDetails(md: MarkdownIt) {
  md.block.ruler.before('html_block', 'github_details', (state, startLine, endLine, silent) => {
    const opening = lineText(state, startLine).match(/^<details(?:\s+(open))?\s*>$/i)
    if (!opening) return false

    let depth = 1
    let closeLine = startLine + 1
    for (; closeLine < endLine; closeLine++) {
      const line = lineText(state, closeLine)
      if (/^<details(?:\s+open)?\s*>$/i.test(line)) depth++
      if (/^<\/details\s*>$/i.test(line) && --depth === 0) break
    }
    if (closeLine >= endLine) return false
    if (silent) return true

    const open = state.push('github_details_open', 'details', 1)
    open.map = [startLine, closeLine + 1]
    if (opening[1]) open.attrSet('open', '')

    let contentLine = startLine + 1
    const summary = contentLine < closeLine
      ? lineText(state, contentLine).match(/^<summary>(.*?)<\/summary\s*>$/i)
      : null
    if (summary) {
      state.push('github_summary_open', 'summary', 1)
      const inline = state.push('inline', '', 0)
      inline.content = summary[1]
      inline.children = []
      state.push('github_summary_close', 'summary', -1)
      contentLine++
    }

    if (contentLine < closeLine) state.md.block.tokenize(state, contentLine, closeLine)
    state.push('github_details_close', 'details', -1)
    state.line = closeLine + 1
    return true
  }, { alt: ['paragraph', 'reference', 'blockquote', 'list'] })
}

function removeHtmlComments(tokens: Token[]) {
  for (let index = tokens.length - 1; index >= 0; index--) {
    const token = tokens[index]
    if ((token.type === 'html_block' || token.type === 'html_inline') && /^<!--[\s\S]*-->$/.test(token.content.trim())) {
      tokens.splice(index, 1)
      continue
    }
    if (token.children) removeHtmlComments(token.children)
  }
}

function githubExtensions(md: MarkdownIt) {
  githubDetails(md)

  // Alert markup is parsed as a blockquote by CommonMark. Promote blockquotes
  // whose first paragraph starts with [!TYPE] into GitHub-style alert blocks.
  md.core.ruler.push('github_alerts_and_comments', (state) => {
    removeHtmlComments(state.tokens)

    for (let index = 0; index < state.tokens.length - 2; index++) {
      const open = state.tokens[index]
      const paragraph = state.tokens[index + 1]
      const inline = state.tokens[index + 2]
      if (open.type !== 'blockquote_open' || paragraph.type !== 'paragraph_open' || inline.type !== 'inline') continue

      const match = inline.content.match(/^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\](?:\n|$)/)
      if (!match) continue

      const label = alertTypes.get(match[1])!
      open.tag = 'div'
      open.attrSet('class', `markdown-alert markdown-alert-${match[1].toLowerCase()}`)

      let depth = 1
      for (let closeIndex = index + 1; closeIndex < state.tokens.length; closeIndex++) {
        if (state.tokens[closeIndex].type === 'blockquote_open') depth++
        if (state.tokens[closeIndex].type === 'blockquote_close' && --depth === 0) {
          state.tokens[closeIndex].tag = 'div'
          break
        }
      }

      const titleOpen = new state.Token('paragraph_open', 'p', 1)
      titleOpen.attrSet('class', 'markdown-alert-title')
      const title = new state.Token('inline', '', 0)
      title.content = label
      title.children = []
      md.inline.parse(label, md, state.env, title.children)
      const titleClose = new state.Token('paragraph_close', 'p', -1)
      state.tokens.splice(index + 1, 0, titleOpen, title, titleClose)

      inline.content = inline.content.slice(match[0].length)
      inline.children = []
      md.inline.parse(inline.content, md, state.env, inline.children)
      index += 3
    }
  })
}

const md = new MarkdownIt({ html: true, linkify: true, breaks: true })
  .use(taskLists)
  .use(githubExtensions)

// Enabling HTML lets the parser distinguish comments from text. Keep the
// output safe by escaping every remaining HTML token; whitelisted details
// elements never pass through these renderer rules because the block plugin
// emits dedicated token types for them.
const renderEscapedHtml = (tokens: Token[], index: number) => md.utils.escapeHtml(tokens[index].content)
md.renderer.rules.html_block = renderEscapedHtml
md.renderer.rules.html_inline = (tokens: Token[], index: number) => {
  const content = tokens[index].content
  // markdown-it-task-lists emits this fixed, disabled checkbox as an HTML
  // token. It contains no user-controlled values, so preserve it verbatim.
  if (/^<input class="task-list-item-checkbox"(?: checked="")? disabled="" type="checkbox">$/.test(content)) return content
  return md.utils.escapeHtml(content)
}

export function renderGithubMarkdown(src: string): string {
  if (!src.trim()) return ''
  return md.render(src)
}
