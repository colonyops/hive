import { endpointFetch } from './managedTransport'
import type { TerminalEndpoint } from './terminalClient'

export interface RemoteInbox {
  id: string
  name: string
  feeds: { id: string; name: string }[]
  problem: string
}
export interface RemoteInboxItem {
  id: number
  title: string
  sourceKind: string
  sourceState: string
  unread: boolean
}
export function createRemoteInboxClient(endpoint: TerminalEndpoint) {
  async function post<T>(path: string, body = {}): Promise<T> {
    const response = await endpointFetch(endpoint, `/api/terminal/remote/${path}`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${endpoint.token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      redirect: 'error',
      signal: AbortSignal.timeout(10000),
    })
    if (!response.ok)
      throw new Error(
        `Could not read remote inbox (HTTP ${response.status}). Check the tunnel and remote Hive version.`,
      )
    return (await response.json()) as T
  }
  return {
    async inboxes(): Promise<RemoteInbox[]> {
      return (await post<{ inboxes: RemoteInbox[] }>('inboxes')).inboxes
    },
    async feed(profileId: string, feedId: string): Promise<RemoteInboxItem[]> {
      return (await post<{ items: RemoteInboxItem[] }>('feed', { profileId, feedId })).items
    },
  }
}
