<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { useResizeObserver } from '@vueuse/core'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import {
  createPopupTerminalClient,
  getPopupTerminalEndpoint,
  decodeFrame,
  encodeInputFrames,
  type PopupTerminalClient,
} from '../lib/popupTerminalClient'
import { xtermTheme } from '../lib/terminalTheme'
import { useTheme } from '../composables/useTheme'
import { useTerminalFont } from '../stores/useTerminalFont'
import { terminalFontStack, loadTerminalFaces } from '../lib/terminalFaces'
import '@xterm/xterm/css/xterm.css'

const props = defineProps<{ command: string; dir: string }>()
const emit = defineEmits<{ (e: 'close'): void }>()
const host = ref<HTMLElement | null>(null)
const status = ref('Starting investigation…')
const error = ref('')
const { theme } = useTheme()
const { px, family, weight, weightBold, lineHeight, letterSpacing } = useTerminalFont()
let terminal: Terminal | undefined
let fit: FitAddon | undefined
let client: PopupTerminalClient | undefined
let socket: WebSocket | undefined
let id = ''
let disposed = false
let resizeTimer: ReturnType<typeof setTimeout> | undefined

function resize(): void {
  if (disposed || !host.value || !terminal || !fit) return
  fit.fit()
  if (id)
    void client?.resize(id, terminal.cols, terminal.rows).catch((e: unknown) => {
      error.value = String(e)
    })
}

onMounted(async () => {
  try {
    const endpoint = await getPopupTerminalEndpoint()
    await loadTerminalFaces(family.value, px.value, weight.value, weightBold.value)
    if (disposed || !host.value) return
    client = createPopupTerminalClient(endpoint)
    terminal = new Terminal({
      theme: xtermTheme(),
      fontSize: px.value,
      fontFamily: terminalFontStack(family.value),
      fontWeight: weight.value,
      fontWeightBold: weightBold.value,
      lineHeight: lineHeight.value,
      letterSpacing: letterSpacing.value,
      cursorBlink: true,
      scrollback: 3000,
    })
    fit = new FitAddon()
    terminal.loadAddon(fit)
    terminal.open(host.value)
    fit.fit()
    const opened = await client.open({
      command: props.command,
      dir: props.dir,
      cols: terminal.cols,
      rows: terminal.rows,
    })
    id = opened.id
    if (disposed) {
      await client.close(id)
      return
    }
    socket = client.openStream(id)
    socket.onopen = () => {
      status.value = 'Investigation running'
      terminal?.focus()
    }
    socket.onmessage = (event: MessageEvent<ArrayBuffer>) => {
      const frame = decodeFrame(event.data)
      if (frame?.type === 'output') terminal?.write(frame.data)
      if (frame?.type === 'exit') status.value = `Agent ended${frame.reason ? `: ${frame.reason}` : ''}`
    }
    socket.onerror = () => {
      error.value = 'Could not connect to the agent terminal. Logs and Copy context remain available.'
    }
    socket.onclose = () => {
      if (status.value === 'Investigation running') status.value = 'Agent connection closed'
    }
    terminal.onData((data) => {
      if (socket?.readyState === WebSocket.OPEN) for (const frame of encodeInputFrames(data)) socket.send(frame)
    })
  } catch (e) {
    error.value = String(e)
    status.value = 'Investigation unavailable'
  }
})
useResizeObserver(host, () => {
  clearTimeout(resizeTimer)
  resizeTimer = setTimeout(resize, 80)
})
watch(theme, () => {
  if (terminal) terminal.options.theme = xtermTheme()
})
onBeforeUnmount(() => {
  disposed = true
  clearTimeout(resizeTimer)
  socket?.close()
  terminal?.dispose()
  if (id) void client?.close(id).catch(() => {})
})
</script>

<template>
  <section class="agent-panel" aria-label="Diagnostic investigation">
    <header>
      <span>{{ status }}</span
      ><button @click="emit('close')">End investigation</button>
    </header>
    <p v-if="error" role="alert">{{ error }}</p>
    <div ref="host" class="agent-terminal" />
  </section>
</template>

<style scoped>
.agent-panel {
  height: 310px;
  min-height: 160px;
  max-height: 65vh;
  resize: vertical;
  overflow: auto;
  flex-shrink: 0;
  border-bottom: 1px solid var(--hv-border);
  padding: 10px 16px;
  display: flex;
  flex-direction: column;
}
header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}
.agent-terminal {
  flex: 1;
  min-height: 80px;
}
p {
  color: var(--hv-severity-error);
}
button {
  cursor: pointer;
  color: var(--hv-accent);
}
</style>
