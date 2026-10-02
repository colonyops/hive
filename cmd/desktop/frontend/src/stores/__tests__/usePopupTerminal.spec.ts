import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { usePopupTerminal } from '../usePopupTerminal'

const mocks = vi.hoisted(() => ({
  Available: vi.fn(),
  getPopupTerminalEndpoint: vi.fn(),
  createPopupTerminalClient: vi.fn(),
}))

vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/popupterminalservice',
  () => ({
    Available: mocks.Available,
  }),
)
vi.mock('../../lib/popupTerminalClient', () => ({
  getPopupTerminalEndpoint: mocks.getPopupTerminalEndpoint,
  createPopupTerminalClient: mocks.createPopupTerminalClient,
}))

describe('usePopupTerminal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.Available.mockResolvedValue({ available: true, reason: '' })
    mocks.getPopupTerminalEndpoint.mockResolvedValue({ url: 'http://127.0.0.1:1', token: 't' })
    mocks.createPopupTerminalClient.mockReturnValue({ id: 'client' })
  })

  it('shows with the request it was asked for and bumps the launch only when it changes', () => {
    const popup = usePopupTerminal()

    popup.show({ launcher: 'lazygit', sessionSlug: 'hive-abc' })
    expect(popup.visible.value).toBe(true)
    expect(popup.request.value).toEqual({ launcher: 'lazygit', sessionSlug: 'hive-abc' })
    expect(popup.launchSeq.value).toBe(1)

    popup.hide()
    popup.show({ launcher: 'lazygit', sessionSlug: 'hive-abc' })
    expect(popup.launchSeq.value).toBe(1)

    popup.show({ sessionSlug: 'hive-abc' })
    expect(popup.launchSeq.value).toBe(2)
  })

  it('toggles the same launch closed and a different one open', () => {
    const popup = usePopupTerminal()

    popup.toggle({ launcher: 'lazygit' })
    expect(popup.visible.value).toBe(true)
    popup.toggle({ launcher: 'lazygit' })
    expect(popup.visible.value).toBe(false)

    popup.toggle({ launcher: 'lazygit' })
    popup.toggle({ launcher: 'dotfiles' })
    expect(popup.visible.value).toBe(true)
    expect(popup.request.value).toEqual({ launcher: 'dotfiles' })
  })

  it('probes availability and the transport once', async () => {
    const popup = usePopupTerminal()
    expect(popup.checking.value).toBe(true)

    await Promise.all([popup.ready(), popup.ready()])
    popup.show()
    await flushPromises()

    expect(mocks.Available).toHaveBeenCalledTimes(1)
    expect(popup.checking.value).toBe(false)
    expect(popup.available.value).toBe(true)
    expect(popup.client.value).toEqual({ id: 'client' })
  })

  it('reports an unavailable terminal with the reason the probe gave', async () => {
    mocks.Available.mockResolvedValue({ available: false, reason: 'no shell' })
    const popup = usePopupTerminal()

    await popup.ready()

    expect(popup.available.value).toBe(false)
    expect(popup.reason.value).toBe('no shell')
    expect(popup.client.value).toBeNull()
  })

  it('reports a failed probe as unavailable', async () => {
    mocks.Available.mockRejectedValue(new Error('backend down'))
    const popup = usePopupTerminal()

    await popup.ready()

    expect(popup.checking.value).toBe(false)
    expect(popup.available.value).toBe(false)
    expect(popup.reason.value).toBe('backend down')
  })
})
