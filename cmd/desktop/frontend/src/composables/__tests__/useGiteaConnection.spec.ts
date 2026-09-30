import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useGiteaConnection } from '../useGiteaConnection'

const mocks = vi.hoisted(() => ({ Connect: vi.fn(), Disconnect: vi.fn() }))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/giteaservice', () => ({
  Connect: mocks.Connect,
  Disconnect: mocks.Disconnect,
}))

const connected = {
  account: 'git.example.com-octocat',
  url: 'https://git.example.com',
  login: 'octocat',
  name: 'Octo Cat',
  version: '1.27.1',
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useGiteaConnection', () => {
  it('connects an instance and keeps the account it resolved', async () => {
    mocks.Connect.mockResolvedValue(connected)
    const { connect, lastConnected, error, busy } = useGiteaConnection()

    await expect(connect('https://git.example.com', 'gta_token')).resolves.toBe(true)
    expect(mocks.Connect).toHaveBeenCalledWith('https://git.example.com', 'gta_token')
    expect(lastConnected.value?.account).toBe('git.example.com-octocat')
    expect(error.value).toBeNull()
    expect(busy.value).toBe(false)
  })

  // The backend distinguishes "not a Gitea instance" from "token rejected", and
  // that distinction is the whole point of the two-call connect — so the
  // message has to reach the drawer rather than being replaced by a generic one.
  it('surfaces the backend message on a failed connect', async () => {
    mocks.Connect.mockRejectedValue(new Error('https://example.com does not answer as a Gitea or Forgejo instance'))
    const { connect, error } = useGiteaConnection()

    await expect(connect('https://example.com', 'gta_token')).resolves.toBe(false)
    expect(error.value).toContain('does not answer as a Gitea or Forgejo instance')
  })

  it('ignores a second connect while one is in flight', async () => {
    let resolveConnect!: (value: typeof connected) => void
    mocks.Connect.mockReturnValue(
      new Promise((resolve) => {
        resolveConnect = resolve
      }),
    )
    const { connect, busy } = useGiteaConnection()

    const first = connect('https://git.example.com', 'gta_token')
    await expect(connect('https://git.example.com', 'gta_token')).resolves.toBe(false)
    expect(mocks.Connect).toHaveBeenCalledTimes(1)

    resolveConnect(connected)
    await expect(first).resolves.toBe(true)
    expect(busy.value).toBe(false)
  })

  // Wails rejections can arrive as bare strings rather than Errors.
  it('surfaces a string rejection as the error message', async () => {
    mocks.Connect.mockRejectedValue('gitea: token is empty')
    const { connect, error } = useGiteaConnection()

    await expect(connect('https://git.example.com', '')).resolves.toBe(false)
    expect(error.value).toBe('gitea: token is empty')
  })

  it('falls back to a generic message when the failure carries none', async () => {
    mocks.Connect.mockRejectedValue({})
    const { connect, error } = useGiteaConnection()

    await expect(connect('https://git.example.com', 'gta_token')).resolves.toBe(false)
    expect(error.value).toBe('Gitea rejected the connection.')
  })

  it('disconnects by account', async () => {
    mocks.Disconnect.mockResolvedValue(undefined)
    const { disconnect, error } = useGiteaConnection()

    await disconnect('git.example.com-octocat')
    expect(mocks.Disconnect).toHaveBeenCalledWith('git.example.com-octocat')
    expect(error.value).toBeNull()
  })

  it('reports a failed disconnect without throwing', async () => {
    mocks.Disconnect.mockRejectedValue(new Error('keychain is locked'))
    const { disconnect, error } = useGiteaConnection()

    await disconnect('git.example.com-octocat')
    expect(error.value).toContain('keychain is locked')
  })
})
