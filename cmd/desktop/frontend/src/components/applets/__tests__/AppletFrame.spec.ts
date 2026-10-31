import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetStores } from '../../../stores/defineStore'
import { usePopupTerminal } from '../../../stores/usePopupTerminal'
import AppletFrame from '../AppletFrame.vue'

const service = vi.hoisted(() => ({
  BridgeToken: vi.fn(),
  Context: vi.fn(),
  Exec: vi.fn(),
  KillSpawns: vi.fn(),
  ReadFile: vi.fn(),
  SaveStorage: vi.fn(),
  Spawn: vi.fn(),
  SpawnKill: vi.fn(),
  SpawnRead: vi.fn(),
  SpawnWrite: vi.fn(),
  WriteFile: vi.fn(),
}))

vi.mock(
  '../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice',
  () => service,
)

const applet = { id: 'pong', title: 'Pong', entry: 'index.html', revision: 7 }

function setup() {
  const wrapper = mount(AppletFrame, {
    props: { applet: applet as never, sessionSlug: 'sess' },
    attachTo: document.body,
  })
  const frame = wrapper.get('iframe').element
  const source = frame.contentWindow as Window
  const replies: unknown[] = []
  vi.spyOn(source, 'postMessage').mockImplementation((message: unknown) => {
    replies.push(message)
  })
  const send = async (method: string, args: object = {}, from: unknown = source, token: unknown = 'secret') => {
    window.dispatchEvent(
      new MessageEvent('message', {
        data: { hiveApplet: 1, token, id: 1, method, args },
        source: from as Window,
      }),
    )
    await flushPromises()
  }
  return { wrapper, frame, replies, send }
}

describe('AppletFrame', () => {
  beforeEach(() => {
    resetStores()
    for (const mock of Object.values(service)) mock.mockReset()
    service.BridgeToken.mockResolvedValue('secret')
  })

  it('loads the page sandboxed, without the same origin as the app', () => {
    const { wrapper, frame } = setup()
    expect(frame.getAttribute('src')).toBe('/__applet/pong/index.html?r=7')
    expect(frame.getAttribute('sandbox')).toContain('allow-scripts')
    expect(frame.getAttribute('sandbox')).not.toContain('allow-same-origin')
    wrapper.unmount()
  })

  it('answers a bridge call from its own frame', async () => {
    service.Exec.mockResolvedValue({ stdout: 'hi', stderr: '', code: 0 })
    const { wrapper, replies, send } = setup()
    await send('exec', { command: 'echo hi', timeout: 50 })
    expect(service.Exec).toHaveBeenCalledWith(
      'pong',
      expect.objectContaining({ command: 'echo hi', sessionSlug: 'sess', timeoutMillis: 50 }),
    )
    expect(replies).toEqual([{ hiveApplet: 1, id: 1, result: { stdout: 'hi', stderr: '', code: 0 } }])
    wrapper.unmount()
  })

  it('ignores a message from any other window', async () => {
    const { wrapper, replies, send } = setup()
    await send('exec', { command: 'rm -rf /' }, window)
    expect(service.Exec).not.toHaveBeenCalled()
    expect(replies).toEqual([])
    wrapper.unmount()
  })

  it('ignores its own frame without the token, as after navigating away', async () => {
    const { wrapper, frame, replies, send } = setup()
    await send('exec', { command: 'rm -rf /' }, frame.contentWindow, null)
    await send('exec', { command: 'rm -rf /' }, frame.contentWindow, 'guess')
    expect(service.Exec).not.toHaveBeenCalled()
    expect(replies).toEqual([])
    wrapper.unmount()
  })

  it('rejects an unknown method and carries a failure back', async () => {
    service.ReadFile.mockRejectedValue(new Error('no such file'))
    const { wrapper, replies, send } = setup()
    await send('nope')
    await send('readFile', { path: 'x' })
    expect(replies).toEqual([
      { hiveApplet: 1, id: 1, error: 'hive has no method "nope"' },
      { hiveApplet: 1, id: 1, error: 'no such file' },
    ])
    wrapper.unmount()
  })

  it('opens the pop-up terminal where the page is running', async () => {
    service.Context.mockResolvedValue({ cwd: '/work' })
    const { wrapper, send } = setup()
    await send('terminal', { command: 'top' })
    expect(usePopupTerminal().request.value).toMatchObject({ command: 'top', dir: '/work' })
    wrapper.unmount()
  })

  it('closes on request and stops the processes the page started', async () => {
    const { wrapper, send } = setup()
    await send('close')
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
    expect(service.KillSpawns).toHaveBeenCalledWith('pong')
  })
})
