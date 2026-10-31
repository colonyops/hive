import { useEventListener, useMutationObserver } from '@vueuse/core'
import { onBeforeUnmount, watch, type Ref } from 'vue'
import { useClipboard } from '@hive/ui/composables/useClipboard'
import {
  BridgeToken,
  Context,
  Exec,
  KillSpawns,
  ReadFile,
  SaveStorage,
  Spawn,
  SpawnKill,
  SpawnRead,
  SpawnWrite,
  WriteFile,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice'
import { Render } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/promptsservice'
import type { Applet } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { useCommandPalette } from '../../composables/useCommands'
import { useKeybindings } from '../../composables/useKeybindings'
import { useToasts } from '../../composables/useToasts'
import { appletCommandID, commandById, commands } from '../../keybindings/catalog'
import { router } from '../../router'
import { useApplets, type AppletFrameHandle, type AppletPageRow } from '../../stores/useApplets'
import { usePopupTerminal } from '../../stores/usePopupTerminal'

export type AppletSurface = 'page' | 'titlebar' | 'dropdown'

export const DEFAULT_DROPDOWN_WIDTH = 320
export const DEFAULT_DROPDOWN_HEIGHT = 360

type AppletBrief = NonNullable<Parameters<typeof Render>[1]['appletBrief']>

// eslint-disable-next-line @typescript-eslint/no-explicit-any -- the page is free-form; the services validate
type BridgeArgs = Record<string, any>

// Custom properties do not cross into a frame, and browsers do not reliably enumerate
// them, so the tokens a page can use are named here.
const THEME_PROPERTIES = [
  'color-scheme',
  ...[
    'app',
    'raised',
    'sidebar',
    'pane',
    'list',
    'chip',
    'hover',
    'border',
    'card',
    'strong',
    'row',
    'row-hover',
    'text',
    'text-2',
    'text-3',
    'text-4',
    'accent',
    'accent-contrast',
    'accent-tint',
    'selection',
    'backdrop',
    'code-key',
    'code-string',
    'code-comment',
    ...['info', 'success', 'warning', 'error', 'running'].flatMap((severity) => [
      `severity-${severity}`,
      `severity-${severity}-tint`,
    ]),
  ].map((token) => `--hv-${token}`),
]

function readTheme(): Record<string, string> {
  const computed = getComputedStyle(document.documentElement)
  return Object.fromEntries(THEME_PROPERTIES.map((name) => [name, computed.getPropertyValue(name).trim()]))
}

// A row goes in the palette, so a wrong one should say so to the page that sent it rather
// than draw a blank row.
function parsePageRows(rows: unknown): AppletPageRow[] {
  if (!Array.isArray(rows)) throw new Error('setCommands takes a list of rows.')
  return rows.map((row: Partial<AppletPageRow>) => {
    if (!row?.id || !row.title) throw new Error('A palette row needs an id and a title.')
    if (row.scope !== undefined && row.scope !== 'goto' && row.scope !== 'actions')
      throw new Error(`A palette row's scope is goto or actions, not "${String(row.scope)}".`)
    return { ...row, id: String(row.id), title: String(row.title) }
  })
}

export interface AppletBridgeOptions {
  // A frame's host keys it by applet id, so the id cannot change under it.
  applet: () => Applet
  frame: Ref<HTMLIFrameElement | null>
  surface: () => AppletSurface
  sessionSlug: () => string
  close: () => void
  resize: (size: { width?: number; height?: number }) => void
}

// What window.hive reaches, for one frame. The window does not prove who is asking,
// because a frame can navigate to another page and keep it, so only the applet's own
// pages hold the token (ADR applets-are-web-pages-with-full-access-to-the-machine).
export function useAppletBridge(options: AppletBridgeOptions): void {
  const { frame } = options
  const id = options.applet().id
  const applets = useApplets()
  const popupTerminal = usePopupTerminal()
  const { showToast } = useToasts()
  const clipboard = useClipboard()
  const token = BridgeToken(id)

  useMutationObserver(
    document.documentElement,
    () => frame.value?.contentWindow?.postMessage({ hiveApplet: 1, theme: readTheme() }, '*'),
    { attributeFilter: ['data-theme'] },
  )

  const keybindings = useKeybindings()
  const palette = useCommandPalette()

  // A page cannot hear anything until its bridge has loaded, and says when it has.
  let pageReady = false
  const queuedCommands: string[] = []

  function postCommand(command: string): void {
    frame.value?.contentWindow?.postMessage({ hiveApplet: 1, event: 'command', data: command }, '*')
  }

  const handle: AppletFrameHandle = {
    surface: options.surface(),
    window: () => frame.value?.contentWindow,
    deliverCommand(command) {
      if (pageReady) postCommand(command)
      else queuedCommands.push(command)
    },
  }
  const disconnect = applets.connectFrame(id, handle)

  watch(options.sessionSlug, (slug) => {
    frame.value?.contentWindow?.postMessage({ hiveApplet: 1, event: 'session', data: slug }, '*')
  })

  const processes = new Set<string>()
  let closed = false

  function endProcesses(): void {
    if (processes.size > 0) void KillSpawns([...processes])
    processes.clear()
  }

  const execRequest = (args: BridgeArgs) => ({
    command: String(args.command ?? ''),
    sessionSlug: options.sessionSlug(),
    cwd: String(args.cwd ?? ''),
    env: (args.env ?? {}) as Record<string, string>,
    stdin: String(args.stdin ?? ''),
    timeoutMillis: Number(args.timeout ?? 0),
  })

  function others(self: Window, message: object): void {
    for (const peer of applets.otherFrames(id, self)) peer.postMessage({ hiveApplet: 1, ...message }, '*')
  }

  const methods: Record<string, (args: BridgeArgs, source: Window) => unknown> = {
    context: () => Context(id, options.sessionSlug()),
    theme: readTheme,
    exec: (args) => Exec(id, execRequest(args)),
    async spawn(args) {
      const handle = await Spawn(id, execRequest(args))
      // The frame can close while the process is starting.
      if (closed) void KillSpawns([handle])
      else processes.add(handle)
      return handle
    },
    async spawnRead(args) {
      const output = await SpawnRead(String(args.handle))
      if (output.done) processes.delete(String(args.handle))
      return output
    },
    spawnWrite: (args) => SpawnWrite(String(args.handle), String(args.data)),
    spawnKill: (args) => SpawnKill(String(args.handle)),
    readFile: (args) => ReadFile(id, String(args.path)),
    writeFile: (args) => WriteFile(id, String(args.path), String(args.content)),
    saveStorage: (args) => SaveStorage(id, String(args.encoded)),
    storageChanged: (args, source) => others(source, { storage: args as object }),
    broadcast: (args, source) => others(source, { event: 'message', data: args.data as unknown }),
    async terminal(args) {
      const context = await Context(id, options.sessionSlug())
      popupTerminal.launch({ command: String(args.command ?? ''), dir: String(args.cwd ?? context.cwd) })
    },
    toast(args) {
      const severity = args.severity as 'info' | 'success' | 'warning' | 'error' | undefined
      showToast(String(args.message ?? ''), { severity, body: args.body as string | undefined })
    },
    async copy(args) {
      if (!(await clipboard.copy(String(args.text ?? '')))) throw new Error('The clipboard refused the text.')
    },
    async authoringPrompt(args) {
      const prompt = await Render('applets', {
        webhookPath: '',
        webhookSample: '',
        appletBrief: args.brief as AppletBrief,
      })
      return prompt.text
    },
    close: () => options.close(),
    open: () => applets.runCommand(appletCommandID(id)),
    dropdown(args) {
      if (options.surface() !== 'titlebar' || !frame.value)
        throw new Error('Only a title bar page can open a dropdown.')
      const applet = options.applet()
      applets.toggleDropdown({
        applet,
        entry: String(args.entry ?? applet.entry),
        width: Number(args.width ?? DEFAULT_DROPDOWN_WIDTH),
        height: Number(args.height ?? DEFAULT_DROPDOWN_HEIGHT),
        anchor: frame.value,
      })
    },
    resize(args) {
      if (options.surface() === 'page') throw new Error('A panel takes its size from applet.yaml.')
      options.resize({ width: args.width as number | undefined, height: args.height as number | undefined })
    },
    ready() {
      pageReady = true
      queuedCommands.splice(0).forEach(postCommand)
    },
    command(args) {
      const commandID = String(args.id)
      if (!commandById.value.has(commandID)) throw new Error(`hive has no command "${commandID}"`)
      applets.runCommand(commandID)
    },
    commands: () =>
      commands.value.map(({ id: commandID, title, group }) => ({
        id: commandID,
        title,
        group,
        keys: keybindings.combosFor(commandID),
      })),
    setCommands(args) {
      applets.setFrameRows(handle, id, parsePageRows(args.rows))
    },
    openPalette(args) {
      palette.openWithScope('all')
      palette.setQuery(String(args.query ?? ''))
    },
    navigate: (args) => router.push(String(args.path)).then(() => undefined),
    key({ continuing, ...init }) {
      // The sequence timed out while the page waited for its next key.
      if (continuing && keybindings.pendingSequence.value === null) return false
      window.dispatchEvent(new KeyboardEvent('keydown', { ...init, bubbles: true, cancelable: true }))
      return keybindings.pendingSequence.value !== null
    },
  }

  useEventListener(window, 'message', (event: MessageEvent) => {
    const source = frame.value?.contentWindow
    const message = event.data as {
      hiveApplet?: number
      token?: string
      id?: number
      method?: string
      args?: BridgeArgs
    } | null
    if (!source || event.source !== source || message?.hiveApplet !== 1) return
    const reply = (body: { result?: unknown; error?: string }) => {
      source.postMessage({ hiveApplet: 1, id: message.id, ...body }, '*')
    }
    void token.then(
      (expected) => {
        if (message.token !== expected) return
        const method = methods[message.method ?? '']
        if (!method) {
          reply({ error: `hive has no method "${message.method}"` })
          return
        }
        Promise.resolve()
          .then(() => method(message.args ?? {}, source))
          .then(
            (result) => reply({ result }),
            (cause: unknown) => reply({ error: cause instanceof Error ? cause.message : String(cause) }),
          )
      },
      () => {},
    )
  })

  watch(
    () => options.applet().revision,
    () => {
      endProcesses()
      pageReady = false
      applets.setFrameRows(handle, id, [])
    },
  )
  onBeforeUnmount(() => {
    closed = true
    disconnect()
    endProcesses()
  })
}
