import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useTerminalAvailability } from '../useTerminalAvailability'

const mocks = vi.hoisted(() => ({
  Available: vi.fn(),
  getTerminalEndpoint: vi.fn(),
  createTerminalClient: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/terminalservice', () => ({
  Available: mocks.Available,
}))
vi.mock('../../lib/terminalClient', () => ({
  getTerminalEndpoint: mocks.getTerminalEndpoint,
  createTerminalClient: mocks.createTerminalClient,
}))

describe('useTerminalAvailability', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.Available.mockResolvedValue({ available: true, reason: '' })
    mocks.getTerminalEndpoint.mockResolvedValue({ url: 'http://127.0.0.1:1', token: 't' })
    mocks.createTerminalClient.mockReturnValue({ id: 'client' })
  })

  it('answers the first probe with a client and clears the gate', async () => {
    const terminal = useTerminalAvailability()
    expect(terminal.checking.value).toBe(true)

    await terminal.probe()

    expect(terminal.checking.value).toBe(false)
    expect(terminal.available.value).toBe(true)
    expect(terminal.client.value).toEqual({ id: 'client' })
  })

  it('revalidates behind the answer on screen and keeps the client it minted', async () => {
    const terminal = useTerminalAvailability()
    await terminal.probe()
    mocks.Available.mockReturnValueOnce(new Promise(() => {}))

    void terminal.probe()

    expect(terminal.checking.value).toBe(false)
    expect(terminal.available.value).toBe(true)
    expect(mocks.createTerminalClient).toHaveBeenCalledTimes(1)
  })

  it('reports an unavailable terminal with the probe reason and no client', async () => {
    mocks.Available.mockResolvedValue({ available: false, reason: 'tmux is not installed.' })
    const terminal = useTerminalAvailability()

    await terminal.probe()

    expect(terminal.available.value).toBe(false)
    expect(terminal.reason.value).toBe('tmux is not installed.')
    expect(terminal.client.value).toBeNull()
  })

  it('reports a failed probe as unavailable', async () => {
    mocks.Available.mockRejectedValue(new Error('backend down'))
    const terminal = useTerminalAvailability()

    await terminal.probe()

    expect(terminal.checking.value).toBe(false)
    expect(terminal.available.value).toBe(false)
    expect(terminal.reason.value).toBe('backend down')
  })
})
