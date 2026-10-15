import type { DriveDocument } from '../types'

export function doc(overrides: Partial<DriveDocument> & Pick<DriveDocument, 'path'>): DriveDocument {
  return {
    id: overrides.path,
    space: 'acme/notes',
    kind: 'markdown',
    sha256: 'abc',
    size: 2048,
    updated_at: '2026-10-15T12:00:00Z',
    ...overrides,
  }
}

// happy-dom would fetch an iframe's src for real, so specs read the src from a stand-in.
export const frameStub = {
  props: ['src', 'html', 'testid'],
  template: '<div data-testid="frame" :data-src="src" />',
}
