import { describe, expect, it, vi } from 'vitest'
import { useTokenConnection } from '../useTokenConnection'

function setup(disconnectAccount = vi.fn()) {
  return { disconnectAccount, ...useTokenConnection(disconnectAccount, 'stack') }
}

describe('useTokenConnection', () => {
  it('runs a call and reports success', async () => {
    const { run, error, busy } = setup()
    const call = vi.fn().mockResolvedValue({ account: 'a' })

    await expect(run(call, 'Rejected.')).resolves.toBe(true)
    expect(call).toHaveBeenCalledOnce()
    expect(error.value).toBeNull()
    expect(busy.value).toBe(false)
  })

  // The backend tells "not a Gitea instance" apart from "token rejected", so
  // its message has to reach the drawer rather than the generic fallback.
  it('surfaces the backend message on a failed call', async () => {
    const { run, error } = setup()

    await expect(
      run(() => Promise.reject(new Error('does not answer as a Gitea instance')), 'Rejected.'),
    ).resolves.toBe(false)
    expect(error.value).toContain('does not answer as a Gitea instance')
  })

  it('accepts a bare string rejection, as Wails sends', async () => {
    const { run, error } = setup()

    await run(vi.fn().mockRejectedValue('gitea: token is empty'), 'Rejected.')
    expect(error.value).toBe('gitea: token is empty')
  })

  it('falls back to the given message when the failure carries none', async () => {
    const { run, error } = setup()

    await run(vi.fn().mockRejectedValue({}), 'Grafana rejected the connection.')
    expect(error.value).toBe('Grafana rejected the connection.')
  })

  it('ignores a second call while one is in flight', async () => {
    const { run, busy } = setup()
    let resolve!: () => void
    const call = vi.fn(() => new Promise<void>((r) => (resolve = r)))

    const first = run(call, 'Rejected.')
    expect(busy.value).toBe(true)
    await expect(run(call, 'Rejected.')).resolves.toBe(false)
    expect(call).toHaveBeenCalledOnce()

    resolve()
    await expect(first).resolves.toBe(true)
    expect(busy.value).toBe(false)
  })

  it('disconnects by account and names the noun when it fails', async () => {
    const { disconnectAccount, disconnect, error } = setup(vi.fn().mockRejectedValue({}))

    await disconnect('grafana.example.com-1')
    expect(disconnectAccount).toHaveBeenCalledWith('grafana.example.com-1')
    expect(error.value).toBe('Could not disconnect the stack.')
  })
})
