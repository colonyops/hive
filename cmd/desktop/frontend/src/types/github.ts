// Wire types re-exported from the generated Wails bindings, mirroring the
// types/feed.ts seam: UI code imports from here, never from bindings/.
export type {
  DeviceFlowInfo,
  ConnectionStatus,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/models'

// Connected/disconnected, not authenticated/unauthenticated: GitHub is one
// connector among several and nothing in the app is gated on holding a
// credential for it.
export type ConnectionState = 'disconnected' | 'connected'
