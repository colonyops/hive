import { beforeEach, describe, expect, it } from 'vitest'
import { useCodeHighlight } from '../useCodeHighlight'

/** A render asks, the queue drains, the next render finds the fence. */
async function highlighted(code: string, lang: string): Promise<string | null> {
  const store = useCodeHighlight()
  store.highlight(code, lang)
  await store.settle()
  return store.highlight(code, lang)
}

describe('useCodeHighlight', () => {
  beforeEach(() => useCodeHighlight().load())

  it('answers null for a fence it has not highlighted yet, then the cached HTML', async () => {
    const { highlight, settle } = useCodeHighlight()
    const code = 'package main\n'
    expect(highlight(code, 'go')).toBeNull()
    await settle()
    expect(highlight(code, 'go')).toContain('class="shiki')
  })

  it('highlights a one-line Go struct with a block comment without stalling', async () => {
    const code =
      'type SignIn struct{ store Store; allow func(string) bool; users map[string]string /* email -> hash */; outbox OutboxStore; limits Limits; now func() time.Time; code func() string }\n'
    const started = performance.now()
    expect(await highlighted(code, 'go')).toContain('class="shiki')
    expect(performance.now() - started).toBeLessThan(2000)
  })

  it('highlights a Go snippet with both theme variables on token spans', async () => {
    const html = (await highlighted('func main() {\n\tfmt.Println("hi")\n}\n', 'go'))!
    expect(html).toContain('class="shiki')
    expect(html).toMatch(/<span style="[^"]*--shiki-light:#[0-9a-f]{6}[^"]*--shiki-dark:#[0-9a-f]{6}/i)
    expect(html).toContain('--shiki-light-bg')
    expect(html).toContain('--shiki-dark-bg')
  })

  it('returns null for an unknown language', async () => {
    expect(await highlighted('x', 'nonsense')).toBeNull()
    expect(await highlighted('x', '')).toBeNull()
  })

  it('does not highlight mermaid', async () => {
    expect(await highlighted('A --> B', 'mermaid')).toBeNull()
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
  ])('resolves the alias %s', async (alias, canonical) => {
    const code = 'x = 1'
    expect(await highlighted(code, alias)).toBe(await highlighted(code, canonical))
    expect(await highlighted(code, alias)).not.toBeNull()
  })

  it('escapes markup in the code', async () => {
    const html = (await highlighted('<script>alert(1)</script>', 'html'))!
    expect(html).not.toContain('<script>')
    expect(html).toContain('&#x3C;')
  })
})
