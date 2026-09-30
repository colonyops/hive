import { beforeEach, describe, expect, it, vi } from 'vitest'
import { usePostHogConnection } from '../usePostHogConnection'

const mocks = vi.hoisted(() => ({ Connect: vi.fn(), Disconnect: vi.fn(), Projects: vi.fn() }))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/posthogservice', () => ({
  Connect: mocks.Connect,
  Disconnect: mocks.Disconnect,
  Projects: mocks.Projects,
}))

beforeEach(() => {
  vi.clearAllMocks()
})

describe('usePostHogConnection', () => {
  it('loads the projects a key can reach', async () => {
    mocks.Projects.mockResolvedValue([
      { account: '', url: 'https://us.posthog.com', id: 1, name: 'Dev' },
      { account: '', url: 'https://us.posthog.com', id: 2, name: 'Prod' },
    ])
    const { projects, loadProjects, error, busy } = usePostHogConnection()

    await expect(loadProjects('https://us.posthog.com', 'phx-key')).resolves.toBe(true)
    expect(projects.value).toHaveLength(2)
    expect(error.value).toBeNull()
    expect(busy.value).toBe(false)
  })

  // A null from Wails is an empty list, not a crash — and an empty list is a
  // failed lookup, so the drawer stays on step one.
  it('treats a null project list as no projects', async () => {
    mocks.Projects.mockResolvedValue(null)
    const { projects, loadProjects } = usePostHogConnection()

    await expect(loadProjects('https://us.posthog.com', 'phx-key')).resolves.toBe(false)
    expect(projects.value).toEqual([])
  })

  it('surfaces the backend message and clears the picker on a rejected key', async () => {
    mocks.Projects.mockRejectedValue(new Error('posthog rejected the API key (it needs the project:read scope)'))
    const { projects, loadProjects, error } = usePostHogConnection()

    await expect(loadProjects('https://us.posthog.com', 'bad')).resolves.toBe(false)
    expect(error.value).toContain('project:read')
    expect(projects.value).toEqual([])
  })

  it('connects the picked project', async () => {
    mocks.Connect.mockResolvedValue({ account: 'us.posthog.com-2', url: 'https://us.posthog.com', id: 2, name: 'Prod' })
    const { connect, lastConnected } = usePostHogConnection()

    await expect(connect('https://us.posthog.com', 'phx-key', 2)).resolves.toBe(true)
    expect(mocks.Connect).toHaveBeenCalledWith('https://us.posthog.com', 'phx-key', 2)
    expect(lastConnected.value?.account).toBe('us.posthog.com-2')
  })

  it('reports a failed connect without throwing', async () => {
    mocks.Connect.mockRejectedValue(new Error('the key cannot see project 99'))
    const { connect, error } = usePostHogConnection()

    await expect(connect('https://us.posthog.com', 'phx-key', 99)).resolves.toBe(false)
    expect(error.value).toContain('cannot see project 99')
  })

  it('reset drops the picker so another key can be tried', async () => {
    mocks.Projects.mockResolvedValue([{ account: '', url: '', id: 1, name: 'Dev' }])
    const { projects, loadProjects, reset } = usePostHogConnection()

    await loadProjects('https://us.posthog.com', 'phx-key')
    expect(projects.value).toHaveLength(1)

    reset()
    expect(projects.value).toEqual([])
  })

  it('disconnects by account', async () => {
    mocks.Disconnect.mockResolvedValue(undefined)
    const { disconnect, error } = usePostHogConnection()

    await disconnect('us.posthog.com-1')
    expect(mocks.Disconnect).toHaveBeenCalledWith('us.posthog.com-1')
    expect(error.value).toBeNull()
  })
})
