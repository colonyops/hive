import { Open } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/popoutservice'
import { errorText } from '../lib/appError'
import { documentsOf, driveScopeQuery, type DriveScope } from '../lib/drive'
import { useDrive } from '../stores/useDrive'
import { useDriveActivity } from '../stores/useDriveActivity'
import { useToasts } from './useToasts'

async function openPopOut(kind: string, params: Record<string, string>): Promise<void> {
  try {
    const result = await Open({ kind, params })
    // The server build has no native windows, so the browser opens the tab.
    if (!result.opened) window.open(result.url, '_blank')
  } catch (error) {
    useToasts().showToast(errorText(error, 'Could not open the window.'), { severity: 'error' })
  }
}

export function usePopOut(): (scope: DriveScope) => void {
  const activity = useDriveActivity()
  const drive = useDrive()

  // A surface with no path shows the session's last write, which only this
  // window saw happen: a new window starts with no writes and would open the
  // newest document instead, so the write travels as a pinned path.
  function pinned(scope: DriveScope): DriveScope {
    if (scope.path) return scope
    const own = activity.lastWrite(scope.session, scope.space)
    if (!own) return scope
    const tree = drive.trees.value.get(scope.space)
    if (tree && !documentsOf(tree).some((entry) => entry.path === own)) return scope
    return { ...scope, path: own }
  }

  return (scope) => void openPopOut('drive', driveScopeQuery(pinned(scope)))
}
