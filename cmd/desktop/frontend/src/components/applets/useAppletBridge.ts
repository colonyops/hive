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
import { SURFACES, type AppletSurface } from '../../stores/appletSurfaces'
import { BUILDER_ID, useApplets, type AppletFrameHandle, type AppletPageRow } from '../../stores/useApplets'
import { usePopupTerminal } from '../../stores/usePopupTerminal'

export const DEFAULT_DROPDOWN_WIDTH = 320
export const DEFAULT_DROPDOWN_HEIGHT = 360
const PORT_LINGER_MS = 1000

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

const rowText = (value: unknown): string =>
  typeof value === 'string' || typeof value === 'number' ? String(value) : ''
const optionalString = (value: unknown): string | undefined => (typeof value === 'string' ? value : undefined)

// A row goes in the palette, so one with no id or title should say so to the page that sent
// it rather than draw a blank row. A scope this Hive does not know falls back to the default,
// and a field of the wrong type is left out, so a page for a later Hive still fills its rows
// and one bad field cannot throw inside the palette.
function parsePageRows(rows: unknown): AppletPageRow[] {
  if (!Array.isArray(rows)) throw new Error('setCommands takes a list of rows.')
  return rows.map((row: Record<string, unknown> | null) => {
    const id = rowText(row?.id)
    const title = rowText(row?.title)
    if (!row || !id || !title) throw new Error('A palette row needs an id and a title.')
    return {
      id,
      title,
      group: optionalString(row.group),
      keywords: Array.isArray(row.keywords) ? row.keywords.filter((word) => typeof word === 'string') : undefined,
      icon: optionalString(row.icon),
      hint: optionalString(row.hint),
      kind: optionalString(row.kind),
      scope: row.scope === 'goto' || row.scope === 'actions' ? row.scope : undefined,
    }
  })
}

// What a page's relative cwd means for a command it runs: under where its commands run.
function terminalDir(cwd: unknown, context: { cwd: string }): string {
  const asked = typeof cwd === 'string' ? cwd : ''
  if (asked === '') return context.cwd
  return asked.startsWith('/') || asked.startsWith('~') ? asked : `${context.cwd}/${asked}`
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

  // The page's own end of the channel. It stays with that page when the frame navigates, so
  // nothing here can reach what loads next.
  let port: MessagePort | null = null

  function post(message: object): void {
    port?.postMessage({ hiveApplet: 1, ...message })
  }

  useMutationObserver(document.documentElement, () => post({ theme: readTheme() }), {
    attributeFilter: ['data-theme'],
  })

  const keybindings = useKeybindings()
  const palette = useCommandPalette()

  // A page cannot hear anything until its bridge has loaded, and says when it has.
  let pageReady = false
  const queuedCommands: string[] = []

  function postCommand(command: string): void {
    post({ event: 'command', data: command })
  }

  const handle: AppletFrameHandle = {
    surface: options.surface(),
    post,
    deliverCommand(command) {
      if (pageReady) postCommand(command)
      else queuedCommands.push(command)
    },
  }
  const disconnect = applets.connectFrame(id, handle)

  watch(options.sessionSlug, (slug) => {
    post({ event: 'session', data: slug })
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

  function others(message: object): void {
    for (const peer of applets.otherFrames(id, handle)) peer.post(message)
  }

  const methods: Record<string, (args: BridgeArgs) => unknown> = {
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
    storageChanged: (args) => others({ storage: args as object }),
    broadcast: (args) => others({ event: 'message', data: args.data as unknown }),
    async terminal(args) {
      const context = await Context(id, options.sessionSlug())
      popupTerminal.launch({
        command: String(args.command ?? ''),
        dir: terminalDir(args.cwd, context),
        env: [`HIVE_APPLET_DIR=${context.dir}`],
      })
    },
    toast(args) {
      const severity = args.severity as 'info' | 'success' | 'warning' | 'error' | undefined
      showToast(String(args.message ?? ''), { severity, body: args.body as string | undefined })
    },
    async copy(args) {
      if (!(await clipboard.copy(String(args.text ?? '')))) throw new Error('The clipboard refused the text.')
    },
    // The Applet Builder's own call, kept off the API so its shape can change with the
    // prompt. A directory that replaces the Builder keeps it.
    async authoringPrompt(args) {
      if (id !== BUILDER_ID) throw new Error('hive.authoringPrompt is only for the Applet Builder.')
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
      if (!SURFACES[options.surface()].canOpenDropdown || !frame.value)
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
      if (!SURFACES[options.surface()].resizable) throw new Error('A panel takes its size from applet.yaml.')
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
        // An override is a reactive array, which postMessage cannot clone.
        keys: [...keybindings.combosFor(commandID)],
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

  function serve(event: MessageEvent, from: MessagePort): void {
    const message = event.data as { id?: number; method?: string; args?: BridgeArgs }
    // A page saves its storage as it goes away, which can land after the frame has.
    if (closed && message.method !== 'saveStorage') return
    const reply = (body: { result?: unknown; error?: string }) => {
      try {
        from.postMessage({ hiveApplet: 1, id: message.id, ...body })
      } catch (cause) {
        // What a method returned cannot cross. The page must not wait forever for it.
        from.postMessage({ hiveApplet: 1, id: message.id, error: `hive could not send its reply: ${String(cause)}` })
      }
    }
    const method = methods[message.method ?? '']
    if (!method) {
      reply({ error: `hive has no method "${message.method}"` })
      return
    }
    Promise.resolve()
      .then(() => method(message.args ?? {}))
      .then(
        (result) => reply({ result }),
        (cause: unknown) => reply({ error: cause instanceof Error ? cause.message : String(cause) }),
      )
  }

  // The only message that arrives on the window. It proves the page with the token, and
  // hands over the port everything else uses.
  useEventListener(window, 'message', (event: MessageEvent) => {
    const message = event.data as { hiveApplet?: number; token?: string; method?: string } | null
    const [from] = event.ports
    if (event.source !== frame.value?.contentWindow || message?.hiveApplet !== 1) return
    if (message.method !== 'connect' || !from) return
    void token.then(
      (expected) => {
        if (message.token !== expected || closed) return
        port?.close()
        port = from
        from.onmessage = (received) => serve(received, from)
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
    // Left open for now, for the save a page sends as it goes away.
    const last = port
    setTimeout(() => last?.close(), PORT_LINGER_MS)
  })
}
