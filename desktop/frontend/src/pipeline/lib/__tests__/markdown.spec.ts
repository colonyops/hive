import { describe, expect, it } from 'vitest'
import { renderMarkdown, summarize } from '../markdown'

describe('renderMarkdown', () => {
  it('renders headings at their level', () => {
    expect(renderMarkdown('# Title')).toBe('<h1>Title</h1>')
    expect(renderMarkdown('### Sub')).toBe('<h3>Sub</h3>')
  })

  it('renders a paragraph, joining wrapped lines with a space', () => {
    expect(renderMarkdown('one\ntwo')).toBe('<p>one two</p>')
  })

  it('renders inline code without further escaping its contents', () => {
    expect(renderMarkdown('call `msg.Payload` here')).toBe('<p>call <code>msg.Payload</code> here</p>')
  })

  it('renders bold text', () => {
    expect(renderMarkdown('**required**')).toBe('<p><strong>required</strong></p>')
  })

  it('escapes HTML-significant characters', () => {
    expect(renderMarkdown('a < b & c > d')).toBe('<p>a &lt; b &amp; c &gt; d</p>')
  })

  it('renders a fenced code block verbatim (no inline processing)', () => {
    const src = ['```js', 'if (msg.Payload.ok) return msg;', '```'].join('\n')
    expect(renderMarkdown(src)).toBe('<pre><code>if (msg.Payload.ok) return msg;</code></pre>')
  })

  it('closes an unterminated fence at EOF', () => {
    const src = ['```', 'return null'].join('\n')
    expect(renderMarkdown(src)).toBe('<pre><code>return null</code></pre>')
  })

  it('renders an unordered list', () => {
    const src = ['- one', '- two'].join('\n')
    expect(renderMarkdown(src)).toBe('<ul><li>one</li><li>two</li></ul>')
  })

  it('renders an ordered list', () => {
    const src = ['1. first', '2. second'].join('\n')
    expect(renderMarkdown(src)).toBe('<ol><li>first</li><li>second</li></ol>')
  })

  it('renders headings, paragraphs, and lists in sequence', () => {
    const src = ['# Title', '', 'Intro paragraph.', '', '- a', '- b', '', 'Outro.'].join('\n')
    expect(renderMarkdown(src)).toBe(
      '<h1>Title</h1>\n<p>Intro paragraph.</p>\n<ul><li>a</li><li>b</li></ul>\n<p>Outro.</p>',
    )
  })
})

describe('summarize', () => {
  it('returns the first paragraph, skipping a leading heading', () => {
    const src = ['# Function', '', 'A function node runs JS.', '', 'More detail below.'].join('\n')
    expect(summarize(src)).toBe('A function node runs JS.')
  })

  it('joins a wrapped paragraph with spaces', () => {
    const src = ['# Title', '', 'Line one', 'line two.'].join('\n')
    expect(summarize(src)).toBe('Line one line two.')
  })

  it('returns an empty string for a heading-only doc', () => {
    expect(summarize('# Title')).toBe('')
  })
})
