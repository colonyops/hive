<script setup lang="ts">
import { useEventListener, useMutationObserver } from '@vueuse/core'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import IconX from '~icons/lucide/x'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import { useClipboard } from '@hive/ui/composables/useClipboard'
import { useEscapeToClose } from '@hive/ui/composables/useEscapeToClose'
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
import { useToasts } from '../../composables/useToasts'
import { usePopupTerminal } from '../../stores/usePopupTerminal'

// Hosts an applet's own page. The frame has an opaque origin, so a page can do what
// it likes to itself without reaching Hive's DOM or storage, and what it asks of the
// machine arrives here as messages (ADR applets-are-web-pages-with-full-access-to-the-machine).

type AppletBrief = NonNullable<Parameters<typeof Render>[1]['appletBrief']>

// eslint-disable-next-line @typescript-eslint/no-explicit-any -- the page is free-form; the services validate
type BridgeArgs = Record<string, any>

const props = defineProps<{ applet: Applet; sessionSlug: string }>()
const emit = defineEmits<{ close: [] }>()

const popupTerminal = usePopupTerminal()
const { showToast } = useToasts()
const clipboard = useClipboard()
const frame = ref<HTMLIFrameElement | null>(null)

// The frame can navigate to another page and keep its window, so the window does not
// prove who is asking. Only the applet's own pages were served this.
// AppletHost keys this component by applet id, so the id cannot change under it.
// eslint-disable-next-line vue/no-setup-props-reactivity-loss
const token = BridgeToken(props.applet.id)

const src = computed(
  () =>
    `/__applet/${encodeURIComponent(props.applet.id)}/${props.applet.entry.split('/').map(encodeURIComponent).join('/')}?r=${props.applet.revision}`,
)

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

useMutationObserver(
  document.documentElement,
  () => frame.value?.contentWindow?.postMessage({ hiveApplet: 1, theme: readTheme() }, '*'),
  { attributeFilter: ['data-theme'] },
)

const execRequest = (args: BridgeArgs) => ({
  command: String(args.command ?? ''),
  sessionSlug: props.sessionSlug,
  cwd: String(args.cwd ?? ''),
  env: (args.env ?? {}) as Record<string, string>,
  stdin: String(args.stdin ?? ''),
  timeoutMillis: Number(args.timeout ?? 0),
})

const methods: Record<string, (args: BridgeArgs) => unknown> = {
  context: () => Context(props.applet.id, props.sessionSlug),
  theme: readTheme,
  exec: (args) => Exec(props.applet.id, execRequest(args)),
  spawn: (args) => Spawn(props.applet.id, execRequest(args)),
  spawnRead: (args) => SpawnRead(String(args.handle)),
  spawnWrite: (args) => SpawnWrite(String(args.handle), String(args.data)),
  spawnKill: (args) => SpawnKill(String(args.handle)),
  readFile: (args) => ReadFile(props.applet.id, String(args.path)),
  writeFile: (args) => WriteFile(props.applet.id, String(args.path), String(args.content)),
  saveStorage: (args) => SaveStorage(props.applet.id, String(args.encoded)),
  async terminal(args) {
    const context = await Context(props.applet.id, props.sessionSlug)
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
  close: () => emit('close'),
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
        .then(() => method(message.args ?? {}))
        .then(
          (result) => reply({ result }),
          (cause: unknown) => reply({ error: cause instanceof Error ? cause.message : String(cause) }),
        )
    },
    () => {},
  )
})

// A page's Escape arrives through the bridge. This one is for focus outside the frame.
useEscapeToClose(() => emit('close'), { enabled: () => !popupTerminal.visible.value })

function focusFrame(): void {
  frame.value?.focus()
}

function endProcesses(): void {
  void KillSpawns(props.applet.id)
}

watch(() => props.applet.revision, endProcesses)
onBeforeUnmount(endProcesses)
</script>

<template>
  <div class="relative min-h-0 flex-1" data-testid="applet-page">
    <iframe
      ref="frame"
      :key="applet.revision"
      :src="src"
      :title="applet.title"
      sandbox="allow-scripts allow-forms allow-modals allow-popups allow-popups-to-escape-sandbox allow-downloads allow-pointer-lock"
      allow="autoplay; fullscreen; clipboard-read; clipboard-write; camera; microphone; gamepad"
      class="absolute inset-0 size-full border-0"
      data-testid="applet-frame"
      @load="focusFrame"
    />
    <IconButton
      class="absolute top-2 right-2 opacity-40 hover:opacity-100"
      label="Close"
      :icon="IconX"
      size="md"
      data-testid="applet-page-close"
      @click="emit('close')"
    />
  </div>
</template>
