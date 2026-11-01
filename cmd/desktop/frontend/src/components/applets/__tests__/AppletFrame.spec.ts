import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useCommandPalette } from '../../../composables/useCommands'
import { useApplets } from '../../../stores/useApplets'
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

function setup(extra: object = {}) {
  const wrapper = mount(AppletFrame, {
    props: { applet: applet as never, sessionSlug: 'sess', ...extra },
    attachTo: document.body,
  })
  const frame = wrapper.get('iframe').element
  const source = frame.contentWindow as Window
  const replies: unknown[] = []
  const port = {
    onmessage: null as ((event: { data: unknown }) => void) | null,
    postMessage: (message: unknown) => replies.push(message),
    close: vi.fn(),
  }
  const connect = async (from: unknown = source, token: unknown = 'secret') => {
    const event = new MessageEvent('message', {
      data: { hiveApplet: 1, token, method: 'connect' },
      source: from as Window,
    })
    Object.defineProperty(event, 'ports', { value: [port] })
    window.dispatchEvent(event)
    await flushPromises()
  }
  const send = async (method: string, args: object = {}, from: unknown = source, token: unknown = 'secret') => {
    await connect(from, token)
    port.onmessage?.({ data: { id: 1, method, args } })
    await flushPromises()
  }
  return { wrapper, frame, replies, send, connect, port }
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

  it('gives a terminal what hive.exec gets: the applet directory and a relative cwd', async () => {
    service.Context.mockResolvedValue({ cwd: '/work', dir: '/applets/pong' })
    const { wrapper, send } = setup()
    await send('terminal', { command: 'top', cwd: 'sub' })
    expect(usePopupTerminal().request.value).toMatchObject({
      dir: '/work/sub',
      env: ['HIVE_APPLET_DIR=/applets/pong'],
    })
    await send('terminal', { command: 'top', cwd: '~/x' })
    expect(usePopupTerminal().request.value).toMatchObject({ dir: '~/x' })
    wrapper.unmount()
  })

  it('answers nothing on the window, so a page the frame navigates to hears nothing', async () => {
    service.Exec.mockResolvedValue({ stdout: '', stderr: '', code: 0 })
    const { wrapper, frame, send } = setup()
    const onWindow = vi.spyOn(frame.contentWindow as Window, 'postMessage')
    await send('exec', { command: 'true' })
    await wrapper.setProps({ sessionSlug: 'other' })
    expect(onWindow).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('treats a new port as a new page: what the last one set up goes with it', async () => {
    service.Spawn.mockResolvedValue('p1')
    const { wrapper, frame, send, port } = setup()
    await send('ready')
    await send('spawn', { command: 'sleep 60' })
    await send('setCommands', { rows: [{ id: 'jazz', title: 'Jazz' }] })
    expect(useApplets().pageRows.value).toHaveLength(1)

    const next = { onmessage: null, postMessage: vi.fn(), close: vi.fn() }
    const event = new MessageEvent('message', {
      data: { hiveApplet: 1, token: 'secret', method: 'connect' },
      source: frame.contentWindow,
    })
    Object.defineProperty(event, 'ports', { value: [next] })
    window.dispatchEvent(event)
    await flushPromises()

    expect(port.close).toHaveBeenCalled()
    expect(service.KillSpawns).toHaveBeenCalledWith(['p1'])
    expect(useApplets().pageRows.value).toEqual([])
    useApplets().deliverCommand('pong', 'held')
    expect(next.postMessage).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('takes only a save after the frame has gone', async () => {
    const { wrapper, send, port } = setup()
    await send('ready')
    wrapper.unmount()
    port.onmessage?.({ data: { id: 2, method: 'saveStorage', args: { encoded: '{}' } } })
    port.onmessage?.({ data: { id: 3, method: 'exec', args: { command: 'true' } } })
    await flushPromises()
    expect(service.SaveStorage).toHaveBeenCalledWith('pong', '{}')
    expect(service.Exec).not.toHaveBeenCalled()
  })

  it('closes on request and stops only the processes this frame started', async () => {
    service.Spawn.mockResolvedValue('p1')
    const { wrapper, send } = setup()
    await send('spawn', { command: 'sleep 60' })
    await send('close')
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
    expect(service.KillSpawns).toHaveBeenCalledWith(['p1'])
  })

  it('stops nothing when the frame started nothing', () => {
    const { wrapper } = setup()
    wrapper.unmount()
    expect(service.KillSpawns).not.toHaveBeenCalled()
  })

  it('does not stop a process that already ended', async () => {
    service.Spawn.mockResolvedValue('p1')
    service.SpawnRead.mockResolvedValue({ stdout: '', stderr: '', done: true, code: 0 })
    const { wrapper, send } = setup()
    await send('spawn', { command: 'true' })
    await send('spawnRead', { handle: 'p1' })
    wrapper.unmount()
    expect(service.KillSpawns).not.toHaveBeenCalled()
  })

  describe('in the title bar', () => {
    function titlebar(surface = 'titlebar') {
      return setup({ surface, entry: 'bar.html' })
    }

    it('says which surface it is on, and loads its own entry', () => {
      const { wrapper, frame } = titlebar()
      expect(frame.getAttribute('src')).toBe('/__applet/pong/bar.html?r=7&surface=titlebar')
      wrapper.unmount()
    })

    it('does not take focus when it loads', async () => {
      const { wrapper, frame } = titlebar()
      const focus = vi.spyOn(frame, 'focus')
      await wrapper.get('iframe').trigger('load')
      expect(focus).not.toHaveBeenCalled()
      wrapper.unmount()
    })

    it('opens a dropdown from its own frame, and a second call closes it', async () => {
      const { wrapper, frame, send } = titlebar()
      await send('dropdown', { entry: 'menu.html', width: 200, height: 100 })
      expect(useApplets().dropdown.value).toMatchObject({
        entry: 'menu.html',
        width: 200,
        height: 100,
        anchor: frame,
      })
      await send('dropdown', { entry: 'menu.html' })
      expect(useApplets().dropdown.value).toBeNull()
      wrapper.unmount()
    })

    it('refuses a dropdown from any other surface', async () => {
      const { wrapper, replies, send } = setup()
      await send('dropdown')
      expect(replies).toEqual([{ hiveApplet: 1, id: 1, error: 'Only a title bar page can open a dropdown.' }])
      expect(useApplets().dropdown.value).toBeNull()
      wrapper.unmount()
    })

    it('passes a resize to its host, and a panel refuses one', async () => {
      const bar = titlebar()
      await bar.send('resize', { width: 120 })
      expect(bar.wrapper.emitted('resize')).toEqual([[{ width: 120 }]])
      bar.wrapper.unmount()

      const panel = setup()
      await panel.send('resize', { width: 120 })
      expect(panel.wrapper.emitted('resize')).toBeUndefined()
      panel.wrapper.unmount()
    })

    it('hands a chord to the app keymap', async () => {
      const { wrapper, send } = titlebar()
      const seen: KeyboardEvent[] = []
      const listener = (event: KeyboardEvent) => seen.push(event)
      window.addEventListener('keydown', listener)
      await send('key', { key: 'k', metaKey: true })
      window.removeEventListener('keydown', listener)
      expect(seen).toHaveLength(1)
      expect(seen[0]).toMatchObject({ key: 'k', metaKey: true })
      wrapper.unmount()
    })

    it('runs an app command for the page', async () => {
      const { wrapper, send } = titlebar()
      const run = vi.fn()
      useApplets().setCommandRunner(run)
      await send('command', { id: 'settings.open' })
      expect(run).toHaveBeenCalledWith('settings.open')
      wrapper.unmount()
    })

    it('refuses a command that does not exist', async () => {
      const { wrapper, send, replies } = titlebar()
      const run = vi.fn()
      useApplets().setCommandRunner(run)
      await send('command', { id: 'settings.nope' })
      expect(run).not.toHaveBeenCalled()
      expect(replies).toEqual([{ hiveApplet: 1, id: 1, error: 'hive has no command "settings.nope"' }])
      wrapper.unmount()
    })

    it('lists commands with the keys they are bound to', async () => {
      const { wrapper, send, replies } = titlebar()
      await send('commands')
      const { result } = replies[0] as { result: { id: string; keys: string[] }[] }
      expect(result.find((command) => command.id === 'palette.toggle')?.keys).toContain('mod+k')
      wrapper.unmount()
    })

    it('opens the palette on a query, and a sigil enters its scope', async () => {
      const { wrapper, send } = titlebar()
      const palette = useCommandPalette()
      await send('openPalette', { query: '+doc' })
      expect(palette.open.value).toBe(true)
      expect(palette.scope.value).toBe('applets')
      expect(palette.query.value).toBe('doc')
      palette.toggle()
      wrapper.unmount()
    })

    it('does not run the rest of a sequence that has timed out', async () => {
      const { wrapper, send, replies } = titlebar()
      const seen: KeyboardEvent[] = []
      const listener = (event: KeyboardEvent) => seen.push(event)
      window.addEventListener('keydown', listener)
      await send('key', { key: 'j', continuing: true })
      window.removeEventListener('keydown', listener)
      expect(seen).toEqual([])
      expect(replies).toEqual([{ hiveApplet: 1, id: 1, result: false }])
      wrapper.unmount()
    })

    it('tells the page when the session on screen changes', async () => {
      const { wrapper, replies, connect } = titlebar()
      await connect()
      await wrapper.setProps({ sessionSlug: 'other' })
      expect(replies).toEqual([{ hiveApplet: 1, event: 'session', data: 'other' }])
      wrapper.unmount()
    })
  })

  describe('palette rows from a page', () => {
    it('lasts as long as the frame, and running one reaches that frame', async () => {
      const { wrapper, send, replies } = setup()
      await send('ready')
      await send('setCommands', { rows: [{ id: 'jazz', title: 'Jazz', hint: 'station', scope: 'goto' }] })
      const applets = useApplets()
      const [row] = applets.pageRows.value
      expect(row).toMatchObject({
        id: 'applet:pong:jazz',
        title: 'Jazz',
        group: 'pong',
        hint: 'station',
        scope: 'goto',
      })

      replies.length = 0
      void row.run()
      expect(replies).toEqual([{ hiveApplet: 1, event: 'command', data: 'jazz' }])

      wrapper.unmount()
      expect(applets.pageRows.value).toEqual([])
    })

    it('replaces the rows it set before', async () => {
      const { wrapper, send } = setup()
      await send('setCommands', { rows: [{ id: 'a', title: 'A' }] })
      await send('setCommands', { rows: [{ id: 'b', title: 'B' }] })
      expect(useApplets().pageRows.value.map((row) => row.id)).toEqual(['applet:pong:b'])
      await send('setCommands', { rows: [] })
      expect(useApplets().pageRows.value).toEqual([])
      wrapper.unmount()
    })

    it('falls back on a scope this Hive does not know, and leaves out a field of the wrong type', async () => {
      const { wrapper, send, replies } = setup()
      await send('setCommands', { rows: [{ id: 'a', title: 'A', scope: 'nowhere', keywords: 5, hint: 7 }] })
      expect(replies).toEqual([{ hiveApplet: 1, id: 1, result: undefined }])
      const [row] = useApplets().pageRows.value
      expect(row).toMatchObject({ id: 'applet:pong:a', title: 'A', scope: undefined, hint: undefined })
      wrapper.unmount()
    })

    it.each([
      ['not a list', 'nope', 'setCommands takes a list of rows.'],
      ['no title', [{ id: 'a' }], 'A palette row needs an id and a title.'],
    ])('refuses %s', async (_name, rows, error) => {
      const { wrapper, send, replies } = setup()
      await send('setCommands', { rows })
      expect(replies).toEqual([{ hiveApplet: 1, id: 1, error }])
      expect(useApplets().pageRows.value).toEqual([])
      wrapper.unmount()
    })
  })

  describe('commands the applet declares', () => {
    it('reaches the title bar page when both are open', async () => {
      const panel = setup()
      const bar = setup({ surface: 'titlebar' })
      await panel.send('ready')
      await bar.send('ready')
      panel.replies.length = 0
      bar.replies.length = 0
      expect(useApplets().deliverCommand('pong', 'next')).toBe(true)
      expect(bar.replies).toEqual([{ hiveApplet: 1, event: 'command', data: 'next' }])
      expect(panel.replies).toEqual([])
      panel.wrapper.unmount()
      bar.wrapper.unmount()
    })

    it('reports that no page can take it', () => {
      expect(useApplets().deliverCommand('pong', 'next')).toBe(false)
    })

    it('waits for the panel it opens to load', async () => {
      useApplets().openForCommand('pong', 'next', 'sess')
      const { wrapper, send, replies } = setup()
      expect(replies).toEqual([])
      await send('ready')
      expect(replies).toContainEqual({ hiveApplet: 1, event: 'command', data: 'next' })
      wrapper.unmount()
    })
  })

  describe('with another frame of the same applet open', () => {
    function pair() {
      const first = setup()
      const second = setup({ surface: 'titlebar' })
      return { first, second }
    }

    it('carries a broadcast to the other frame and not back', async () => {
      const { first, second } = pair()
      await second.connect()
      await first.send('broadcast', { data: { playing: true } })
      expect(second.replies).toEqual([{ hiveApplet: 1, event: 'message', data: { playing: true } }])
      expect(first.replies).toEqual([{ hiveApplet: 1, id: 1 }])
      first.wrapper.unmount()
      second.wrapper.unmount()
    })

    it('carries a storage change to the other frame', async () => {
      const { first, second } = pair()
      await second.connect()
      await first.send('storageChanged', { key: 'k', value: 'v' })
      expect(second.replies).toEqual([{ hiveApplet: 1, storage: { key: 'k', value: 'v' } }])
      first.wrapper.unmount()
      second.wrapper.unmount()
    })

    it('stops reaching a frame that closed', async () => {
      const { first, second } = pair()
      second.wrapper.unmount()
      await first.send('broadcast', { data: 1 })
      expect(second.replies).toEqual([])
      first.wrapper.unmount()
    })
  })
})
