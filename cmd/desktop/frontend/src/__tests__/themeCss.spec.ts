import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { themes } from '../composables/useTheme'

// The theme registry (useTheme.ts) and the palette blocks (main.css) can only
// drift apart silently: a registered id without a block renders as the :root
// palette, and a block without an id is dead weight. Parity is asserted here
// instead of discovered in the picker.

const css = readFileSync(join(process.cwd(), 'src/styles/main.css'), 'utf8')

const blocks = new Map<string, Set<string>>()
for (const match of css.matchAll(/\[data-theme="([a-z-]+)"\]\s*\{([^}]*)\}/g)) {
  const tokens = new Set([...match[2].matchAll(/--hv-[a-z0-9-]+/g)].map((m) => m[0]))
  blocks.set(match[1], tokens)
}

function value(theme: string, token: string): string | undefined {
  const block = css.match(new RegExp(`\\[data-theme="${theme}"\\]\\s*\\{([^}]*)\\}`))
  return block?.[1].match(new RegExp(`${token}:\\s*(#[0-9a-f]{6})`, 'i'))?.[1]
}

function luminance(hex: string): number {
  const channels = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
  const [r, g, b] = channels.map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

describe('theme CSS', () => {
  it('defines a block for every registered theme and registers every block', () => {
    expect([...blocks.keys()].sort()).toEqual([...themes].sort())
  })

  it('defines the full per-theme token set in every block', () => {
    // The light block is the canonical per-theme set: unlike dark it defines
    // nothing that themes inherit from :root (kind-*, severity-*, node hues).
    const required = [...blocks.get('light')!]
    expect(required.length).toBeGreaterThan(40)

    for (const [theme, tokens] of blocks) {
      // Midnight predates the per-theme ANSI palettes and shares :root's by
      // design (it is the same hue family as dark).
      const exempt = theme === 'midnight' ? /^--hv-term-/ : /^$/
      const missing = required.filter((token) => !tokens.has(token) && !exempt.test(token))
      expect(missing, `theme "${theme}"`).toEqual([])
    }
  })

  // A dark theme that leaves unstyled output on --hv-text lands near 16:1,
  // which is glare across a full screen of monospace — the drift #181 ended up
  // being about. The ceiling only applies to dark themes: dark-on-light wants
  // all the contrast it can get, which is why black on white is the norm.
  it('keeps terminal foreground contrast in the legible band for every theme', () => {
    for (const theme of blocks.keys()) {
      const background = value(theme, '--hv-app')
      // No fallback to --hv-text here on purpose. :root defines
      // --hv-term-foreground, so a theme that omits it inherits the *dark*
      // value in the browser rather than falling through — which is how the
      // light themes shipped a 1.7:1 grey on white while this test passed.
      const foreground = value(theme, '--hv-term-foreground')
      expect(background, `theme "${theme}" --hv-app`).toBeDefined()
      expect(foreground, `theme "${theme}" foreground`).toBeDefined()

      const ratio = contrast(foreground!, background!)
      expect(ratio, `theme "${theme}" is below AAA`).toBeGreaterThanOrEqual(7)
      if (luminance(background!) < 0.5) {
        expect(ratio, `theme "${theme}" is glare-bright for a terminal`).toBeLessThanOrEqual(13)
      }
    }
  })
})
