const units = ['B', 'KB', 'MB', 'GB', 'TB']

// Byte sizes at a glance: three significant figures at most, so a column of
// them stays the same width and a change of a few hundred kilobytes in a
// hundred-megabyte number does not jitter the digit that carries the meaning.
export function formatBytesParts(bytes: number): { value: string, unit: string } {
  if (!Number.isFinite(bytes) || bytes <= 0) return { value: '0', unit: 'B' }
  const index = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)))
  const value = bytes / 1024 ** index
  const decimals = index === 0 ? 0 : value >= 100 ? 0 : value >= 10 ? 1 : 2
  return { value: value.toFixed(decimals), unit: units[index] }
}

export function formatBytes(bytes: number): string {
  const { value, unit } = formatBytesParts(bytes)
  return `${value} ${unit}`
}
