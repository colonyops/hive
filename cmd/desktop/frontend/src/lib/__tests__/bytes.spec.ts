import { describe, expect, it } from 'vitest'
import { formatBytes } from '../bytes'

describe('formatBytes', () => {
  it('scales to the largest unit that leaves a readable number', () => {
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(2048)).toBe('2.00 KB')
    expect(formatBytes(1024 * 1024 * 12.5)).toBe('12.5 MB')
    expect(formatBytes(1024 * 1024 * 214)).toBe('214 MB')
    expect(formatBytes(1024 ** 3 * 3)).toBe('3.00 GB')
  })

  it('reads nothing as zero rather than as NaN', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(-1)).toBe('0 B')
    expect(formatBytes(Number.NaN)).toBe('0 B')
  })
})
