import { describe, expect, it } from 'vitest'
import { generatedName, presenceColors, useViewerIdentity } from '../lib/identity'

describe('generatedName', () => {
  it('stays the same for one seed', () => {
    expect(generatedName('p_abc')).toBe(generatedName('p_abc'))
    expect(generatedName('p_abc')).toMatch(/^[A-Z][a-z]+ [A-Z][a-z]+$/)
  })
})

describe('useViewerIdentity', () => {
  it('keeps the name across visits and falls back to a generated one', () => {
    const first = useViewerIdentity()
    expect(first.name.value).toMatch(/^[A-Z][a-z]+ [A-Z][a-z]+$/)
    expect(first.color).toBeGreaterThanOrEqual(0)
    expect(first.color).toBeLessThan(presenceColors)

    first.rename('  Ada  ')
    expect(useViewerIdentity().name.value).toBe('Ada')

    first.rename('')
    expect(useViewerIdentity().name.value).toMatch(/^[A-Z][a-z]+ [A-Z][a-z]+$/)
  })
})
