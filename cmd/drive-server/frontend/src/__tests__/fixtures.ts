import type { DriveDocument } from '../types'

export function doc(overrides: Partial<DriveDocument> & Pick<DriveDocument, 'path'>): DriveDocument {
  return {
    id: overrides.path,
    space: 'acme/notes',
    title: overrides.path.slice(overrides.path.lastIndexOf('/') + 1).replace(/\.[^.]+$/, ''),
    kind: 'markdown',
    sha256: 'abc',
    size: 2048,
    updated_at: '2026-10-15T12:00:00Z',
    ...overrides,
  }
}
