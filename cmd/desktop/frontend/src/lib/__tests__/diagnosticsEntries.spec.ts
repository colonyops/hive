import { describe, expect, it } from 'vitest'
import type { Entry as DiagnosticEntry } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/diagnostics/models'
import { retainUnchangedDiagnosticEntries } from '../diagnosticsEntries'

function entry(overrides: Partial<DiagnosticEntry> = {}): DiagnosticEntry {
  return {
    id: 'desktop-1',
    time: '2026-10-09T12:00:00Z',
    source: 'desktop',
    level: 'debug',
    message: 'request complete',
    fields: { method: 'GET', status: '200' },
    raw: '{"message":"request complete"}',
    truncated: false,
    ...overrides,
  }
}

describe('retainUnchangedDiagnosticEntries', () => {
  it('keeps entry and array identity when a refresh returns the same evidence', () => {
    const previous = [entry()]
    const result = retainUnchangedDiagnosticEntries(previous, [entry({ fields: { method: 'GET', status: '200' } })])

    expect(result).toBe(previous)
    expect(result[0]).toBe(previous[0])
  })

  it('replaces only entries whose rendered evidence changed', () => {
    const unchanged = entry()
    const changed = entry({ id: 'desktop-2', message: 'before' })
    const replacement = entry({ id: 'desktop-2', message: 'after' })
    const previous = [unchanged, changed]

    const result = retainUnchangedDiagnosticEntries(previous, [entry(), replacement])

    expect(result).not.toBe(previous)
    expect(result[0]).toBe(unchanged)
    expect(result[1]).toBe(replacement)
  })
})
