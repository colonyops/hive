// Wire types re-exported from the generated Wails bindings, mirroring the
// types/feed.ts seam: UI code imports from here, never from bindings/.
export type { DeviceFlowInfo, Status as AuthStatus } from '../../bindings/github.com/colonyops/hive/internal/desktop/auth/models'

export type AuthState = 'unauthenticated' | 'authenticated'
