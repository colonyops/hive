import { describe, expect, it } from 'vitest'
import { byType, instantiate, palette } from '../registry'

describe('byType', () => {
  it('discovers exactly the node types with app modules (index.ts)', () => {
    expect(Object.keys(byType).sort()).toEqual(['action', 'feed', 'function', 'github-filter', 'notify', 'sources.exec', 'sources.gitea', 'sources.github', 'sources.grafana_alerts', 'sources.grafana_irm_alerts', 'sources.grafana_metrics', 'sources.posthog_alerts', 'sources.posthog_errors', 'sources.rss', 'sources.webhook'])
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

    expect(palette.Sources.map((d) => d.type).sort()).toEqual(['sources.exec', 'sources.gitea', 'sources.github', 'sources.grafana_alerts', 'sources.grafana_irm_alerts', 'sources.grafana_metrics', 'sources.posthog_alerts', 'sources.posthog_errors', 'sources.rss', 'sources.webhook'])
    expect(palette.Process.map((d) => d.type).sort()).toEqual(['function', 'github-filter'])
    expect(palette.Destinations.map((d) => d.type).sort()).toEqual(['action', 'feed', 'notify'])
  })
})

describe('instantiate', () => {
  it('builds a FlowNode with a generated id, the requested type, and a deep clone of defaults', () => {
    const node = instantiate('feed')
    expect(node.type).toBe('feed')
    expect(node.id).toMatch(/^feed-\d+$/)
    expect(node.config).toEqual(byType['feed']!.defaults)
  })

  // Go rejects a node id that is not `^[a-z0-9][a-z0-9-]*$` when the flow is
  // deployed, so a generated id that embeds a namespaced type verbatim makes
  // the node unsaveable. Asserted over every registered type, since the types
  // carrying a dot are exactly the ones no other test instantiates.
  it('generates an id Go accepts as a slug for every registered type', () => {
    for (const type of Object.keys(byType)) {
      expect(instantiate(type).id).toMatch(/^[a-z0-9][a-z0-9-]*$/)
    }
  })

  it('two instances of the same type never share config (deep clone, not reference)', () => {
    const a = instantiate('github-filter')
    const b = instantiate('github-filter')
    expect(a.id).not.toBe(b.id)
    ;(a.config as Record<string, any>).repos = ['acme/*']
    expect(b.config).not.toHaveProperty('repos')
    expect(byType['github-filter']!.defaults).not.toHaveProperty('repos')
  })

  it('seeds per-instance config from freshConfig, leaving defaults untouched', () => {
    const a = instantiate('sources.webhook').config as Record<string, any>
    const b = instantiate('sources.webhook').config as Record<string, any>
    expect(a.path).toMatch(/^hook-[a-z0-9]{8}$/)
    expect(a.path).not.toBe(b.path)
    expect(byType['sources.webhook']!.defaults).toEqual({ path: '' })
  })

  it('throws for an unknown type', () => {
    expect(() => instantiate('nope')).toThrow(/unknown node type/)
  })
})
