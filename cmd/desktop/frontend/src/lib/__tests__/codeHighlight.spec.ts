import { describe, expect, it } from 'vitest'
import { highlightCode } from '../codeHighlight'

describe('highlightCode', () => {
  it('highlights a Go snippet with both theme variables on token spans', () => {
    const html = highlightCode('func main() {\n\tfmt.Println("hi")\n}\n', 'go')!
    expect(html).toContain('class="shiki')
    expect(html).toMatch(/<span style="[^"]*--shiki-light:#[0-9a-f]{6}[^"]*--shiki-dark:#[0-9a-f]{6}/i)
    expect(html).toContain('--shiki-light-bg')
    expect(html).toContain('--shiki-dark-bg')
  })

  it('returns null for an unknown language', () => {
    expect(highlightCode('x', 'nonsense')).toBeNull()
    expect(highlightCode('x', '')).toBeNull()
  })

  it('does not highlight mermaid', () => {
    expect(highlightCode('A --> B', 'mermaid')).toBeNull()
  })

  it.each([
    ['sh', 'bash'],
    ['zsh', 'bash'],
    ['ts', 'typescript'],
    ['js', 'javascript'],
    ['yml', 'yaml'],
    ['md', 'markdown'],
    ['py', 'python'],
    ['rs', 'rust'],
    ['golang', 'go'],
    ['GO', 'go'],
  ])('resolves the alias %s', (alias, canonical) => {
    const code = 'x = 1'
    expect(highlightCode(code, alias)).toBe(highlightCode(code, canonical))
    expect(highlightCode(code, alias)).not.toBeNull()
  })

  it('escapes markup in the code', () => {
    const html = highlightCode('<script>alert(1)</script>', 'html')!
    expect(html).not.toContain('<script>')
    expect(html).toContain('&#x3C;')
  })
})
