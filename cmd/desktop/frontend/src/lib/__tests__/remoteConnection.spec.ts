import { afterEach, describe, expect, it, vi } from 'vitest'
import { listRemoteSessions, validateRemoteHost } from '../remoteConnection'
import { connectManaged } from '../managedTransport'
import {
  Connect,
  Request,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/remoteservice'
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/remoteservice', () => ({
  Connect: vi.fn(() => Promise.resolve('opaque-connection')),
  Request: vi.fn(),
}))
afterEach(() => vi.unstubAllGlobals())
describe('remote connection', () => {
  it.each(['-oProxyCommand=bad', 'http://example.com', 'host;command', 'host\ncommand'])(
    'rejects invalid SSH destinations: %s',
    (host) => {
      expect(() => validateRemoteHost(host)).toThrow()
    },
  )
  it('passes SSH configuration to Go and receives no credentials', async () => {
    const endpoint = await connectManaged('hive-server', '~/connection.json')
    expect(Connect).toHaveBeenCalledWith({ host: 'hive-server', connectionFile: '~/connection.json' })
    expect(endpoint).toEqual({ httpBaseURL: 'hive-remote://opaque-connection', wsURL: '', token: '' })
    vi.mocked(Request).mockResolvedValue({
      status: 200,
      body: JSON.stringify({ protocolVersion: 1, terminalWireVersion: '2', sessions: [{ id: 'one' }] }),
    })
    const fetcher = vi.fn()
    vi.stubGlobal('fetch', fetcher)
    expect(await listRemoteSessions(endpoint)).toHaveLength(1)
    expect(fetcher).not.toHaveBeenCalled()
    expect(Request).toHaveBeenCalledWith('opaque-connection', '/api/terminal/remote/sessions', '{}')
  })
  it('rejects protocol mismatches before attachment', async () => {
    const endpoint = await connectManaged('hive-server', '~/connection.json')
    vi.mocked(Request).mockResolvedValue({
      status: 200,
      body: JSON.stringify({ protocolVersion: 2, terminalWireVersion: '2', sessions: [] }),
    })
    await expect(listRemoteSessions(endpoint)).rejects.toThrow('incompatible protocol')
  })
})
