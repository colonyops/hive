import { describe, expect, it } from 'vitest'
import { colorForBranch, DEV_BAR_COLORS, hashBranch, readableTextColor } from '../devBarColor'

describe('devBarColor', () => {
  it('is deterministic — the same branch always maps to the same color', () => {
    expect(colorForBranch('main')).toBe(colorForBranch('main'))
    expect(hashBranch('feat/foo')).toBe(hashBranch('feat/foo'))
  })

  it('always returns a color from the palette', () => {
    for (const branch of ['main', 'feat/slack-titlebar', 'hay-kot/fix', 'unknown', '', 'x']) {
      expect(DEV_BAR_COLORS).toContain(colorForBranch(branch))
    }
  })

  it('spreads distinct branches across multiple colors', () => {
    const branches = Array.from({ length: 60 }, (_, i) => `feat/branch-${i}`)
    const distinct = new Set(branches.map(colorForBranch))
    // A good hash should light up most of the palette across 60 names.
    expect(distinct.size).toBeGreaterThan(DEV_BAR_COLORS.length / 2)
  })

  it('picks a readable text color by luminance', () => {
    expect(readableTextColor('#a3e635')).toBe('#0b0b0c') // bright lime → dark text
    expect(readableTextColor('#1e3a8a')).toBe('#ffffff') // deep blue → light text
    expect(readableTextColor('not-a-hex')).toBe('#0b0b0c') // safe fallback
  })

  it('every palette color is bright enough for dark text', () => {
    // The palette is intentionally all-bright, so the bar reads like the
    // original yellow strip; if a darker color is ever added, DevBar still
    // stays legible because readableTextColor would flip it to white.
    for (const color of DEV_BAR_COLORS) {
      expect(readableTextColor(color)).toBe('#0b0b0c')
    }
  })
})
