import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { computed } from 'vue'
import { fakePlatform } from '../../test-utils/platform'
import { cacheLimit, createCodeHighlight, type CodeHighlight } from '../highlight'

let current: CodeHighlight
const useCodeHighlight = () => current

/** A render asks, the queue drains, the next render finds the fence. */
async function highlighted(code: string, lang: string): Promise<string | null> {
  const store = useCodeHighlight()
  store.highlight(code, lang)
  await store.settle()
  return store.highlight(code, lang)
}

describe('highlight', () => {
  beforeEach(() => {
    current = createCodeHighlight(fakePlatform)
    return current.load()
  })

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
    expect(await highlighted(code, 'go')).toContain('class="shiki')
  }, 2000)

  it('highlights a Go snippet in the app theme, with one variable per token', async () => {
    const html = (await highlighted('func main() {\n\tfmt.Println("hi")\n}\n', 'go'))!
    expect(html).toContain('class="shiki')
    expect(html).toMatch(/<span style="--shiki-app:#f5b23f">func<\/span>/i)
    expect(html).not.toContain('--shiki-dark')
  })

  it('re-highlights in the new code theme when the app theme changes, and keeps the old one cached', async () => {
    const { theme } = fakePlatform
    const code = 'func main() {}\n'
    const hive = (await highlighted(code, 'go'))!
    theme.value = 'tokyo-night'
    try {
      const { highlight, settle } = useCodeHighlight()
      // The theme is not loaded yet, so the fence is not even queued.
      expect(highlight(code, 'go')).toBeNull()
      await settle()
      const night = (await highlighted(code, 'go'))!
      expect(night).toContain('--shiki-app:')
      expect(night).not.toBe(hive)
      expect(night).not.toMatch(/#f5b23f/i)
      theme.value = 'dark'
      expect(highlight(code, 'go')).toBe(hive)
    } finally {
      theme.value = 'dark'
    }
  })

  it('re-runs a computed that asked while the new theme was still loading', async () => {
    const { theme } = fakePlatform
    const { highlight, settle } = useCodeHighlight()
    const code = 'func themed() {}\n'
    await highlighted(code, 'go')
    const runs = vi.fn()
    const html = computed(() => {
      runs()
      return highlight(code, 'go')
    })
    const rendered = () => html.value
    theme.value = 'catppuccin-latte'
    try {
      expect(rendered()).toBeNull()
      await settle()
      // The bump from the theme load re-runs it, which queues the fence.
      expect(rendered()).toBeNull()
      await settle()
      expect(rendered()).toContain('class="shiki')
      expect(runs).toHaveBeenCalledTimes(3)
    } finally {
      theme.value = 'dark'
    }
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

  it('holds every fence on screen when there are more than the cache limit', async () => {
    const { highlight, settle } = useCodeHighlight()
    const fences = Array.from({ length: cacheLimit + 1 }, (_, i) => `x := ${i}`)
    const render = () => fences.map((code) => highlight(code, 'go'))

    render()
    await settle()
    expect(render().every((html) => html?.includes('class="shiki'))).toBe(true)
  })

  it('escapes markup in the code', async () => {
    const html = (await highlighted('<script>alert(1)</script>', 'html'))!
    expect(html).not.toContain('<script>')
    expect(html).toContain('&#x3C;')
  })
})

describe('highlight with a failing engine', () => {
  async function storeWith(core: () => Promise<unknown>) {
    vi.resetModules()
    vi.doMock('shiki/core', () => ({ createHighlighterCore: core }))
    vi.doMock('shiki/engine/oniguruma', () => ({ createOnigurumaEngine: () => ({}) }))
    vi.doMock('shiki/wasm', () => ({ default: {} }))
    const { createCodeHighlight } = await import('../highlight')
    return createCodeHighlight(fakePlatform)
  }

  afterEach(() => {
    vi.doUnmock('shiki/core')
    vi.doUnmock('shiki/engine/oniguruma')
    vi.doUnmock('shiki/wasm')
  })

  it('leaves every fence unhighlighted when the engine does not load', async () => {
    const store = await storeWith(() => Promise.reject(new Error('no wasm')))
    await expect(store.load()).resolves.toBeUndefined()
    expect(store.highlight('x', 'go')).toBeNull()
  })

  it('answers null for a fence that throws and still highlights the others', async () => {
    const codeToHtml = vi.fn((code: string) => {
      if (code === 'bad') throw new Error('pathological')
      return `<pre class="shiki">${code}</pre>`
    })
    const store = await storeWith(() => Promise.resolve({ getLoadedLanguages: () => ['go'], codeToHtml }))
    await store.load()

    store.highlight('bad', 'go')
    store.highlight('good', 'go')
    await store.settle()

    expect(store.highlight('bad', 'go')).toBeNull()
    expect(store.highlight('good', 'go')).toBe('<pre class="shiki">good</pre>')
    await store.settle()
    expect(codeToHtml).toHaveBeenCalledTimes(2)
  })
})
