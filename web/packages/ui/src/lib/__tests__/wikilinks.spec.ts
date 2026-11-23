import MarkdownIt from 'markdown-it'
import { describe, expect, it } from 'vitest'
import { resolveWikilink, wikilinks } from '../wikilinks'

const paths = new Set(['plans/foo.md', 'research/bar.md', 'a/dup.md', 'b/dup.md', 'notes/my plan.md', 'img/x.html'])

describe('resolveWikilink', () => {
  it.each([
    ['an exact path', 'plans/foo.md', 'plans/foo.md'],
    ['an exact path without the extension', 'plans/foo', 'plans/foo.md'],
    ['a leading slash', '/plans/foo', 'plans/foo.md'],
    ['a unique basename', 'foo', 'plans/foo.md'],
    ['a unique basename with the extension', 'bar.md', 'research/bar.md'],
    ['a partial path', 'research/bar', 'research/bar.md'],
    ['a non-markdown document', 'img/x.html', 'img/x.html'],
    ['spaces', 'my plan', 'notes/my plan.md'],
    ['an ambiguous basename', 'dup', null],
    ['an exact path beating a basename', 'a/dup', 'a/dup.md'],
    ['no match', 'nope', null],
    ['an empty target', '  ', null],
    ['a suffix inside a name', 'oo', null],
  ])('%s', (_name, target, expected) => {
    expect(resolveWikilink(target, paths)).toBe(expected)
  })
})

describe('wikilinks plugin', () => {
  const md = new MarkdownIt().use(wikilinks)
  const links = { docPath: 'plans/x.md', paths }
  const render = (src: string, env: object = { links }) => md.renderInline(src, env)

  it('is inert without env.links', () => {
    expect(render('see [[plans/foo]]', {})).toBe('see [[plans/foo]]')
  })

  it.each([
    ['a path', '[[plans/foo]]', '<a class="wikilink" href=".hive/plans/foo.md">plans/foo</a>'],
    ['a path with the extension', '[[plans/foo.md]]', '<a class="wikilink" href=".hive/plans/foo.md">plans/foo.md</a>'],
    ['a basename', '[[foo]]', '<a class="wikilink" href=".hive/plans/foo.md">foo</a>'],
    ['an alias', '[[plans/foo|the plan]]', '<a class="wikilink" href=".hive/plans/foo.md">the plan</a>'],
    ['an escaped alias pipe', '[[plans/foo\\|the plan]]', '<a class="wikilink" href=".hive/plans/foo.md">the plan</a>'],
    ['an encoded href', '[[my plan]]', '<a class="wikilink" href=".hive/notes/my%20plan.md">my plan</a>'],
    ['a broken link', '[[nope]]', '<span class="wikilink is-broken">nope</span>'],
    ['an ambiguous link', '[[dup]]', '<span class="wikilink is-broken">dup</span>'],
    ['label html escaped', '[[nope|<b>x</b>]]', '<span class="wikilink is-broken">&lt;b&gt;x&lt;/b&gt;</span>'],
  ])('renders %s', (_name, src, expected) => {
    expect(render(src)).toBe(expected)
  })

  it.each(['[[]]', '[[ ]]', '[[a', '[[a\nb]]', '[[a [b]]'])('leaves %j as text', (src) => {
    expect(render(src)).not.toContain('wikilink')
  })

  it('keeps ordinary links working', () => {
    expect(render('[x](http://a.b)')).toContain('<a href="http://a.b">x</a>')
  })
})
