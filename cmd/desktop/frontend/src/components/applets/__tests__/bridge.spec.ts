import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import bridgeSource from '../../../../../internal/app/applets/bridge.js?raw'
import { useKeybindings } from '../../../composables/useKeybindings'
import { resetStores } from '../../../stores/defineStore'
import { useApplets } from '../../../stores/useApplets'
import AppletFrame from '../AppletFrame.vue'

// Runs the real bridge.js against the real AppletFrame, so a method name that drifts
// between the two fails here and not at runtime in a page.

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

const render = vi.hoisted(() => vi.fn())

vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/promptsservice', () => ({
  Render: render,
}))

const applet = { id: 'pong', title: 'Pong', entry: 'index.html', revision: 1 }

interface Hive {
  exec(command: string, options?: object): Promise<unknown>
  spawn(command: string, options: object): Promise<{ write(data: string): Promise<void>; kill(): Promise<void> }>
  readFile(path: string): Promise<string>
  writeFile(path: string, content: string): Promise<void>
  context(): Promise<unknown>
  toast(message: string): Promise<void>
  authoringPrompt(brief: object): Promise<string>
  close(): Promise<void>
  on(event: string, listener: (data: unknown) => void): () => void
  setCommands(rows: object[]): Promise<void>
}

interface PageWindow {
  hive: Hive
  localStorage: Storage
  __hiveToken?: string
  __hiveStorage?: Record<string, string>
}

type Listener = (event: { source?: unknown; data?: unknown; key?: string; defaultPrevented?: boolean }) => void

function loadPage(storage: Record<string, string> = {}) {
  const wrapper = mount(AppletFrame, {
    props: { applet: applet as never, sessionSlug: 'sess' },
    attachTo: document.body,
  })
  const frameWindow = wrapper.get('iframe').element.contentWindow as Window
  const listeners: Record<string, Listener[]> = {}
  const page: PageWindow = {
    __hiveToken: 'secret',
    __hiveStorage: storage,
  } as unknown as PageWindow
  const parent = {
    postMessage(data: unknown) {
      window.dispatchEvent(new MessageEvent('message', { data, source: frameWindow }))
    },
  }
  vi.spyOn(frameWindow, 'postMessage').mockImplementation((data: unknown) => {
    for (const listener of listeners.message ?? []) listener({ source: parent, data })
  })
  const addEventListener = (type: string, listener: Listener) => {
    ;(listeners[type] ??= []).push(listener)
  }
  // eslint-disable-next-line no-new-func, @typescript-eslint/no-implied-eval -- running the script a page receives is the point
  new Function('window', 'parent', 'addEventListener', bridgeSource)(page, parent, addEventListener)
  return { wrapper, page, listeners }
}

describe('bridge.js', () => {
  beforeEach(() => {
    resetStores()
    for (const mock of Object.values(service)) mock.mockReset()
    service.BridgeToken.mockResolvedValue('secret')
  })

  it('takes the token and the saved storage off the window', () => {
    const { wrapper, page } = loadPage({ a: '1' })
    expect(page.__hiveToken).toBeUndefined()
    expect(page.__hiveStorage).toBeUndefined()
    expect(page.localStorage.getItem('a')).toBe('1')
    wrapper.unmount()
  })

  it('reaches every service call through the host', async () => {
    service.Exec.mockResolvedValue({ stdout: 'hi', stderr: '', code: 0 })
    service.ReadFile.mockResolvedValue('text')
    service.Context.mockResolvedValue({ id: 'pong' })
    const { wrapper, page } = loadPage()

    await expect(page.hive.exec('echo hi', { timeout: 5 })).resolves.toMatchObject({ stdout: 'hi' })
    await expect(page.hive.readFile('a.txt')).resolves.toBe('text')
    await page.hive.writeFile('a.txt', 'body')
    await expect(page.hive.context()).resolves.toEqual({ id: 'pong' })
    await page.hive.toast('hello')

    expect(service.Exec).toHaveBeenCalledWith('pong', expect.objectContaining({ command: 'echo hi', timeoutMillis: 5 }))
    expect(service.WriteFile).toHaveBeenCalledWith('pong', 'a.txt', 'body')
    wrapper.unmount()
  })

  it('asks Hive for the authoring prompt with the answers it was given', async () => {
    render.mockResolvedValue({ text: 'the prompt' })
    const { wrapper, page } = loadPage()
    const brief = { style: 'hive', reference: '' }

    await expect(page.hive.authoringPrompt(brief)).resolves.toBe('the prompt')
    expect(render).toHaveBeenCalledWith('applets', { webhookPath: '', webhookSample: '', appletBrief: brief })
    wrapper.unmount()
  })

  it('rejects when the host reports a failure', async () => {
    service.ReadFile.mockRejectedValue(new Error('no such file'))
    const { wrapper, page } = loadPage()
    await expect(page.hive.readFile('absent')).rejects.toThrow('no such file')
    wrapper.unmount()
  })

  it('pumps a spawned process to its callbacks until it exits', async () => {
    service.Spawn.mockResolvedValue('p1')
    service.SpawnRead.mockResolvedValueOnce({ stdout: 'one', stderr: '', done: false, code: 0 }).mockResolvedValueOnce({
      stdout: 'two',
      stderr: 'warn',
      done: true,
      code: 3,
    })
    const { wrapper, page } = loadPage()
    const onStdout = vi.fn()
    const onStderr = vi.fn()
    const exited = new Promise((resolve) => {
      void page.hive.spawn('run', { onStdout, onStderr, onExit: resolve }).then(async (process) => {
        await process.write('in')
        await process.kill()
      })
    })

    await expect(exited).resolves.toBe(3)
    expect(onStdout.mock.calls.flat()).toEqual(['one', 'two'])
    expect(onStderr.mock.calls.flat()).toEqual(['warn'])
    expect(service.SpawnWrite).toHaveBeenCalledWith('p1', 'in')
    expect(service.SpawnKill).toHaveBeenCalledWith('p1')
    wrapper.unmount()
  })

  it('saves localStorage through the host after a burst of writes', async () => {
    vi.useFakeTimers()
    try {
      const { wrapper, page } = loadPage()
      page.localStorage.setItem('a', '1')
      page.localStorage.setItem('b', '2')
      page.localStorage.removeItem('a')
      expect(service.SaveStorage).not.toHaveBeenCalled()
      await vi.advanceTimersByTimeAsync(250)
      await flushPromises()
      expect(service.SaveStorage).toHaveBeenCalledTimes(1)
      expect(service.SaveStorage).toHaveBeenCalledWith('pong', '{"b":"2"}')
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  it('closes on Escape unless the page kept the key', async () => {
    vi.useFakeTimers()
    try {
      const { wrapper, listeners } = loadPage()
      const keydown = listeners.keydown?.[0]
      keydown?.({ key: 'Escape', defaultPrevented: true })
      await vi.advanceTimersByTimeAsync(5)
      await flushPromises()
      expect(wrapper.emitted('close')).toBeUndefined()

      keydown?.({ key: 'Escape', defaultPrevented: false })
      await vi.advanceTimersByTimeAsync(5)
      await flushPromises()
      expect(wrapper.emitted('close')).toHaveLength(1)
      wrapper.unmount()
    } finally {
      vi.useRealTimers()
    }
  })

  describe('keys', () => {
    function keysSeenByTheApp() {
      const seen: string[] = []
      const listener = (event: KeyboardEvent) => seen.push(event.key)
      window.addEventListener('keydown', listener)
      return { seen, stop: () => window.removeEventListener('keydown', listener) }
    }

    async function press(keydown: Listener | undefined, event: object) {
      keydown?.(event)
      await vi.advanceTimersByTimeAsync(5)
      await flushPromises()
    }

    it('hands the app a chord from a panel, unless the page kept it', async () => {
      vi.useFakeTimers()
      const app = keysSeenByTheApp()
      try {
        const { wrapper, listeners } = loadPage()
        const keydown = listeners.keydown?.[0]
        await press(keydown, { key: 'k', metaKey: true, defaultPrevented: true })
        expect(app.seen).toEqual([])
        await press(keydown, { key: 'k', metaKey: true, defaultPrevented: false })
        expect(app.seen).toEqual(['k'])
        wrapper.unmount()
      } finally {
        app.stop()
        vi.useRealTimers()
      }
    })

    it('keeps a bare key and a modifier alone', async () => {
      vi.useFakeTimers()
      const app = keysSeenByTheApp()
      try {
        const { wrapper, listeners } = loadPage()
        const keydown = listeners.keydown?.[0]
        await press(keydown, { key: 'j', defaultPrevented: false })
        await press(keydown, { key: 'Meta', metaKey: true, defaultPrevented: false })
        expect(app.seen).toEqual([])
        wrapper.unmount()
      } finally {
        app.stop()
        vi.useRealTimers()
      }
    })

    it('hands on the next key while a sequence waits', async () => {
      vi.useFakeTimers()
      const app = keysSeenByTheApp()
      const { pendingSequence } = useKeybindings()
      const { wrapper, listeners } = loadPage()
      const keydown = listeners.keydown?.[0]
      // The app is what decides that a chord started a sequence.
      const start = () => {
        pendingSequence.value = { steps: ['mod+g'] } as never
      }
      window.addEventListener('keydown', start, { once: true })
      try {
        await press(keydown, { key: 'g', metaKey: true, defaultPrevented: false })
        await press(keydown, { key: 'i', defaultPrevented: false })
        expect(app.seen).toEqual(['g', 'i'])
        // Timed out: the app drops the key that was on its way, and the page stops asking.
        pendingSequence.value = null
        await press(keydown, { key: 'j', defaultPrevented: false })
        await press(keydown, { key: 'k', defaultPrevented: false })
        expect(app.seen).toEqual(['g', 'i'])
        wrapper.unmount()
      } finally {
        pendingSequence.value = null
        app.stop()
        vi.useRealTimers()
      }
    })
  })

  describe('commands', () => {
    it('holds a command until the page listens', async () => {
      const { wrapper, page } = loadPage()
      await flushPromises()
      useApplets().deliverCommand('pong', 'next')
      await flushPromises()
      const heard: unknown[] = []
      page.hive.on('command', (id) => heard.push(id))
      expect(heard).toEqual(['next'])

      useApplets().deliverCommand('pong', 'again')
      await flushPromises()
      expect(heard).toEqual(['next', 'again'])
      wrapper.unmount()
    })

    it('puts a row in the palette and runs it in the page', async () => {
      const { wrapper, page } = loadPage()
      await page.hive.setCommands([{ id: 'jazz', title: 'Jazz' }])
      const heard: unknown[] = []
      page.hive.on('command', (id) => heard.push(id))
      void useApplets().pageRows.value[0].run()
      await flushPromises()
      expect(heard).toEqual(['jazz'])
      wrapper.unmount()
    })
  })
})
