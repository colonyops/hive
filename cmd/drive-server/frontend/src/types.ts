// The wire types live in wire.gen.ts, generated from internal/platform/driveapi/wire.go
// by `mise run generate:tygo`. This file keeps the names the UI imports.
import type { CreateToken, Document } from './wire.gen'

export type {
  Anchor,
  Author,
  Comment,
  CreatedToken,
  DocumentKind,
  LineRange,
  Me,
  PresenceSnapshot,
  PresenceViewer,
  PublicDocument,
  SearchHit,
  SearchResults,
  Share,
  ShareExpiry,
  SpaceSummary,
  StreamHello,
  Thread,
  ThreadList,
  Token,
  TokenScope,
  Viewer,
} from './wire.gen'

// Renamed so it does not shadow the DOM's Document.
export type DriveDocument = Document
export type CreateTokenRequest = CreateToken
