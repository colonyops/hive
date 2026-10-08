<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import type { Terminal } from '@xterm/xterm'
import IconPower from '~icons/lucide/power'
import { loadTerminalFont, useXtermPane } from '../composables/useXtermPane'
import {
  createPopupTerminalClient,
  decodeFrame,
  encodeInputFrames,
  getPopupTerminalEndpoint,
  type PopupTerminalClient,
} from '../lib/popupTerminalClient'
import BaseBadge from './ui/BaseBadge.vue'
import IconButton from './ui/IconButton.vue'
import InlineError from './ui/InlineError.vue'
import '@xterm/xterm/css/xterm.css'

const props = defineProps<{ command: string; dir: string }>()
const emit = defineEmits<{ close: [] }>()
const host = ref<HTMLElement | null>(null)
const status = ref('Starting…')
const error = ref('')
const pane = useXtermPane(host, resize)
let client: PopupTerminalClient | undefined
let id = ''
let disposed = false

function resize(): void {
  if (disposed) return
  pane.fitToHost()
  const terminal = pane.term.value
  if (id && terminal) {
    void client?.resize(id, terminal.cols, terminal.rows).catch((failure: unknown) => {
      error.value = String(failure)
    })
  }
}

function attachStream(terminal: Terminal): void {
  if (!client) return
  const socket = client.openStream(id)
  socket.onopen = () => {
    status.value = 'Running'
    terminal.focus()
  }
  socket.onmessage = (event: MessageEvent<ArrayBuffer>) => {
    const frame = decodeFrame(event.data)
    if (frame?.type === 'output') terminal.write(frame.data)
    if (frame?.type === 'exit') {
      status.value = `Ended${frame.reason ? `: ${frame.reason}` : ''}`
      pane.closeStream()
    }
  }
  socket.onerror = () => {
    error.value = 'Could not connect to the agent terminal. Logs and context actions remain available.'
  }
  socket.onclose = () => {
    if (status.value === 'Running') status.value = 'Connection closed'
  }
  pane.track(terminal.onData((data) => pane.send(encodeInputFrames(data))))
  pane.track(
    terminal.onResize(({ cols, rows }) => {
      void client?.resize(id, cols, rows).catch(() => {})
    }),
  )
  pane.attach(socket)
}

async function start(): Promise<void> {
  try {
    const endpoint = await getPopupTerminalEndpoint()
    await loadTerminalFont()
    await nextTick()
    if (disposed) return
    const terminal = pane.build()
    if (!terminal) throw new Error('The investigation terminal could not be rendered.')
    const size = pane.measure()
    if (size) terminal.resize(size.cols, size.rows)
    client = createPopupTerminalClient(endpoint)
    const opened = await client.open({ command: props.command, dir: props.dir, ...size })
    id = opened.id
    if (disposed) {
      await client.close(id)
      return
    }
    attachStream(terminal)
  } catch (failure) {
    status.value = 'Unavailable'
    error.value = String(failure)
  }
}

async function end(): Promise<void> {
  const current = id
  id = ''
  pane.closeStream()
  if (current) await client?.close(current).catch(() => {})
  emit('close')
}

onMounted(() => void start())
onBeforeUnmount(() => {
  disposed = true
  pane.teardown()
  if (id) void client?.close(id).catch(() => {})
})
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col" data-testid="diagnostics-agent-pane">
    <div class="flex h-9 shrink-0 items-center gap-2 border-b border-row bg-canvas-toolbar px-4">
      <h2 class="font-medium text-text">Investigation</h2>
      <BaseBadge
        :tone="error ? 'danger' : status === 'Running' ? 'success' : 'muted'"
        variant="pill"
        dot
        class="px-2 py-0.5 font-mono text-micro"
        data-testid="diagnostics-agent-status"
      >
        {{ status }}
      </BaseBadge>
      <IconButton
        class="ml-auto"
        label="End investigation"
        :icon="IconPower"
        tone="danger"
        size="md"
        data-testid="diagnostics-agent-end"
        @click="end"
      />
    </div>
    <InlineError
      v-if="error"
      :message="error"
      variant="line"
      class="shrink-0 border-b border-row px-3 py-2"
      testid="diagnostics-agent-error"
    />
    <div
      ref="host"
      class="min-h-0 flex-1 bg-app p-1.5"
      data-terminal-input-scope
      data-testid="diagnostics-agent-terminal"
      @click="pane.term.value?.focus()"
    />
  </div>
</template>
