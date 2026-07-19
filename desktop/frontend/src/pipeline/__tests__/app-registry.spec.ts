import { describe, expect, it } from 'vitest'
import { byType, instantiate, palette } from '../registry'

describe('byType', () => {
  it('discovers exactly the five node types with app modules (index.ts)', () => {
    expect(Object.keys(byType).sort()).toEqual(['action', 'feed', 'function', 'github-filter', 'github-source'])
  })

  it('every entry carries a `type` matching its registry key and has a glyph/editor/help/defaults', () => {
    for (const [key, def] of Object.entries(byType)) {
      expect(def.type).toBe(key)
      expect(def.label).toEqual(expect.any(String))
      expect(def.glyph).toBeTruthy()
      expect(def.editor).toBeTruthy()
      expect(def.help).toEqual(expect.any(String))
      expect(def.help.length).toBeGreaterThan(0)
      expect(def.defaults).toBeTruthy()
    }
  })

  it('entries are frozen (defineNodeType)', () => {
    expect(Object.isFrozen(byType['function'])).toBe(true)
  })
})

describe('palette', () => {
  it('groups every registered type into its declared category', () => {
    const grouped = [...palette.Sources, ...palette.Process, ...palette.Destinations]
    expect(grouped.map((def) => def.type).sort()).toEqual(Object.keys(byType).sort())

    expect(palette.Sources.map((d) => d.type)).toEqual(['github-source'])
    expect(palette.Process.map((d) => d.type).sort()).toEqual(['function', 'github-filter'])
    expect(palette.Destinations.map((d) => d.type).sort()).toEqual(['action', 'feed'])
  })
})

describe('instantiate', () => {
  it('builds a FlowNode with a generated id, the requested type, and a deep clone of defaults', () => {
    const node = instantiate('feed')
    expect(node.type).toBe('feed')
    expect(node.id).toMatch(/^feed-\d+$/)
    expect(node.config).toEqual(byType['feed']!.defaults)
  })

  it('two instances of the same type never share config (deep clone, not reference)', () => {
    const a = instantiate('github-filter')
    const b = instantiate('github-filter')
    expect(a.id).not.toBe(b.id)
    ;(a.config as Record<string, any>).repos = ['acme/*']
    expect(b.config).not.toHaveProperty('repos')
    expect(byType['github-filter']!.defaults).not.toHaveProperty('repos')
  })

  it('throws for an unknown type', () => {
    expect(() => instantiate('nope')).toThrow(/unknown node type/)
  })
})
