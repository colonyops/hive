import { Open } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/popoutservice'
import { errorText } from '../lib/appError'
import { driveScopeQuery, type DriveScope } from '../lib/drive'
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

export function popOutDrive(scope: DriveScope): void {
  void openPopOut('drive', driveScopeQuery(scope))
}
