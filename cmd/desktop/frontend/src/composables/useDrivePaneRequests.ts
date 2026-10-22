import { nextTick } from 'vue'
import { AcknowledgePane } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/driveservice'
import { driveEventAuthor } from '../lib/drive'
import { createLogger } from '../lib/log'
import { useWailsEvent } from './useWailsEvent'

const log = createLogger('drive-pane-requests')

interface PaneRequest {
  requestId: string
  session?: string
  hiveSession?: string
  path?: string
  open: boolean
}

// App owns this listener so a request gets an answer even before Code or Chats mounts.
export function useDrivePaneRequests(
  currentSession: () => string | null,
  navigate: (open: boolean, path?: string) => Promise<void>,
) {
  async function handle(request: PaneRequest): Promise<void> {
    let status = 'session_not_visible'
    const author = driveEventAuthor(request)
    if (author && author === currentSession()) {
      try {
        await navigate(request.open, request.path || undefined)
        await nextTick()
        const visible = [...document.querySelectorAll('[data-testid="drive-pane"]')].some(
          (pane) => pane.getBoundingClientRect().width > 0,
        )
        status =
          author !== currentSession()
            ? 'session_not_visible'
            : visible === request.open
              ? request.open
                ? 'opened'
                : 'closed'
              : 'navigation_failed'
      } catch (error) {
        log.warn('Could not change Drive pane visibility', error)
        status = 'navigation_failed'
      }
    }
    if (request.requestId) {
      try {
        await AcknowledgePane(request.requestId, status)
      } catch (error) {
        log.warn('Could not acknowledge Drive pane request', error)
      }
    }
  }

  useWailsEvent('drive:toggle', (event) => {
    const request = (Array.isArray(event.data) ? event.data[0] : event.data) as PaneRequest | undefined
    if (request) void handle(request)
  })
}
