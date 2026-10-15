// Mirrors internal/platform/driveapi/wire.go.
export type DocumentKind = 'markdown' | 'html'
export type TokenScope = 'read' | 'push'

export interface Share {
  url: string
  expires_at?: string
  /** The preset behind expires_at; empty for no expiry. */
  expires_in: ShareExpiry
}

export interface DriveDocument {
  id: string
  space: string
  path: string
  title: string
  kind: DocumentKind
  sha256: string
  size: number
  updated_at: string
  shared?: Share
}

export interface SearchHit {
  document: DriveDocument
  snippet: string
}

export interface SearchResults {
  hits: SearchHit[]
}

export interface SpaceSummary {
  space: string
  kind: 'repository' | 'workspace' | 'shared'
  documents: number
  bytes: number
}

export interface PublicDocument {
  owner_email: string
  document: DriveDocument
  body?: string
}

export interface Me {
  email: string
  policy: string
  quota: { used: number; limit: number | null }
}

export interface Token {
  id: string
  name: string
  scope: TokenScope
  expires_at?: string
  last_used_at?: string
  created_at: string
}

export interface CreatedToken extends Token {
  secret: string
}

export interface CreateTokenRequest {
  name: string
  scope: TokenScope
  expires_in: string
}

export type ShareExpiry = '' | '1d' | '7d' | '30d'
