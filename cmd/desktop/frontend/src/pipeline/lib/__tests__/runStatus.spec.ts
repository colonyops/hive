import { describe, expect, it } from 'vitest'
import { ageLabel } from '../runStatus'

describe('ageLabel', () => {
  const now = 1_789_666_612_465

  it('formats Unix millisecond ages', () => {
    expect(ageLabel(now - 500, now)).toBe('just now')
    expect(ageLabel(now - 30_000, now)).toBe('30s ago')
    expect(ageLabel(now - 5 * 60_000, now)).toBe('5m ago')
    expect(ageLabel(now - 2 * 60 * 60_000, now)).toBe('2h ago')
  })
})
