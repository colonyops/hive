// Tiny hand-rolled markdown -> HTML renderer for node help.md docs — same
// posture as ../../lib/yamlHighlight.ts (no markdown npm dependency; see D2's
// "Documentation" section). Supports just enough of the subset every
// help.md in this repo actually uses: headings, paragraphs, inline code,
// fenced code blocks, bold, and unordered/ordered lists. Anything else
// degrades to a plain paragraph — help.md authors are first-party/trusted
// content, not untrusted user input, so this deliberately isn't a full
// CommonMark implementation (and doesn't attempt to sanitize input beyond
// the escaping below).

function escapeHtml(text: string): string {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function renderInline(text: string): string {
  let out = escapeHtml(text)
  // Inline code first so `**not bold**` inside backticks isn't touched by
  // the bold pass below.
  out = out.replace(/`([^`]+)`/g, '<code>$1</code>')
  out = out.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
  return out
}

interface ListState {
  ordered: boolean
  items: string[]
}

/** Renders a help.md document's markdown source into a small, fixed set of HTML tags (h1-h6, p, code, pre, strong, ul/ol/li). */
export function renderMarkdown(src: string): string {
  const lines = src.replace(/\r\n/g, '\n').split('\n')
  const html: string[] = []
  let list: ListState | null = null
  let inCode = false
  let codeLines: string[] = []
  let paragraph: string[] = []

  function flushParagraph() {
    if (paragraph.length > 0) {
      html.push(`<p>${renderInline(paragraph.join(' '))}</p>`)
      paragraph = []
    }
  }

  function flushList() {
    if (list) {
      const tag = list.ordered ? 'ol' : 'ul'
      html.push(`<${tag}>${list.items.map((item) => `<li>${renderInline(item)}</li>`).join('')}</${tag}>`)
      list = null
    }
  }

  for (const line of lines) {
    if (inCode) {
      if (line.trim() === '```') {
        html.push(`<pre><code>${escapeHtml(codeLines.join('\n'))}</code></pre>`)
        codeLines = []
        inCode = false
      } else {
        codeLines.push(line)
      }
      continue
    }

    if (line.trim().startsWith('```')) {
      flushParagraph()
      flushList()
      inCode = true
      continue
    }

    const heading = /^(#{1,6})\s+(.*)$/.exec(line)
    if (heading) {
      flushParagraph()
      flushList()
      const level = heading[1]!.length
      html.push(`<h${level}>${renderInline(heading[2]!)}</h${level}>`)
      continue
    }

    const unordered = /^[-*]\s+(.*)$/.exec(line)
    const ordered = /^\d+\.\s+(.*)$/.exec(line)
    if (unordered || ordered) {
      flushParagraph()
      const isOrdered = !!ordered
      const text = (unordered ?? ordered)![1]!
      if (!list || list.ordered !== isOrdered) {
        flushList()
        list = { ordered: isOrdered, items: [] }
      }
      list.items.push(text)
      continue
    }

    if (line.trim() === '') {
      flushParagraph()
      flushList()
      continue
    }

    paragraph.push(line.trim())
  }

  if (inCode) html.push(`<pre><code>${escapeHtml(codeLines.join('\n'))}</code></pre>`)
  flushParagraph()
  flushList()

  return html.join('\n')
}

/**
 * The first paragraph of a help.md doc, skipping leading headings — used as
 * a short summary by the drawer's collapsed Docs section and the palette's
 * hover card.
 */
export function summarize(markdown: string): string {
  const lines = markdown.replace(/\r\n/g, '\n').split('\n')
  const paragraph: string[] = []
  let started = false
  for (const rawLine of lines) {
    const line = rawLine.trim()
    if (!started) {
      if (line === '' || line.startsWith('#')) continue
      started = true
    }
    if (line === '') break
    paragraph.push(line)
  }
  return paragraph.join(' ')
}
