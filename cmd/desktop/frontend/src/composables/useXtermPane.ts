import { markRaw, shallowRef, watch, type Ref, type ShallowRef } from 'vue'
import { useResizeObserver } from '@vueuse/core'
import { Browser } from '@wailsio/runtime'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal, type IDisposable, type ILinkHandler, type ITerminalOptions } from '@xterm/xterm'
import { loadTerminalFaces, terminalFontStack } from '../lib/terminalFaces'
import { claimAtlasRenderer } from '../lib/terminalRenderer'
import { claimsShiftEnter } from '../lib/terminalKeys'
import { xtermTheme } from '../lib/terminalTheme'
import { useTerminalFont } from '../stores/useTerminalFont'
import { useTheme } from './useTheme'

const RESIZE_DEBOUNCE_MS = 80

export interface PaneSize {
  cols: number
  rows: number
}

// A link has to leave the webview: it hosts one document for the app's whole
// lifetime, and xterm's own default for an OSC 8 hyperlink -- confirm() then
// window.open() -- is answered by neither, so a click on one does nothing at
// all. WebLinksAddon covers the bare URLs xterm does not linkify on its own.
export function openLink(uri: string): void {
  void Browser.OpenURL(uri).catch(() => {})
}

const linkHandler: ILinkHandler = { activate: (_event, uri) => openLink(uri) }

/** A Terminal at the current font and theme, with links routed out of the webview. */
export function createTerminal(options: ITerminalOptions = {}): Terminal {
  const font = useTerminalFont()
  const term = markRaw(
    new Terminal({
      fontFamily: terminalFontStack(font.family.value),
      fontSize: font.px.value,
      fontWeight: font.weight.value,
      fontWeightBold: font.weightBold.value,
      lineHeight: font.lineHeight.value,
      letterSpacing: font.letterSpacing.value,
      scrollback: 5000,
      theme: xtermTheme(),
      linkHandler,
      ...options,
    }),
  )
  term.loadAddon(markRaw(new WebLinksAddon((_event, uri) => openLink(uri))))
  return term
}

// Before a Terminal is constructed, not after: xterm measures its cell on open
// and never re-measures, and the atlas renderer caches the glyphs that were
// resident, so a pane opened ahead of the face shows tofu for every Nerd Font
// icon.
export function loadTerminalFont(): Promise<void> {
  const font = useTerminalFont()
  return loadTerminalFaces(font.family.value, font.px.value, font.weight.value, font.weightBold.value)
}

/**
 * Keeps every terminal `terms` returns on the app's theme and font. `onFont`
 * runs after new font metrics land, since they change how many cells fit the
 * same box. Call it in a component or effect scope.
 */
export function watchTerminalAppearance(
  terms: () => Iterable<Terminal>,
  onFont: () => void,
  onTheme?: () => void,
): void {
  const { theme } = useTheme()
  const { px, family, weight, weightBold, lineHeight, letterSpacing } = useTerminalFont()
  watch(theme, () => {
    const palette = xtermTheme()
    for (const term of terms()) term.options.theme = palette
    onTheme?.()
  })
  // The faces have to be resident before xterm re-measures its cell against
  // them, or it measures the outgoing font and the atlas caches glyphs at the
  // wrong metrics (ADR terminal-atlas-renderer).
  watch(
    [px, family, weight, weightBold, lineHeight, letterSpacing],
    async ([size, face, regular, bold, height, spacing]) => {
      await loadTerminalFaces(face, size, regular, bold)
      const current = [...terms()]
      if (!current.length) return
      for (const term of current) {
        term.options.fontFamily = terminalFontStack(face)
        term.options.fontSize = size
        term.options.fontWeight = regular
        term.options.fontWeightBold = bold
        term.options.lineHeight = height
        term.options.letterSpacing = spacing
      }
      onFont()
    },
  )
}

/**
 * One xterm pane on `host` with one stream socket: the shape the pop-up
 * terminal and the Chats pane share. `onResize` runs debounced whenever the
 * host box or the font changes once a stream is attached.
 */
export function useXtermPane(host: Ref<HTMLElement | null>, onResize: () => void) {
  const term: ShallowRef<Terminal | null> = shallowRef(null)
  let fit: FitAddon | null = null
  let socket: WebSocket | null = null
  let resizeTimer: ReturnType<typeof setTimeout> | undefined
  // An atlas renderer is live on the pane. False after a claim that did not
  // survive, which is what makes claimRenderer retry it (ADR
  // terminal-renderer-claimed-on-activation).
  let rendered = false
  const disposers: IDisposable[] = []
  // The observer keeps the grid on the box as the window changes; it is not
  // what establishes it, so it is armed by attach rather than by build.
  const observedHost = shallowRef<HTMLElement | null>(null)
  useResizeObserver(observedHost, scheduleResize)
  watchTerminalAppearance(() => (term.value ? [term.value] : []), scheduleResize)

  function scheduleResize(): void {
    clearTimeout(resizeTimer)
    resizeTimer = setTimeout(onResize, RESIZE_DEBOUNCE_MS)
  }

  // The pane exists before the process does, because measuring it is what the
  // launch needs.
  function build(): Terminal | null {
    if (!host.value) return null
    const created = createTerminal()
    const fitAddon = markRaw(new FitAddon())
    created.loadAddon(fitAddon)
    created.attachCustomKeyEventHandler((event) => !claimsShiftEnter(created, event))
    created.open(host.value)
    loadRenderer(created)
    term.value = created
    fit = fitAddon
    return created
  }

  // Null when there is nothing to measure: proposeDimensions on a host with no
  // box answers a bogus tiny grid rather than failing.
  function measure(): PaneSize | null {
    if (!host.value?.clientWidth || !host.value.clientHeight) return null
    const proposed = fit?.proposeDimensions()
    if (!proposed?.cols || !proposed.rows) return null
    return { cols: proposed.cols, rows: proposed.rows }
  }

  function fitToHost(): void {
    try {
      fit?.fit()
    } catch {
      // A box mid-transition can measure to nothing; the next observation fits.
    }
  }

  function loadRenderer(target: Terminal): void {
    claimAtlasRenderer(
      target,
      (addon) => disposers.push(addon),
      (claimed) => {
        rendered = claimed
      },
    )
  }

  function claimRenderer(): void {
    if (!rendered && term.value) loadRenderer(term.value)
  }

  function attach(opened: WebSocket): void {
    socket = opened
    observedHost.value = host.value
  }

  function send(frames: Iterable<Uint8Array<ArrayBuffer>>): void {
    if (socket?.readyState !== WebSocket.OPEN) return
    for (const frame of frames) socket.send(frame)
  }

  function closeStream(): void {
    if (!socket) return
    socket.onmessage = null
    socket.onerror = null
    socket.onclose = null
    socket.close()
    socket = null
  }

  function teardown(): void {
    closeStream()
    clearTimeout(resizeTimer)
    observedHost.value = null
    for (const disposer of disposers.splice(0)) disposer.dispose()
    term.value?.dispose()
    term.value = null
    fit = null
    rendered = false
  }

  return {
    term,
    build,
    measure,
    fitToHost,
    claimRenderer,
    track: (disposer: IDisposable) => disposers.push(disposer),
    attach,
    socket: () => socket,
    send,
    closeStream,
    teardown,
  }
}
