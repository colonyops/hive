import { watch } from 'vue'
import { useNotify, type NotifyEvent } from '../../composables/useNotify'

/**
 * Notifies once when a server enters a state, not on every poll that finds it
 * there. A server that recovers and fails again notifies again.
 */
export function useServerConnectionNotices(
  servers: () => readonly { name: string; state: string; error: string }[],
  listError: () => string | null,
  notify: (event: NotifyEvent) => Promise<void> = useNotify().notify,
): void {
  let notified = new Map<string, string>()
  watch(servers, (current) => {
    for (const server of current) {
      if (notified.get(server.name) === server.state) continue
      void notify({ title: `${server.name} is ${server.state}`, body: server.error, severity: 'error' })
    }
    notified = new Map(current.map((server) => [server.name, server.state]))
  })
  watch(listError, (error) => {
    if (error) void notify({ title: 'Could not reach your servers', body: error, severity: 'error' })
  })
}
