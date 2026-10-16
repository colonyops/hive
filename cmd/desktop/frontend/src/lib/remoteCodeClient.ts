import type { SessionLaunchOptions } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import type { SessionStatusSnapshot } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { endpointFetch } from './managedTransport'
import type { TerminalEndpoint } from './terminalClient'

export interface RemoteCodeInput {
  repository: string
  name: string
  prompt: string
  agent: string
}

export function createRemoteCodeClient(endpoint: TerminalEndpoint) {
  async function post<T>(path: string, body = {}): Promise<T> {
    const response = await endpointFetch(endpoint, `/api/terminal/remote/${path}`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${endpoint.token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      redirect: 'error',
      signal: AbortSignal.timeout(30000),
    })
    if (!response.ok) {
      const detail = (await response.json()) as { message?: string }
      throw new Error(detail.message || `Remote Hive request failed (HTTP ${response.status}).`)
    }
    return (await response.json()) as T
  }
  return {
    options: () => post<SessionLaunchOptions>('session-options'),
    statuses: () => post<SessionStatusSnapshot>('session-statuses'),
    create: (input: RemoteCodeInput) => post<{ jobId: number }>('create-session', input),
    job: (id: number) => post<{ status: string; error?: string }>('job', { id }),
  }
}
