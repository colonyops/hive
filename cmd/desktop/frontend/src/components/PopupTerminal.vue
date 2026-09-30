<script setup lang="ts">
import { computed, markRaw, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { Browser } from '@wailsio/runtime'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal, type IDisposable, type ILinkHandler } from '@xterm/xterm'
import IconTerminal from '~icons/lucide/terminal'
import IconX from '~icons/lucide/x'
import IconPower from '~icons/lucide/power'
import { usePopupTerminal } from '../composables/usePopupTerminal'
import { useTerminalFont } from '../composables/useTerminalFont'
import { useTheme } from '../composables/useTheme'
import { xtermTheme } from '../lib/terminalTheme'
import { decodeFrame, encodeInputFrames, type PopupTerminalState } from '../lib/popupTerminalClient'
import { loadTerminalFaces, terminalFontStack } from '../lib/terminalFaces'
import { claimAtlasRenderer } from '../lib/terminalRenderer'
import { installTerminalImages } from '../lib/terminalImages'
import '@xterm/xterm/css/xterm.css'

// The floating pop-up terminal: one PTY this process owns, rendered over
// whatever is on screen (ADR ephemeral-popup-terminals). Hiding it leaves the shell running — the
// panel is a view of the terminal, not the terminal's lifetime — so the only
// things that end it are the process exiting, End, and quitting Hive.

const { visible, checking, available, reason, client, request, launchSeq, hide, ready } = usePopupTerminal()
const {
  px: fontSizePx,
  family: fontFamily,
  weight: fontWeight,
  weightBold: fontWeightBold,
  lineHeight,
  letterSpacing,
} = useTerminalFont()
const { theme } = useTheme()

const MIN_WIDTH = 380
const MIN_HEIGHT = 220
const RESIZE_DEBOUNCE_MS = 80

// The share of the window a pop-up opens at, centred. It is a constant until
// there is a setting for it: the size is a preference, and 85% is the one that
// leaves the app readable behind a terminal big enough to work in.
const DEFAULT_SIZE_FRACTION = 0.85

const panel = ref<HTMLElement | null>(null)
const host = ref<HTMLElement | null>(null)
const term = shallowRef<Terminal | null>(null)
const terminal = ref<PopupTerminalState | null>(null)
const status = ref<'idle' | 'opening' | 'live' | 'ended'>('idle')
const error = ref('')
const endedReason = ref('')

let socket: WebSocket | null = null
let fit: FitAddon | null = null
let observer: ResizeObserver | null = null
let resizeTimer: ReturnType<typeof setTimeout> | undefined
// An atlas renderer is live on the pane. False after a claim that did not
// survive, which is what makes the next reveal retry it (ADR terminal-renderer-claimed-on-activation).
let rendered = false
// The launch the pane on screen belongs to. Behind launchSeq means someone has
// asked for a different terminal since, and the pane is showing the wrong one.
let renderedSeq = -1
// Where focus was when the pop-up took it, so dismissing the panel puts the
// caller back where they were rather than on the document body.
let focusReturn: HTMLElement | null = null
const disposers: IDisposable[] = []

const title = computed(() => terminal.value?.title || 'Terminal')
const subtitle = computed(() => terminal.value?.dir ?? '')

// The pane is laid out from the moment a terminal is being opened, not from the
// moment one is live: it has to be measured before the PTY is spawned, and a
// host inside a display:none subtree has no box to measure.
const paneLaidOut = computed(() => status.value === 'opening' || term.value !== null)

// The window's own size, tracked so the panel follows it. The box is derived
// from the viewport and nothing else: it cannot be dragged or resized, so there
// is no remembered geometry to go stale against a window that changed since.
const viewport = ref({ width: window.innerWidth, height: window.innerHeight })

const panelStyle = computed(() => {
  const width = Math.max(MIN_WIDTH, Math.round(viewport.value.width * DEFAULT_SIZE_FRACTION))
  const height = Math.max(MIN_HEIGHT, Math.round(viewport.value.height * DEFAULT_SIZE_FRACTION))
  return {
    width: `${width}px`,
    height: `${height}px`,
    left: `${Math.max(8, Math.round((viewport.value.width - width) / 2))}px`,
    top: `${Math.max(40, Math.round((viewport.value.height - height) / 2))}px`,
  }
})

async function openTerminal(): Promise<void> {
  if (!client.value || status.value === 'opening') return
  // A terminal that ended leaves its pane mounted so the last screen stays
  // readable; opening the next one is what releases it. The one before it is
  // closed rather than abandoned: a stream that dropped for a reason other than
  // the process exiting would otherwise leave a shell running with no view of
  // it and no way to reach it again.
  const previous = terminal.value?.id
  teardown()
  terminal.value = null
  if (previous) void client.value.close(previous).catch(() => {})

  status.value = 'opening'
  error.value = ''
  renderedSeq = launchSeq.value
  try {
    // Before the Terminal is constructed, not after: xterm measures its cell on
    // open and never re-measures, and the atlas renderer caches the glyphs that
    // were resident — a pane opened ahead of the face shows tofu where every
    // Nerd Font icon in a TUI should be.
    await loadTerminalFaces(fontFamily.value, fontSizePx.value, fontWeight.value, fontWeightBold.value)
    // Built and measured before the process exists, so the PTY is spawned on
    // the grid it will be drawn on. Sizing it afterwards is what a TUI sees as
    // a full screen at the fallback grid followed by a SIGWINCH reflow — the
    // pop-up appearing small and snapping wider a moment later.
    //
    // The tick is what gives the host its box, and xterm measures its cell on
    // open and never re-measures: a pane built a frame early measures nothing
    // and keeps that cell for the rest of its life.
    await nextTick()
    const created = buildPane()
    if (!created) {
      error.value = 'The terminal could not be rendered.'
      status.value = 'idle'
      return
    }
    const size = measurePane()
    if (size) created.resize(size.cols, size.rows)

    const opened = await client.value.open({ ...request.value, ...size })
    terminal.value = opened
    attachStream(created, opened)
    status.value = 'live'
    created.focus()
  } catch (failure) {
    teardown()
    status.value = 'idle'
    error.value = failure instanceof Error ? failure.message : 'The terminal could not be opened.'
  }
}

// The pane exists before the terminal does, because measuring it is what the
// launch needs.
function buildPane(): Terminal | null {
  if (!host.value) return null

  const created = markRaw(new Terminal({
    fontFamily: terminalFontStack(fontFamily.value),
    fontSize: fontSizePx.value,
    fontWeight: fontWeight.value,
    fontWeightBold: fontWeightBold.value,
    lineHeight: lineHeight.value,
    letterSpacing: letterSpacing.value,
    scrollback: 5000,
    theme: xtermTheme(),
    linkHandler,
  }))
  const fitAddon = markRaw(new FitAddon())
  created.loadAddon(fitAddon)
  created.loadAddon(markRaw(new WebLinksAddon((_event, uri) => openLink(uri))))
  created.open(host.value)
  loadRenderer(created)

  term.value = created
  fit = fitAddon
  return created
}

// The grid this pane's box holds, or null when there is nothing to measure:
// proposeDimensions on a host with no box answers a bogus tiny grid rather than
// failing, and opening a PTY at that is worse than letting the server default.
function measurePane(): { cols: number; rows: number } | null {
  if (!host.value?.clientWidth || !host.value.clientHeight) return null
  const proposed = fit?.proposeDimensions()
  if (!proposed?.cols || !proposed.rows) return null
  return { cols: proposed.cols, rows: proposed.rows }
}

function attachStream(created: Terminal, state: PopupTerminalState): void {
  if (!client.value || !host.value) return

  disposers.push(created.onData((data) => send(data)))
  // The server applies whatever size it is told, so the grid xterm measured is
  // the grid the process is given — there is nothing to reconcile afterwards.
  disposers.push(created.onResize(({ cols, rows }) => {
    void client.value?.resize(state.id, cols, rows).catch(() => {})
  }))

  socket = client.value.openStream(state.id)
  const capturedSocket = socket
  disposers.push({ dispose: installTerminalImages(host.value, {
    pasteText: (text) => created.paste(text),
    capture: () => ({
      current: () => visible.value && socket === capturedSocket && capturedSocket.readyState === WebSocket.OPEN && terminal.value?.id === state.id,
      paste: (text) => created.paste(text),
      focus: () => created.focus(),
    }),
  }) })
  socket.onmessage = (event: MessageEvent<ArrayBuffer>) => {
    const frame = decodeFrame(event.data)
    if (!frame) return
    if (frame.type === 'output') created.write(frame.data)
    else exited()
  }
  socket.onerror = () => fail('The terminal connection dropped.')
  socket.onclose = () => { if (status.value === 'live') fail('The terminal connection closed.') }

  // The observer keeps the grid on the box as the window changes; it is not
  // what establishes it, so it is armed after the launch rather than before.
  observer = new ResizeObserver(() => scheduleFit())
  observer.observe(host.value)
}

function send(data: string): void {
  if (socket?.readyState !== WebSocket.OPEN) return
  for (const frame of encodeInputFrames(data)) socket.send(frame)
}

function scheduleFit(): void {
  clearTimeout(resizeTimer)
  resizeTimer = setTimeout(() => {
    try {
      fit?.fit()
    } catch {
      // A panel mid-transition can measure to nothing; the next observation fits.
    }
  }, RESIZE_DEBOUNCE_MS)
}

// The shell exited, so the pop-up goes with it. Typing `exit` is how a terminal
// is dismissed, and leaving a dead pane on screen would make the quick way in
// the slow way out. Nothing is closed server-side: a process that exited took
// its terminal with it.
function exited(): void {
  teardown()
  status.value = 'idle'
  terminal.value = null
  hide()
}

// A stream that dropped for any other reason keeps the panel up: the shell may
// still be alive on the far side, and vanishing without saying so would look
// like the terminal simply closed itself.
function fail(why: string): void {
  if (status.value === 'ended') return
  status.value = 'ended'
  endedReason.value = why
  teardownStream()
}

// Ends the terminal on purpose, the deliberate counterpart to typing `exit`:
// the process is killed and the panel goes with it.
async function endTerminal(): Promise<void> {
  const id = terminal.value?.id
  exited()
  if (id) await client.value?.close(id).catch(() => {})
}

function teardownStream(): void {
  if (socket) {
    socket.onmessage = null
    socket.onerror = null
    socket.onclose = null
    socket.close()
    socket = null
  }
}

function teardown(): void {
  teardownStream()
  clearTimeout(resizeTimer)
  observer?.disconnect()
  observer = null
  for (const disposer of disposers.splice(0)) disposer.dispose()
  term.value?.dispose()
  term.value = null
  fit = null
  rendered = false
  endedReason.value = ''
}

// A link has to leave the webview: it hosts one document for the app's whole
// lifetime, and xterm's own default for an OSC 8 hyperlink — confirm() then
// window.open() — is answered by neither. WebLinksAddon covers the bare URLs
// xterm does not linkify on its own. Same rule as the session panes.
function openLink(uri: string): void {
  void Browser.OpenURL(uri).catch(() => {})
}

const linkHandler: ILinkHandler = { activate: (_event, uri) => openLink(uri) }

// A failed claim leaves `rendered` false, which is what makes the next reveal
// retry it rather than leaving the pane on the DOM renderer (ADR terminal-renderer-claimed-on-activation).
function loadRenderer(target: Terminal): void {
  claimAtlasRenderer(target, (addon) => disposers.push(addon), (claimed) => { rendered = claimed })
}

// What being shown means: a terminal, focused, in the box the panel opens at.
async function reveal(): Promise<void> {
  rememberFocus()
  // The first show races the availability probe, and a terminal cannot be
  // opened before the transport it opens over is known.
  await ready()
  if (!available.value || !visible.value) return
  await nextTick()
  // Asking for the pop-up is asking for a terminal, so a panel with none live
  // opens one rather than showing an empty box with a button in it. A live one
  // that belongs to an earlier launch is replaced: the caller asked for a
  // different terminal, and one pop-up is open at a time (ADR ephemeral-popup-terminals).
  if (status.value !== 'live' || renderedSeq !== launchSeq.value) {
    if (status.value !== 'opening') await openTerminal()
    return
  }
  // A renderer claim that failed earlier gets another chance every time the
  // pane comes back on screen (ADR terminal-renderer-claimed-on-activation).
  if (!rendered && term.value) loadRenderer(term.value)
  term.value?.focus()
}

// launchSeq is watched beside visible because a launcher invoked while the
// panel is already up changes what was asked for without changing whether the
// panel is on screen, and reveal is what notices.
watch([visible, launchSeq], ([open]) => {
  if (open) void reveal()
  else restoreFocus()
})

watch(theme, () => { if (term.value) term.value.options.theme = xtermTheme() })
// The faces have to be resident before xterm re-measures its cell against them,
// or it measures the outgoing font and the atlas caches glyphs at the wrong
// metrics (ADR terminal-atlas-renderer).
watch(
  [fontSizePx, fontFamily, fontWeight, fontWeightBold, lineHeight, letterSpacing],
  async ([px, family, weight, weightBold, height, spacing]) => {
    await loadTerminalFaces(family, px, weight, weightBold)
    if (!term.value) return
    term.value.options.fontFamily = terminalFontStack(family)
    term.value.options.fontSize = px
    term.value.options.fontWeight = weight
    term.value.options.fontWeightBold = weightBold
    term.value.options.lineHeight = height
    term.value.options.letterSpacing = spacing
    scheduleFit()
  },
)

// Only what was focused outside the panel is worth returning to; a re-reveal
// while the pane already has focus must not record the pane itself.
function rememberFocus(): void {
  const active = document.activeElement
  if (!(active instanceof HTMLElement) || panel.value?.contains(active)) return
  focusReturn = active
}

function restoreFocus(): void {
  const target = focusReturn
  focusReturn = null
  if (target?.isConnected) target.focus()
}

function onWindowResize(): void {
  viewport.value = { width: window.innerWidth, height: window.innerHeight }
}

onMounted(() => {
  window.addEventListener('resize', onWindowResize)
  // The panel is mounted lazily by the same action that shows it, so it comes up
  // with `visible` already true and the watcher above has nothing to react to.
  if (visible.value) void reveal()
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', onWindowResize)
  teardown()
})
</script>

<template>
  <Teleport to="body">
    <!-- v-show, not v-if: hiding the panel must not tear down the shell behind
         it, and remounting xterm would lose the screen either way. -->
    <section
      v-show="visible"
      ref="panel"
      class="fixed z-50 flex flex-col overflow-hidden rounded-xl border border-strong bg-pane shadow-2xl"
      :style="panelStyle"
      role="dialog"
      aria-label="Terminal"
      data-testid="popup-terminal"
    >
      <header
        class="flex h-9 shrink-0 select-none items-center gap-2 border-b border-row bg-raised px-3"
        data-testid="popup-terminal-header"
      >
        <IconTerminal class="size-3.5 shrink-0 text-text-4" />
        <span class="shrink-0 font-mono text-[12px] text-text">{{ title }}</span>
        <span class="truncate font-mono text-[11px] text-text-4">{{ subtitle }}</span>
        <div class="ml-auto flex shrink-0 items-center gap-1">
          <button
            v-if="status === 'live'"
            class="cursor-pointer rounded-[5px] p-1 text-text-4 hover:bg-chip hover:text-severity-error"
            title="End this terminal"
            aria-label="End this terminal"
            data-testid="popup-terminal-end"
            @click="endTerminal"
          ><IconPower class="size-3.5" /></button>
          <button
            class="cursor-pointer rounded-[5px] p-1 text-text-4 hover:bg-chip hover:text-text"
            title="Hide — the shell keeps running"
            aria-label="Hide the terminal"
            data-testid="popup-terminal-hide"
            @click="hide"
          ><IconX class="size-3.5" /></button>
        </div>
      </header>

      <div class="relative min-h-0 flex-1 bg-app">
        <!-- data-terminal-input-scope hands every key to the pane, so app
             shortcuts do not steal keys from whatever is running in it. -->
        <div
          v-show="paneLaidOut"
          ref="host"
          class="absolute inset-0 p-1.5"
          data-terminal-input-scope
          data-testid="popup-terminal-pane"
          @click="term?.focus()"
        />

        <div
          v-if="!term"
          class="flex h-full flex-col items-center justify-center gap-2 px-6 text-center"
        >
          <template v-if="checking">
            <p class="font-mono text-xs text-text-4">Checking…</p>
          </template>
          <template v-else-if="!available">
            <p class="max-w-[420px] text-xs leading-relaxed text-text-3" data-testid="popup-terminal-unavailable">{{ reason }}</p>
          </template>
          <template v-else-if="status === 'opening'">
            <p class="font-mono text-xs text-text-4">Opening a shell…</p>
          </template>
          <template v-else>
            <p v-if="error" class="max-w-[420px] text-xs leading-relaxed text-severity-error" data-testid="popup-terminal-error">{{ error }}</p>
            <button
              class="cursor-pointer rounded-[7px] bg-chip px-2.5 py-1 font-mono text-[11.5px] text-text hover:bg-strong"
              data-testid="popup-terminal-new"
              @click="openTerminal"
            >Open a terminal</button>
          </template>
        </div>

        <!-- The pane stays behind this strip so the last screen a command left
             is still readable after its process is gone. -->
        <div
          v-if="status === 'ended'"
          class="absolute inset-x-0 bottom-0 flex items-center gap-3 border-t border-row bg-raised px-3 py-2"
          data-testid="popup-terminal-ended"
        >
          <span class="truncate font-mono text-[11px] text-text-4">{{ endedReason }}</span>
          <button
            class="ml-auto shrink-0 cursor-pointer rounded-[5px] bg-chip px-2 py-0.5 font-mono text-[11.5px] text-text hover:bg-strong"
            data-testid="popup-terminal-new"
            @click="openTerminal"
          >New terminal</button>
        </div>
      </div>
    </section>
  </Teleport>
</template>
