// The palette's scopes, declared once (Registry pattern): metadata only, no
// dispatch, no self-registration. useCommands.ts owns dispatch; components own
// the tab strip.

/** Which tab the palette is on. */
export type PaletteScopeId = 'all' | 'goto' | 'actions' | 'shell' | 'keys'

/** The scopes a Command row can belong to. Rows default to 'actions'. */
export type CommandScope = 'goto' | 'actions'

export interface PaletteScopeSpec {
  id: PaletteScopeId
  /** Tab label, e.g. "Go to". */
  label: string
  /** First-character sigil that enters the scope; null for All. */
  sigil: '@' | '>' | '!' | '?' | null
  /** Input placeholder while the scope is active. */
  placeholder: string
}

/** Presentation order of the tab strip. */
export const paletteScopes: readonly PaletteScopeSpec[] = [
  { id: 'all', label: 'All', sigil: null, placeholder: 'Search or run a command…' },
  { id: 'goto', label: 'Go to', sigil: '@', placeholder: 'Go to…' },
  { id: 'actions', label: 'Actions', sigil: '>', placeholder: 'Run a command…' },
  { id: 'shell', label: 'Shell', sigil: '!', placeholder: 'Run in a new window…' },
  { id: 'keys', label: 'Keys', sigil: '?', placeholder: 'Search shortcuts…' },
]

/** The scope a sigil character enters, or null for a plain character. */
export function scopeForSigil(char: string): PaletteScopeId | null {
  return paletteScopes.find((s) => s.sigil === char)?.id ?? null
}
