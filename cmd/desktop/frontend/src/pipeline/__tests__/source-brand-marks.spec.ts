// A source node stands for a real product, so it wears that product's mark
// and hue rather than a lucide approximation. These assert the flow editor
// reuses the marks the app already ships (the same components the Integrations
// screen and the inbox source badge render) instead of growing a second,
// drifting set.
import { describe, expect, it } from 'vitest'
import { byType } from '../registry'
import { sourceKindForNodeType } from '../../lib/itemPresentation'
import GithubMark from '../../components/marks/GithubMark.vue'
import GiteaMark from '../../components/marks/GiteaMark.vue'
import GrafanaMark from '../../components/marks/GrafanaMark.vue'
import PostHogMark from '../../components/marks/PostHogMark.vue'

/** sourceKind -> the brand mark and accent every node of that kind renders. */
const BRANDS = {
  github: { mark: GithubMark, accent: 'var(--color-brand-github)', tint: 'var(--color-brand-github-tint)' },
  gitea: { mark: GiteaMark, accent: 'var(--color-brand-gitea)', tint: 'var(--color-brand-gitea-tint)' },
  grafana: { mark: GrafanaMark, accent: 'var(--color-brand-grafana)', tint: 'var(--color-brand-grafana-tint)' },
  posthog: { mark: PostHogMark, accent: 'var(--color-brand-posthog)', tint: 'var(--color-brand-posthog-tint)' },
} as const

const sourceTypes = Object.values(byType).filter((def) => def.role === 'source')

describe('source brand marks', () => {
  it('covers every source node type — each is either a known brand or a deliberate generic', () => {
    const generic = ['sources.exec', 'sources.rss', 'sources.webhook']
    for (const def of sourceTypes) {
      const kind = sourceKindForNodeType(def.type)
      expect(kind, `${def.type} has no sourceKind`).toBeTruthy()
      expect(kind! in BRANDS || generic.includes(def.type), `${def.type} is neither branded nor a declared generic`).toBe(true)
    }
  })

  it('gives every branded source its product mark and brand hue', () => {
    for (const def of sourceTypes) {
      const brand = BRANDS[sourceKindForNodeType(def.type) as keyof typeof BRANDS]
      if (!brand) continue
      expect(def.glyph, `${def.type} does not use its product mark`).toBe(brand.mark)
      expect(def.accentToken, `${def.type} does not use its brand hue`).toBe(brand.accent)
      expect(def.tint, `${def.type} does not use its brand tint`).toBe(brand.tint)
    }
  })

  // Grafana ships three source types and PostHog two; a brand that resolved to
  // two different hues would read as two different products on one canvas.
  it('renders one identity per product, however many node types it has', () => {
    const byBrand = new Map<string, Set<string>>()
    for (const def of sourceTypes) {
      const kind = sourceKindForNodeType(def.type)!
      if (!(kind in BRANDS)) continue
      byBrand.set(kind, (byBrand.get(kind) ?? new Set()).add(`${def.accentToken}|${def.tint}`))
    }
    expect([...byBrand].filter(([, hues]) => hues.size > 1)).toEqual([])
    expect(byBrand.get('grafana')?.size).toBe(1)
    expect(byBrand.get('posthog')?.size).toBe(1)
  })

  // The protocol sources have no vendor behind them, so they keep the generic
  // hue rather than borrowing a brand's.
  it('leaves the unbranded sources on the generic source hue', () => {
    for (const type of ['sources.exec', 'sources.rss', 'sources.webhook']) {
      expect(byType[type]!.accentToken).toBe('var(--color-node-blue)')
    }
  })

  // Sizing follows the mark, not the role: a branded source draws its logo
  // full-bleed, while the unbranded ones keep the lucide optical size that
  // lines them up with the downstream nodes.
  it('marks a branded source as a logomark and leaves the generics as glyphs', () => {
    for (const def of sourceTypes) {
      const branded = sourceKindForNodeType(def.type)! in BRANDS
      expect(def.logoMark === true, `${def.type} logoMark should be ${branded}`).toBe(branded)
    }
  })

  it('never gives two different products the same hue', () => {
    const accents = Object.values(BRANDS).map((b) => b.accent)
    expect(new Set(accents).size).toBe(accents.length)
    expect(accents).not.toContain('var(--color-node-blue)')
  })
})
