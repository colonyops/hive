import { afterEach, describe, expect, it, vi } from 'vitest'
import { listRemoteSessions, remoteEndpoint } from '../remoteConnection'

const endpoint = remoteEndpoint('http://127.0.0.1:19001', 'secret')
afterEach(() => vi.unstubAllGlobals())

describe('remote connection', () => {
  it.each([
    'https://127.0.0.1:1',
    'http://example.com',
    'http://user@localhost',
    'http://localhost/api',
    'http://localhost?token=x',
  ])('rejects a destination outside the SSH tunnel origin: %s', (address) => {
    expect(() => remoteEndpoint(address, 'secret')).toThrow()
  })
  it('derives the stream from the forwarded address', () => {
    expect(endpoint.wsURL).toBe('ws://127.0.0.1:19001/api/terminal/stream')
  })
  it('authenticates discovery without putting the token in the URL', async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          protocolVersion: 1,
          terminalWireVersion: '2',
          sessions: [{ id: 'one', slug: 'same-as-local' }],
        }),
      ),
    )
    vi.stubGlobal('fetch', fetcher)
    expect(await listRemoteSessions(endpoint)).toHaveLength(1)
    expect(fetcher).toHaveBeenCalledWith(
      'http://127.0.0.1:19001/api/terminal/remote/sessions',
      expect.objectContaining({ headers: { Authorization: 'Bearer secret' }, redirect: 'error' }),
    )
  })
  it('rejects protocol mismatches before attaching', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify({ protocolVersion: 2, terminalWireVersion: '2', sessions: [] })),
        ),
    )
    await expect(listRemoteSessions(endpoint)).rejects.toThrow('incompatible protocol')
  })
  it('explains expired credentials', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('', { status: 401 })))
    await expect(listRemoteSessions(endpoint)).rejects.toThrow('fresh token')
  })
})
