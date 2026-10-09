import { TERMINAL_WIRE_VERSION, type TerminalEndpoint } from './terminalClient'

export interface RemoteSession {
  id: string
  name: string
  slug: string
  repo: string
  state: string
}

export function remoteEndpoint(address: string, token: string): TerminalEndpoint {
  const url = new URL(address.trim())
  if (
    url.protocol !== 'http:' ||
    !['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname) ||
    url.username ||
    url.password ||
    url.search ||
    url.hash ||
    url.pathname !== '/'
  )
    throw new Error('Use the local HTTP address of your SSH tunnel, such as http://127.0.0.1:19001.')
  if (!token.trim()) throw new Error('Enter the remote installation’s connection token.')
  return {
    httpBaseURL: url.origin,
    wsURL: `${url.origin.replace('http:', 'ws:')}/api/terminal/stream`,
    token: token.trim(),
  }
}

export async function listRemoteSessions(endpoint: TerminalEndpoint): Promise<RemoteSession[]> {
  const response = await fetch(`${endpoint.httpBaseURL}/api/terminal/remote/sessions`, {
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
