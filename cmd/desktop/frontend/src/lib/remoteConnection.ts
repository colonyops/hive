import { endpointFetch } from './managedTransport'
import { TERMINAL_WIRE_VERSION, type TerminalEndpoint } from './terminalClient'

export interface RemoteSession {
  id: string
  name: string
  slug: string
  repo: string
  state: string
}

export function validateRemoteHost(host: string): string {
  const value = host.trim()
  if (!/^[a-zA-Z0-9][a-zA-Z0-9_.@:-]*$/.test(value))
    throw new Error('Enter an SSH host or config alias, optionally with user@.')
  return value
}

export async function listRemoteSessions(endpoint: TerminalEndpoint): Promise<RemoteSession[]> {
  const response = await endpointFetch(endpoint, `/api/terminal/remote/sessions`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${endpoint.token}` },
    redirect: 'error',
    signal: AbortSignal.timeout(10000),
  })
  if (response.status === 401)
    throw new Error('The remote token was rejected. Retrieve a fresh token after restarting the remote installation.')
  if (!response.ok)
    throw new Error(
      `Remote Hive could not list sessions (HTTP ${response.status}). Check the tunnel and remote terminal support.`,
    )
  const body = (await response.json()) as {
    protocolVersion: number
    terminalWireVersion: string
    sessions: RemoteSession[]
  }
  if (body.protocolVersion !== 1 || body.terminalWireVersion !== TERMINAL_WIRE_VERSION) {
    throw new Error(
      'This remote Hive uses an incompatible protocol. Update the client and remote installation together.',
    )
  }
  if (!Array.isArray(body.sessions)) throw new Error('Remote Hive returned an invalid session list.')
  return body.sessions
}
