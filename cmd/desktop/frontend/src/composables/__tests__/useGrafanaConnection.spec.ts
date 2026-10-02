import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useGrafanaConnection } from '../useGrafanaConnection'

const mocks = vi.hoisted(() => ({ Connect: vi.fn(), Disconnect: vi.fn() }))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/grafanaservice', () => ({
  Connect: mocks.Connect,
  Disconnect: mocks.Disconnect,
}))

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useGrafanaConnection', () => {
  it('connects and records the resolved stack', async () => {
    mocks.Connect.mockResolvedValue({
      account: 'grafana.example.com-1',
      url: 'https://grafana.example.com',
      orgID: 1,
      orgName: 'Main',
    })
    const { connect, lastConnected, error, busy } = useGrafanaConnection()

    const ok = await connect('https://grafana.example.com', 'token')

    expect(ok).toBe(true)
    expect(mocks.Connect).toHaveBeenCalledWith('https://grafana.example.com', 'token')
    expect(lastConnected.value?.account).toBe('grafana.example.com-1')
    expect(error.value).toBeNull()
    expect(busy.value).toBe(false)
  })

  it('surfaces a rejected connection as an error rather than throwing', async () => {
    mocks.Connect.mockRejectedValue(new Error('grafana rejected the token'))
    const { connect, error, lastConnected } = useGrafanaConnection()

    const ok = await connect('https://grafana.example.com', 'bad')

    expect(ok).toBe(false)
    expect(error.value).toBe('grafana rejected the token')
    expect(lastConnected.value).toBeNull()
  })

  it('disconnects an account by name', async () => {
    mocks.Disconnect.mockResolvedValue(undefined)
    const { disconnect } = useGrafanaConnection()

    await disconnect('grafana.example.com-1')

    expect(mocks.Disconnect).toHaveBeenCalledWith('grafana.example.com-1')
  })

  it('ignores a concurrent connect while one is in flight', async () => {
    let resolve: (v: unknown) => void = () => {}
    mocks.Connect.mockReturnValue(
      new Promise((r) => {
        resolve = r
      }),
    )
    const { connect } = useGrafanaConnection()

    const first = connect('https://grafana.example.com', 'token')
    const second = await connect('https://grafana.example.com', 'token') // rejected: busy
    expect(second).toBe(false)
    expect(mocks.Connect).toHaveBeenCalledTimes(1)

    resolve({ account: 'grafana.example.com-1', url: 'https://grafana.example.com', orgID: 1, orgName: 'Main' })
    expect(await first).toBe(true)
  })
})
