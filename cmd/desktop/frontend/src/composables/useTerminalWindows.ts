import { effectScope, markRaw, nextTick, ref, type Ref } from 'vue'
import { SearchAddon, type ISearchOptions } from '@xterm/addon-search'
import type { IDisposable, Terminal } from '@xterm/xterm'
// Rides the async terminal chunk on purpose: ~10MB of glyphs nobody pays for
// until they open Terminal mode.
import {
  decodeFrame,
  encodeInputFrames,
  encodePasteFrames,
  TerminalRequestError,
  type PaneDirection,
  type PaneLayout,
  type SplitDirection,
  type TerminalClient,
  type WindowEventKind,
  type WindowState,
} from '../lib/terminalClient'
import { resetTerminalFacesForTests } from '../lib/terminalFaces'
import { proposeGrid, terminalCellSize, type CellSize } from '../lib/terminalGrid'
import { activePaneOf, paneGrids, windowPanes } from '../lib/terminalLayout'
import { claimAtlasRenderer } from '../lib/terminalRenderer'
import { TerminalOutputWriter } from '../lib/terminalOutput'
import { installTerminalImages } from '../lib/terminalImages'
import { pasteTerminalImage } from '../lib/terminalImagesClient'
import { appClaimsPaneKey, claimsShiftEnter } from '../lib/terminalKeys'
import { silenceDeviceReports } from '../lib/terminalReports'
import { scrolledOffTail } from '../lib/terminalTail'
import { paneMayAutoFocus } from '../lib/terminalTree'
import { searchHighlightColors } from '../lib/terminalTheme'
import { resizeTerminalPreservingViewport } from '../lib/terminalViewport'
import { useTerminalFont } from '../stores/useTerminalFont'
import { useTerminalOutcomes, type TerminalEndReason } from '../stores/useTerminalOutcomes'
export type { TerminalEndReason } from '../stores/useTerminalOutcomes'
import { createTerminal, loadTerminalFont, watchTerminalAppearance } from './useXtermPane'

/**
 * 'ended' is terminal: this view has no stream any more — whether or not the
 * control client behind it survived — and the only way forward is reconnect().
 */
export type TerminalStatus = 'connecting' | 'live' | 'ended'

export interface TerminalSize {
  cols: number
  rows: number
}

/**
 * tmux answered a size vote with a different grid, so this pane is rendering a
 * window some other client's size decided. Not an error — the rule needs
 * naming, or it reads as a rendering bug.
 */
export interface TerminalSizeConstraint {
  voted: TerminalSize
  granted: TerminalSize
}

/** One tmux pane: one emulator, placed by its window's layout. */
export interface TerminalPane {
  // Unique per Terminal instance so reconnect mounts a fresh host for the new
  // emulator.
  uid: number
  paneId: string
  term: Terminal
  scrolledUp: boolean
}

export interface TerminalWindowTab {
  // Unique per Terminal instance, not per tmux window: reconnect rebuilds the
  // terminals under the same window ids, and a v-for keyed on the window id
  // would reuse the host element and never open the new Terminal on it.
  uid: number
  windowId: string
  name: string
  active: boolean
  // Clicks update tmux's active pane optimistically; stream events retarget
  // focus.
  activePane: string
  width: number
  height: number
  zoomed: boolean
  layout: PaneLayout | null
  panes: TerminalPane[]
}

/**
 * The find bar, which searches one pane at a time: the active window's active
 * pane, scrollback included. `matches` is -1 when there are more than the
 * addon will highlight, and `index` is the 1-based position of the current
 * match, 0 for none.
 */
export interface TerminalSearch {
  open: boolean
  query: string
  matches: number
  index: number
}

export interface UseTerminalWindows {
  tabs: Ref<TerminalWindowTab[]>
  activeWindowId: Ref<string>
  status: Ref<TerminalStatus>
  // True once a terminal has processed output. 'live' is not enough to swap a
  // held pane onto this session: the socket opens before the first-paint
  // capture lands, and swapping then shows a blank grid for a frame.
  painted: Ref<boolean>
  endReason: Ref<TerminalEndReason | null>
  error: Ref<string | null>
  actionError: Ref<string | null>
  sizeConstraint: Ref<TerminalSizeConstraint | null>
  dismissSizeConstraint: () => void
  // True from a degraded lifecycle frame until it is dismissed: output was
  // dropped and these panes were repainted from tmux rather than streamed.
  outputDropped: Ref<boolean>
  dismissOutputDropped: () => void
  // The pixel size of one cell, measured off the shown window's panes, and what
  // turns a layout's cells into pane boxes. Null until a pane has measured.
  cell: Ref<CellSize | null>
  search: Ref<TerminalSearch>
  openSearch: () => void
  closeSearch: () => void
  setSearchQuery: (query: string) => void
  findNext: () => void
  findPrevious: () => void
  start: () => Promise<void>
  reconnect: () => Promise<void>
  reportOutcome: (reason: TerminalEndReason, detail: string) => void
  select: (windowId: string) => Promise<void>
  newWindow: (command?: string) => Promise<void>
  newAgentWindow: (agent: string) => Promise<void>
  closeWindow: (windowId: string) => Promise<void>
  rename: (windowId: string, name: string) => Promise<void>
  moveWindow: (windowId: string, position: number) => Promise<void>
  attachTab: (windowId: string, host: HTMLElement) => void
  attachPane: (paneId: string, host: HTMLElement) => void
  disposeTab: (windowId: string) => void
  /** Make a pane its window's active pane and put the keyboard in it. */
  selectPane: (paneId: string) => Promise<void>
  /** Split the active window's active pane; horizontal puts the new pane to the right, vertical below. */
  splitPane: (direction: SplitDirection) => Promise<void>
  /** Close a pane, the active window's active one by default; the last pane of a window closes the window. */
  closePane: (paneId?: string) => Promise<void>
  zoomPane: () => Promise<void>
  focusPane: (direction: PaneDirection) => Promise<void>
  /** Set a pane's size in cells; a divider drag names the pane before it. */
  resizePane: (paneId: string, size: { width?: number; height?: number }) => Promise<void>
  focusActive: () => void
  scrollToBottom: () => void
  dispose: () => void
}

// The grid a pane renders at while tmux has reported none for it, and only
// then. Every path that learns tmux's size overrides it.
const DEFAULT_SIZE: TerminalSize = { cols: 80, rows: 24 }
const RESIZE_DEBOUNCE_MS = 80
// How long tmux's answer to a vote is waited for before the difference is
// reported as a constraint. A vote is answered by a window event, which arrives
// well inside this; an unanswered vote means something else decided the size.
const CONSTRAINT_SETTLE_MS = 750

interface WindowRuntime {
  host?: HTMLElement
  observer?: ResizeObserver
}

interface PaneRuntime {
  windowId: string
  host?: HTMLElement
  // The same Terminal the pane holds, reachable without going through the
  // reactive tabs array — see refreshScrolledUp.
  term: Terminal
  finder: SearchAddon
  output: TerminalOutputWriter
  disposers: IDisposable[]
  // An atlas renderer is live on this terminal. False after a context loss the
  // canvas claim did not survive, which is what makes the next activation
  // retry instead of leaving the pane on the DOM renderer. ADR terminal-renderer-claimed-on-activation.
  rendered?: boolean
  // The last value written to the pane's reactive `scrolledUp`, held raw so the
  // per-line refresh can tell "unchanged" without touching a Vue proxy. See
  // refreshScrolledUp.
  scrolledUp?: boolean
}

// The pane box belongs to the app window, not to a session, so one remembered
// vote serves every session — including one being attached for the first time.
// It is keyed by every typography setting that moves the cell, because a vote
// counted against one set of cell metrics is wrong under another.
const VOTE_KEY = 'hive.terminal.vote'
// tmux's own bound on a client dimension. A stored value outside it would fail
// the attach, and the session would land on the error overlay instead.
const MAX_DIMENSION = 1000

let nextUID = 1

function rememberedVote(metrics: string): TerminalSize | null {
  try {
    const stored = JSON.parse(localStorage.getItem(VOTE_KEY) ?? 'null') as {
      cols?: number
      rows?: number
      metrics?: string
    } | null
    if (!stored || stored.metrics !== metrics) return null
    if (!validDimension(stored.cols) || !validDimension(stored.rows)) return null
    return { cols: stored.cols, rows: stored.rows }
  } catch {
    // unparseable or unreadable storage is simply no memory
    return null
  }
}

function rememberVote(size: TerminalSize, metrics: string): void {
  try {
    localStorage.setItem(VOTE_KEY, JSON.stringify({ ...size, metrics }))
  } catch {
    // storage denied; the vote is re-measured next attach either way
  }
}

function validDimension(value: number | undefined): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value > 0 && value <= MAX_DIMENSION
}

export function useTerminalWindows(slug: string, client: TerminalClient): UseTerminalWindows {
  const tabs = ref<TerminalWindowTab[]>([]) as Ref<TerminalWindowTab[]>
  const activeWindowId = ref('')
  const status = ref<TerminalStatus>('connecting')
  const painted = ref(false)
  const endReason = ref<TerminalEndReason | null>(null)
  const error = ref<string | null>(null)
  const actionError = ref<string | null>(null)
  const sizeConstraint = ref<TerminalSizeConstraint | null>(null)
  const outputDropped = ref(false)
  const cell = ref<CellSize | null>(null)
  const search = ref<TerminalSearch>({ open: false, query: '', matches: 0, index: 0 })

  const windows = new Map<string, WindowRuntime>()
  const panes = new Map<string, PaneRuntime>()
  const expectedWindowClosures = new Set<string>()
  const expectedPaneClosures = new Set<string>()
  const scope = effectScope(true)
  let socket: WebSocket | null = null
  let disposed = false
  let connectionEpoch = 0
  const outcomes = useTerminalOutcomes()

  const { cellMetrics } = useTerminalFont()

  // The last size this client voted for: a request, never the size anything
  // renders at. It opens at the last measured vote, and null — nothing measured
  // and nothing remembered — is a real state rather than a placeholder, because
  // tmux obeys the attach vote and would resize the session to it.
  let vote: TerminalSize | null = rememberedVote(cellMetrics())
  let resizeTimer: ReturnType<typeof setTimeout> | undefined
  let constraintTimer: ReturnType<typeof setTimeout> | undefined
  let constraintDismissed = false
  // terminalGrid.ts reads xterm's private _core, which no test against a fake
  // terminal can notice moving; an opened, visible pane with no cell is the
  // one signal there is.
  let warnedUnmeasuredCell = false
  // A window created from the toolbar is only knowable by id once tmux
  // announces it, so the intent to focus it is parked until then.
  let pendingActivate = ''
  // New-window commands deferred until tmux reports the target pane.
  const pendingCommands = new Map<string, () => boolean>()

  // New cell metrics change how many cells fit the same box, so the vote
  // must re-run; the grid itself stays at tmux's size until tmux answers.
  // A decoration keeps the colour it was drawn with, so live search highlights
  // re-run on a theme change rather than wait for the next keystroke.
  scope.run(() =>
    watchTerminalAppearance(
      () => [...panes.values()].map((state) => state.term),
      scheduleVote,
      () => {
        if (search.value.open) runSearch('incremental')
      },
    ),
  )

  function findTab(windowId: string): TerminalWindowTab | undefined {
    return tabs.value.find((tab) => tab.windowId === windowId)
  }

  function findPane(paneId: string): { tab: TerminalWindowTab; pane: TerminalPane } | undefined {
    for (const tab of tabs.value) {
      const pane = tab.panes.find((candidate) => candidate.paneId === paneId)
      if (pane) return { tab, pane }
    }
    return undefined
  }

  function activePaneId(): string {
    const tab = findTab(activeWindowId.value)
    return tab ? (activePaneOf(tab)?.paneId ?? '') : ''
  }

  function createTab(state: WindowState): TerminalWindowTab {
    windows.set(state.windowId, {})
    const grids = paneGrids(state)
    return {
      uid: nextUID++,
      windowId: state.windowId,
      name: state.name,
      active: state.active,
      activePane: state.activePane,
      width: state.width,
      height: state.height,
      zoomed: state.zoomed,
      layout: state.layout,
      panes: windowPanes(state).map((paneId) => createPane(state.windowId, paneId, grids.get(paneId))),
    }
  }

  function createPane(windowId: string, paneId: string, grid: TerminalSize | undefined): TerminalPane {
    // registerDecoration is still proposed API, and every find highlights
    // through it -- without this the first findNext throws and search is dead.
    const term = createTerminal({ allowProposedApi: true })
    const finder = markRaw(new SearchAddon())
    term.loadAddon(finder)
    const output = markRaw(
      new TerminalOutputWriter((data) => {
        if (painted.value) term.write(data)
        else
          term.write(data, () => {
            painted.value = true
          })
      }),
    )
    // Before any output reaches it: xterm re-wraps its buffer on resize, so a
    // grid sized after the first paint mangles the snapshot it just drew.
    term.resize(grid?.cols || unreportedSize().cols, grid?.rows || unreportedSize().rows)
    term.attachCustomKeyEventHandler((event: KeyboardEvent) => {
      if (event.type !== 'keydown') return !claimsShiftEnter(term, event)
      // These fire from App.vue's window listener, which runs after this one.
      // Returning false only stops xterm from *also* sending the key to the
      // pane — Ctrl+Shift+K would otherwise arrive as 0x0B, and the `i` of
      // ⌘G I as text.
      if (appClaimsPaneKey(event)) return false
      // After the chords: a rebind can put a piercing command on shift+enter.
      return !claimsShiftEnter(term, event)
    })
    panes.set(paneId, {
      windowId,
      term,
      finder,
      output,
      disposers: [
        finder,
        { dispose: () => output.dispose() },
        silenceDeviceReports(term),
        term.onData((data: string) => sendInput(paneId, data)),
        // onScroll covers what output does to the buffer — the auto-pin to the
        // tail, and a trim moving it — but *not* the user scrolling: xterm's
        // viewport syncs the buffer from its own DOM scroll handler and
        // suppresses the event to avoid feeding itself. attachPane listens to
        // that DOM scroll for the other half.
        term.onScroll(() => refreshScrolledUp(paneId)),
        // Entering the alternate screen has no scrollback and fires no scroll
        // event on the way in.
        term.buffer.onBufferChange(() => refreshScrolledUp(paneId)),
        // Fires as output lands too, not just on a new query: a match count is
        // only true of the buffer it was counted in.
        finder.onDidChangeResults(({ resultIndex, resultCount }) => {
          if (activePaneId() !== paneId) return
          search.value = { ...search.value, matches: resultCount, index: resultIndex + 1 }
        }),
      ],
    })
    return { uid: nextUID++, paneId, term, scrolledUp: false }
  }

  // Runs once per rendered line of output, per streaming pane — xterm fires
  // onScroll for every line feed that reaches the bottom of the scroll region,
  // and hidden pooled panes keep parsing, so this is the hottest app-owned path
  // there is. Everything reactive is therefore behind an unchanged-value guard
  // read from the raw runtime record: the steady state (pinned to the tail,
  // nothing to say) touches no Vue proxy at all.
  //
  // The write itself still goes through findPane, because it has to mutate the
  // reactive proxy rather than the raw object createPane returned — a raw write
  // would leave the pill stale.
  function refreshScrolledUp(paneId: string): void {
    const state = panes.get(paneId)
    if (!state) return
    const scrolledUp = scrolledOffTail(state.term)
    if (scrolledUp === state.scrolledUp) return
    state.scrolledUp = scrolledUp
    const found = findPane(paneId)
    if (found) found.pane.scrolledUp = scrolledUp
  }

  // Keep xterm at tmux-reported pane grids. Ignore zero-size placeholders until
  // reconciliation supplies a real layout.
  function applyPaneGrids(tab: TerminalWindowTab): void {
    const grids = paneGrids(tab)
    for (const pane of tab.panes) {
      const grid = grids.get(pane.paneId)
      if (!grid?.cols || !grid.rows) continue
      if (pane.term.cols === grid.cols && pane.term.rows === grid.rows) continue
      resizeTerminalPreservingViewport(pane.term, grid.cols, grid.rows)
      refreshScrolledUp(pane.paneId)
    }
  }

  function sendInput(paneId: string, data: string): void {
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    for (const frame of encodeInputFrames(paneId, data)) socket.send(frame)
  }

  function sendPaste(paneId: string, text: string): void {
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    for (const frame of encodePasteFrames(paneId, text)) socket.send(frame)
  }

  function setActive(windowId: string): void {
    if (activeWindowId.value !== windowId) clearHighlights()
    activeWindowId.value = windowId
    for (const tab of tabs.value) tab.active = tab.windowId === windowId
    showRenderers(windowId)
    // A search belongs to the buffer it ran against, so switching tabs re-runs
    // it rather than carrying the old window's hit count onto the new one.
    if (search.value.open) runSearch('incremental')
  }

  // A GL context is claimed when a window is first shown, not when its pane
  // mounts. Mounting covers every pane of every pooled session, which spends
  // a context per background tab and pushes WebKit past its per-page limit on
  // each attach — and the pane it then kills is somebody else's. ADR terminal-renderer-claimed-on-activation.
  function showRenderers(windowId: string): void {
    const tab = findTab(windowId)
    if (!tab) return
    for (const pane of tab.panes) showRenderer(pane.paneId)
  }

  function showRenderer(paneId: string): void {
    const state = panes.get(paneId)
    if (!state?.host || state.rendered) return
    loadRenderer(state)
  }

  function openSearch(): void {
    search.value = { ...search.value, open: true }
    runSearch('incremental')
  }

  function closeSearch(): void {
    clearHighlights()
    search.value = { open: false, query: '', matches: 0, index: 0 }
    focusActive()
  }

  function setSearchQuery(query: string): void {
    search.value = { ...search.value, query }
    runSearch('incremental')
  }

  function findNext(): void {
    runSearch('next')
  }
  function findPrevious(): void {
    runSearch('previous')
  }

  // 'incremental' keeps the viewport on the match it is already showing while
  // the query is still being typed; the other two are the user stepping.
  function runSearch(mode: 'incremental' | 'next' | 'previous'): void {
    const finder = panes.get(activePaneId())?.finder
    if (!finder) return
    if (!search.value.query) {
      finder.clearDecorations()
      search.value = { ...search.value, matches: 0, index: 0 }
      return
    }
    const highlight = searchHighlightColors()
    const options: ISearchOptions = {
      incremental: mode === 'incremental',
      decorations: {
        matchBackground: highlight.match,
        matchOverviewRuler: highlight.match,
        activeMatchBackground: highlight.active,
        activeMatchColorOverviewRuler: highlight.active,
      },
    }
    if (mode === 'previous') finder.findPrevious(search.value.query, options)
    else finder.findNext(search.value.query, options)
  }

  function clearHighlights(): void {
    for (const state of panes.values()) state.finder.clearDecorations()
  }

  // A search belongs to the buffer it ran against, so a new active pane re-runs
  // it rather than carrying the old pane's highlights and hit count over.
  function retargetSearch(): void {
    if (!search.value.open) return
    clearHighlights()
    runSearch('incremental')
  }

  // The window's box is what this client votes tmux's window size from, so the
  // observer sits on it rather than on any one pane's.
  function attachTab(windowId: string, host: HTMLElement): void {
    const state = windows.get(windowId)
    if (!state || state.host) return
    state.host = host
    const observer = new ResizeObserver(() => scheduleVote())
    observer.observe(host)
    state.observer = observer
    scheduleVote()
  }

  function attachPane(paneId: string, host: HTMLElement): void {
    const found = findPane(paneId)
    const state = panes.get(paneId)
    if (!found || !state || state.host) return
    state.host = host
    state.term.open(host)
    watchViewportScroll(state, paneId, host)
    const releasePaste = installTerminalImages(host, {
      pasteText: (text) => sendPaste(paneId, text),
      capture: () => {
        const capturedSocket = socket
        return {
          current: () =>
            socket === capturedSocket && capturedSocket?.readyState === WebSocket.OPEN && panes.get(paneId) === state,
          paste: (text, signal) => pasteTerminalImage(slug, paneId, text, signal),
          focus: () => {
            void selectPane(paneId)
            state.term.focus()
          },
        }
      },
    })
    state.disposers.push({ dispose: releasePaste })
    if (found.tab.windowId === activeWindowId.value) {
      // After open(), never before: an unopened Terminal defers addon
      // activation to its own open(), which would throw a missing-context
      // error out of there rather than out of the load, past the fallback.
      showRenderer(paneId)
      // A pane that opens as its window's active pane takes the keyboard when
      // the keyboard is already in that window — the pane a split just made,
      // which tmux has made active. One that opens under a keyboard elsewhere
      // leaves it there.
      if (
        found.tab.activePane === paneId &&
        paneMayAutoFocus.value &&
        (found.tab.panes.length === 1 || focusIsInside(found.tab.windowId))
      ) {
        state.term.focus()
      }
    }
    measureCell()
    scheduleVote()
  }

  function focusIsInside(windowId: string): boolean {
    const host = windows.get(windowId)?.host
    return !!host && host.contains(document.activeElement)
  }

  // The only signal that the user scrolled. xterm's own onScroll is suppressed
  // on this path — the viewport reads its DOM scrollTop, syncs the buffer, and
  // swallows the event so it cannot feed itself — so a wheel, a trackpad or a
  // dragged scrollbar moves the viewport off the tail silently, and nothing
  // would ever offer the way back. The element only exists after open(), and
  // xterm's own listener is registered inside it, so ours runs second and reads
  // a buffer already synced.
  function watchViewportScroll(state: PaneRuntime, paneId: string, host: HTMLElement): void {
    const viewport = host.querySelector('.xterm-viewport')
    if (!viewport) return
    const onScroll = (): void => refreshScrolledUp(paneId)
    viewport.addEventListener('scroll', onScroll, { passive: true })
    state.disposers.push({ dispose: () => viewport.removeEventListener('scroll', onScroll) })
  }

  function scheduleVote(): void {
    clearTimeout(resizeTimer)
    resizeTimer = setTimeout(voteSize, RESIZE_DEBOUNCE_MS)
  }

  // All panes share font metrics. Prefer an active-window atlas renderer because
  // the DOM renderer's fractional width can drift over many columns.
  function measureCell(): CellSize | null {
    for (const state of panesToMeasure()) {
      const measured = terminalCellSize(state.term)
      if (!measured) {
        warnUnmeasuredCell(state)
        continue
      }
      if (cell.value?.width !== measured.width || cell.value.height !== measured.height) cell.value = measured
      return measured
    }
    return null
  }

  function panesToMeasure(): PaneRuntime[] {
    const rank = (state: PaneRuntime): number => {
      if (state.windowId !== activeWindowId.value) return 2
      return state.rendered ? 0 : 1
    }
    return [...panes.values()].filter((state) => state.host).sort((a, b) => rank(a) - rank(b))
  }

  // A pane inside a display:none subtree measures nothing, and that is
  // expected; one with a box that has opened and still reports no cell is not.
  function warnUnmeasuredCell(state: PaneRuntime): void {
    if (warnedUnmeasuredCell || !state.term.element || !state.host?.clientWidth) return
    warnedUnmeasuredCell = true
    console.warn(
      'terminalGrid.ts: an opened terminal reports no cell; xterm may have moved _core._renderService.dimensions',
    )
  }

  // Vote the whole window box against shared cell metrics; tmux remains
  // authoritative for the actual grid. Hidden scrollbars reserve no column.
  function voteSize(): void {
    const tab = findTab(activeWindowId.value)
    const host = tab ? windows.get(tab.windowId)?.host : undefined
    if (!tab || !host) return
    // A pane inside a display:none subtree — every pooled session behind the
    // shown one — has no rendered box. Voting from one would squeeze every
    // window of the session to a few cells and force the TUI inside to reflow,
    // then reflow back on reveal — the repaint the pool exists to avoid.
    if (!host.clientWidth || !host.clientHeight) return
    const measured = measureCell()
    if (!measured) return
    const proposed = proposeGrid({ width: host.clientWidth, height: host.clientHeight }, measured)
    if (!proposed) return
    scheduleConstraintCheck()
    if (vote && proposed.cols === vote.cols && proposed.rows === vote.rows) return
    vote = { cols: proposed.cols, rows: proposed.rows }
    rememberVote(vote, cellMetrics())
    void client.resize(slug, vote.cols, vote.rows).catch((e: unknown) => {
      actionError.value = message(e, 'Could not resize the terminal.')
    })
  }

  // The grid a pane opens at while tmux has reported no size for it — a
  // window created from the toolbar lands at the pane's size rather than
  // snapping to it when the reconcile behind it arrives.
  function unreportedSize(): TerminalSize {
    return vote ?? DEFAULT_SIZE
  }

  function scheduleConstraintCheck(): void {
    clearTimeout(constraintTimer)
    constraintTimer = setTimeout(checkSizeConstraint, CONSTRAINT_SETTLE_MS)
  }

  // A vote tmux did not grant means another client decided this window's size.
  // The pane then renders a grid that does not match its box, which reads as a
  // rendering bug unless the rule is named.
  function checkSizeConstraint(): void {
    const tab = findTab(activeWindowId.value)
    if (!tab || !vote || constraintDismissed || !tab.width || !tab.height) {
      sizeConstraint.value = null
      return
    }
    const granted = { cols: tab.width, rows: tab.height }
    sizeConstraint.value =
      granted.cols === vote.cols && granted.rows === vote.rows ? null : { voted: { ...vote }, granted }
  }

  // Dismissal lasts as long as this attach: the constraint is a property of the
  // other client, so re-raising it on the next resize would nag about something
  // already read and understood.
  function dismissSizeConstraint(): void {
    constraintDismissed = true
    clearTimeout(constraintTimer)
    sizeConstraint.value = null
  }

  function handleFrame(payload: unknown): void {
    if (!(payload instanceof ArrayBuffer) && !(payload instanceof Uint8Array)) return
    const frame = decodeFrame(payload)
    if (!frame) return

    switch (frame.type) {
      case 'output': {
        panes.get(frame.paneId)?.output.write(frame.data)
        break
      }
      case 'window':
        applyWindowEvent(frame.kind, frame.state)
        break
      case 'lifecycle':
        if (frame.kind === 'exited') void exited(frame.message || 'No exit details are available.')
        else if (frame.kind === 'error') end('error', frame.message || 'The terminal client failed.')
        else if (frame.kind === 'degraded') degraded()
        break
    }
  }

  // The backend dropped output rather than ending the stream, and the repaint
  // that follows this frame is already on the wire. Clearing the buffers is
  // what makes that repaint the truth: a snapshot carries the pane's own
  // scrollback, so leaving the screen it lands on in place would show the same
  // lines twice with the gap between them unmarked. The held frame goes too —
  // it belongs to the stream the snapshot replaces.
  function degraded(): void {
    if (disposed) return
    outputDropped.value = true
    for (const [paneId, state] of panes) {
      state.output.reset()
      state.term.reset()
      refreshScrolledUp(paneId)
    }
  }

  function dismissOutputDropped(): void {
    outputDropped.value = false
  }

  function applyWindowEvent(kind: WindowEventKind, state: WindowState): void {
    applyWindowChange(kind, state)
    if (pendingCommands.get(state.windowId)?.()) pendingCommands.delete(state.windowId)
  }

  // Every window event carries the whole window, so the layout is taken from
  // all of them rather than from 'layout-changed' alone — a reconcile reports
  // one change per window and its kind may be any of these.
  function applyWindowChange(kind: WindowEventKind, state: WindowState): void {
    const { windowId } = state
    switch (kind) {
      case 'added': {
        if (findTab(windowId)) return
        const requested = pendingActivate === windowId
        tabs.value = [...tabs.value, createTab(state)]
        if (state.active || tabs.value.length === 1 || requested) {
          pendingActivate = ''
          if (requested) activateCreated(windowId)
          else setActive(windowId)
          void nextTick(() => scheduleVote())
        }
        return
      }
      case 'closed': {
        const tab = findTab(windowId)
        const intended =
          expectedWindowClosures.delete(windowId) ||
          (!!tab && tab.panes.length > 0 && tab.panes.every((pane) => expectedPaneClosures.has(pane.paneId)))
        if (intended && tabs.value.length === 1) {
          outcomes.report(slug, { reason: 'stopped', detail: 'The terminal was closed.' })
        } else if (!intended) {
          actionError.value = `Tmux window ${tab?.name || windowId} closed. No exit details are available.`
        }
        disposeTab(windowId)
        return
      }
      case 'renamed':
        break
      // The kind reports "this window's active flag or pane changed", not
      // "this window is now the session's", and a reconcile emits one for the
      // window that just *lost* the flag as well — in tmux index order, so
      // selecting a lower-indexed window lands the deactivated one last.
      // Following that would put the selection back where it came from.
      case 'active-changed':
        if (state.active && findTab(windowId)) setActive(windowId)
        break
      case 'layout-changed':
        break
    }
    const tab = findTab(windowId)
    if (tab) applyWindowState(tab, state)
    scheduleConstraintCheck()
  }

  function applyWindowState(tab: TerminalWindowTab, state: WindowState): void {
    const previousActivePane = tab.activePane
    // Read before any pane is disposed: closing the pane the keyboard was in
    // takes the keyboard with it, and the window's new active pane is where
    // it should land.
    const hadFocus = focusIsInside(tab.windowId)
    if (state.width && state.height) {
      tab.width = state.width
      tab.height = state.height
    }
    tab.zoomed = state.zoomed
    if (state.name) tab.name = state.name
    if (state.layout) tab.layout = state.layout
    if (state.activePane) tab.activePane = state.activePane
    reconcilePanes(tab)
    applyPaneGrids(tab)
    if (tab.activePane !== previousActivePane && tab.windowId === activeWindowId.value) {
      retargetSearch()
      if (hadFocus) void nextTick(() => panes.get(tab.activePane)?.term.focus())
    }
  }

  // Keep zoom-hidden emulators alive because tmux continues streaming to them.
  function reconcilePanes(tab: TerminalWindowTab): void {
    const wanted = windowPanes(tab)
    const grids = paneGrids(tab)
    for (const pane of [...tab.panes]) {
      if (!wanted.includes(pane.paneId)) {
        if (!expectedPaneClosures.delete(pane.paneId)) {
          actionError.value = `Tmux pane ${pane.paneId} closed. No exit details are available.`
        }
        disposePane(tab, pane.paneId)
      }
    }
    for (const paneId of wanted) {
      if (tab.panes.some((pane) => pane.paneId === paneId)) continue
      tab.panes.push(createPane(tab.windowId, paneId, grids.get(paneId)))
      if (tab.windowId === activeWindowId.value) void nextTick(() => showRenderer(paneId))
    }
  }

  function openSocket(): void {
    socket = client.openStream(slug)
    socket.binaryType = 'arraybuffer'
    socket.onopen = () => {
      if (disposed) return
      status.value = 'live'
      endReason.value = null
      error.value = null
      outcomes.running(slug)
    }
    socket.onmessage = (event: MessageEvent) => handleFrame(event.data)
    socket.onclose = () => dropped()
    socket.onerror = () => dropped()
  }

  // A transport drop is a distinct signal from a lifecycle end: tmux may still
  // be attached, but this webview has lost the stream either way.
  function dropped(): void {
    if (disposed || status.value === 'ended') return
    end('disconnected', 'Terminal connection lost. The tmux session may still be running.')
    void reconcileEnd(connectionEpoch)
  }

  function end(reason: TerminalEndReason, detail: string): void {
    if (disposed) return
    const intended = outcomes.outcome(slug)
    if (intended?.reason === 'stopped' && (reason === 'exited' || reason === 'disconnected')) {
      reason = 'stopped'
      detail = intended.detail
    }
    status.value = 'ended'
    endReason.value = reason
    error.value = detail
    outcomes.report(slug, { reason, detail })
    closeSocket()
  }

  async function exited(detail: string): Promise<void> {
    end('exited', detail)
    await reconcileEnd(connectionEpoch)
  }

  async function reconcileEnd(epoch: number): Promise<void> {
    let listings: Record<string, WindowState[]>
    try {
      listings = await client.listWindows([slug])
    } catch {
      return
    }
    if (disposed || epoch !== connectionEpoch || status.value !== 'ended' || listings[slug]?.length) return
    const previous = outcomes.outcome(slug)
    if (previous?.reason === 'stopped') end('stopped', previous.detail)
    else {
      const report = ['exited', 'error'].includes(endReason.value ?? '') ? error.value : null
      const detail = 'Tmux session terminated. No exit details are available.'
      end('terminated', report ? `${detail}\n\nTerminal connection report:\n${report}` : detail)
    }
    disposeTabs()
  }

  function closeSocket(): void {
    if (!socket) return
    socket.onopen = null
    socket.onmessage = null
    socket.onclose = null
    socket.onerror = null
    try {
      socket.close()
    } catch {
      // a socket that never opened throws on close; nothing left to clean up
    }
    socket = null
  }

  function disposePane(tab: TerminalWindowTab, paneId: string): void {
    const state = panes.get(paneId)
    if (state) {
      for (const disposer of state.disposers) disposer.dispose()
      state.term.dispose()
      panes.delete(paneId)
    }
    tab.panes = tab.panes.filter((pane) => pane.paneId !== paneId)
  }

  function disposeTab(windowId: string): void {
    const tab = findTab(windowId)
    if (tab) {
      for (const pane of [...tab.panes]) disposePane(tab, pane.paneId)
      tabs.value = tabs.value.filter((other) => other.windowId !== windowId)
    }
    const state = windows.get(windowId)
    if (state) {
      state.observer?.disconnect()
      windows.delete(windowId)
    }
    pendingCommands.delete(windowId)
    if (activeWindowId.value === windowId) setActive(tabs.value[0]?.windowId ?? '')
  }

  function disposeTabs(): void {
    for (const tab of [...tabs.value]) disposeTab(tab.windowId)
  }

  async function start(): Promise<void> {
    const epoch = ++connectionEpoch
    status.value = 'connecting'
    endReason.value = null
    error.value = null
    actionError.value = null
    outputDropped.value = false
    try {
      // Concurrent, not sequential: the faces have to be resident before
      // term.open() measures a cell, which attachPane does well after this
      // resolves — so ~100ms of woff2 decode has no reason to be spent ahead of
      // the tmux round trip instead of alongside it.
      //
      // 0x0 sets no client size at all: tmux ignores a control client until it
      // sets one, so the session keeps the size its other clients gave it.
      const [, { windows: listed }] = await Promise.all([
        loadTerminalFont(),
        client.attach(slug, vote?.cols ?? 0, vote?.rows ?? 0),
      ])
      if (disposed || epoch !== connectionEpoch) return
      tabs.value = listed.map(createTab)
      setActive(listed.find((window) => window.active)?.windowId ?? listed[0]?.windowId ?? '')
      openSocket()
    } catch (e) {
      if (disposed || epoch !== connectionEpoch) return
      if (e instanceof TerminalRequestError && e.kind === 'not_found') {
        const previous = outcomes.outcome(slug)
        if (previous && ['terminated', 'start-failed', 'completed', 'stopped'].includes(previous.reason))
          end(previous.reason, previous.detail)
        else if (outcomes.wasRunning(slug)) end('terminated', 'Tmux session terminated. No exit details are available.')
        else end('not-started', e.message)
        return
      }
      end('attach-failed', message(e, 'Could not attach to this session.'))
    }
  }

  // Reconnect always builds new terminals. What the attach behind it guarantees
  // is a snapshot to open on, not a fresh control client: the backend captures
  // the panes again whether or not its client survived the drop, and a reused
  // terminal would paint that capture over a stale screen.
  async function reconnect(): Promise<void> {
    closeSocket()
    disposeTabs()
    await start()
  }

  async function select(windowId: string): Promise<void> {
    if (!findTab(windowId)) return
    // Reselecting the active window is still an intent to type into it: the
    // click just moved DOM focus onto the tab, so hand it back to the pane.
    // An arrow onto it is not — it is passing through.
    if (activeWindowId.value === windowId) {
      if (paneMayAutoFocus.value) focusActive()
      return
    }
    setActive(windowId)
    await nextTick()
    if (paneMayAutoFocus.value) focusActive()
    scheduleVote()
    await control(() => client.selectWindow(slug, windowId), 'Could not select that window.')
  }

  function focusActive(): void {
    panes.get(activePaneId())?.term.focus()
  }

  function scrollToBottom(): void {
    const state = panes.get(activePaneId())
    if (!state) return
    state.term.scrollToBottom()
    state.term.focus()
  }

  // Whichever of the two arrives second activates the window: the stream may
  // announce it before this call returns.
  // `command` is typed into the new window rather than handed to tmux's
  // new-window: tmux would parse the line as its own command words, and `;` in
  // it separates tmux commands. Sent as input, the shell reads it the way it
  // reads anything else the user types — history included — and stays alive
  // once it finishes. The pane's input queue holds the bytes if the shell has
  // not reached its prompt yet.
  async function newWindow(command?: string): Promise<void> {
    const created = await control(() => client.newWindow(slug), 'Could not create a window.')
    if (!created || disposed) return
    if (findTab(created.windowId)) activateCreated(created.windowId)
    else pendingActivate = created.windowId
    if (command) {
      // The window's pane is only knowable once tmux has announced its layout,
      // so the line waits for that rather than being addressed to a window.
      const typeInto = (): boolean => {
        const tab = findTab(created.windowId)
        const pane = tab && activePaneOf(tab)
        if (!pane) return false
        sendInput(pane.paneId, `${command}\r`)
        return true
      }
      if (!typeInto()) pendingCommands.set(created.windowId, typeInto)
    }
  }

  async function newAgentWindow(agent: string): Promise<void> {
    const created = await control(() => client.newAgentWindow(slug, agent), 'Could not start the agent.')
    if (!created || disposed) return
    if (findTab(created.windowId)) activateCreated(created.windowId)
    else pendingActivate = created.windowId
  }

  // A window this view asked for is one to type in, so it takes focus as well —
  // unlike one another client opened, which must not pull the keyboard out of
  // the pane in front of the user.
  function activateCreated(windowId: string): void {
    setActive(windowId)
    if (paneMayAutoFocus.value) void nextTick(() => focusActive())
  }

  async function closeWindow(windowId: string): Promise<void> {
    expectedWindowClosures.add(windowId)
    const closed = await control(async () => {
      await client.closeWindow(slug, windowId)
      return true
    }, 'Could not close that window.')
    if (!closed) expectedWindowClosures.delete(windowId)
  }

  async function rename(windowId: string, name: string): Promise<void> {
    await control(() => client.renameWindow(slug, windowId, name), 'Could not rename that window.')
  }

  // The strip reorders on the pointer and tmux confirms after, because the round
  // trip is long enough for a drop to read as ignored. tmux owns the order — a
  // window may have moved from another client since — so its answer replaces the
  // guess rather than being assumed to match it, and a refusal puts the strip
  // back where it was instead of leaving the tabs somewhere tmux never agreed to.
  async function moveWindow(windowId: string, position: number): Promise<void> {
    const before = tabs.value
    const optimistic = reorderTabs(before, windowId, position)
    if (!optimistic) return
    tabs.value = optimistic
    actionError.value = null
    try {
      const { windows: ordered } = await client.moveWindow(slug, windowId, position)
      if (disposed) return
      tabs.value = applyOrder(
        tabs.value,
        ordered.map((window) => window.windowId),
      )
    } catch (e) {
      if (disposed) return
      // The order goes back, not the tab set: a window opened or closed while
      // the move was in flight is tmux's news, and the refusal is not about it.
      tabs.value = applyOrder(
        tabs.value,
        before.map((tab) => tab.windowId),
      )
      actionError.value = message(e, 'Could not move that window.')
    }
  }

  // A click into a pane is a select. The tab's active pane moves at once so
  // the keystrokes that follow are addressed to it — the input frame names a
  // pane — while tmux's own announcement, which agrees, lands a round trip
  // later.
  async function selectPane(paneId: string): Promise<void> {
    const found = findPane(paneId)
    if (!found) return
    panes.get(paneId)?.term.focus()
    if (found.tab.activePane === paneId) return
    found.tab.activePane = paneId
    if (found.tab.windowId === activeWindowId.value) retargetSearch()
    await control(() => client.selectPane(slug, paneId), 'Could not select that pane.')
  }

  async function splitPane(direction: SplitDirection): Promise<void> {
    const paneId = activePaneId()
    if (!paneId) return
    await control(() => client.splitPane(slug, paneId, direction), 'Could not split that pane.')
  }

  async function closePane(paneId: string = activePaneId()): Promise<void> {
    if (!paneId) return
    expectedPaneClosures.add(paneId)
    const closed = await control(async () => {
      await client.closePane(slug, paneId)
      return true
    }, 'Could not close that pane.')
    if (!closed) expectedPaneClosures.delete(paneId)
  }

  async function zoomPane(): Promise<void> {
    const paneId = activePaneId()
    if (!paneId) return
    await control(() => client.zoomPane(slug, paneId), 'Could not zoom that pane.')
  }

  async function focusPane(direction: PaneDirection): Promise<void> {
    const paneId = activePaneId()
    if (!paneId) return
    await control(() => client.selectPane(slug, paneId, direction), 'Could not select that pane.')
  }

  // A divider drag produces a size per pointer move and tmux answers each with a
  // layout; only the latest is worth sending, so one request is in flight at a
  // time and a newer size replaces one still waiting.
  let resizeInFlight = false
  let queuedResize: { paneId: string; size: { width?: number; height?: number } } | null = null

  async function resizePane(paneId: string, size: { width?: number; height?: number }): Promise<void> {
    queuedResize = { paneId, size }
    if (resizeInFlight) return
    resizeInFlight = true
    try {
      while (queuedResize && !disposed) {
        const next = queuedResize
        queuedResize = null
        await control(() => client.resizePane(slug, next.paneId, next.size), 'Could not resize that pane.')
      }
    } finally {
      resizeInFlight = false
    }
  }

  async function control<T>(run: () => Promise<T>, fallback: string): Promise<T | undefined> {
    actionError.value = null
    try {
      return await run()
    } catch (e) {
      actionError.value = message(e, fallback)
      return undefined
    }
  }

  function dispose(): void {
    if (disposed) return
    disposed = true
    clearTimeout(resizeTimer)
    clearTimeout(constraintTimer)
    closeSocket()
    disposeTabs()
    scope.stop()
    // Intentional teardown releases the control client; only an unexpected drop
    // leaves tmux attached (ADR terminal-transport / decision D5).
    void client.detach(slug).catch(() => {})
  }

  return {
    tabs,
    activeWindowId,
    status,
    painted,
    endReason,
    error,
    actionError,
    sizeConstraint,
    dismissSizeConstraint,
    outputDropped,
    dismissOutputDropped,
    cell,
    search,
    openSearch,
    closeSearch,
    setSearchQuery,
    findNext,
    findPrevious,
    start,
    reconnect,
    reportOutcome: (reason: TerminalEndReason, detail: string) => {
      connectionEpoch++
      end(reason, detail)
    },
    select,
    newWindow,
    newAgentWindow,
    closeWindow,
    rename,
    moveWindow,
    attachTab,
    attachPane,
    disposeTab,
    selectPane,
    splitPane,
    closePane,
    zoomPane,
    focusPane,
    resizePane,
    focusActive,
    scrollToBottom,
    dispose,
  }
}

/**
 * Moves one tab to `position`, an index into the resulting order. Returns null
 * when nothing would change, so a drop that lands where it started costs no
 * round trip.
 */
function reorderTabs<T extends { windowId: string }>(tabs: T[], windowId: string, position: number): T[] | null {
  const from = tabs.findIndex((tab) => tab.windowId === windowId)
  if (from < 0 || position < 0 || position >= tabs.length || position === from) return null
  const next = [...tabs]
  next.splice(position, 0, ...next.splice(from, 1))
  return next
}

/**
 * Reorders tabs to match an authoritative window-id order. Ids the order does
 * not name keep their relative places at the end rather than vanishing: the
 * strip is what the user is looking at, and a window this client has not caught
 * up on yet is not evidence that its tab should go.
 */
function applyOrder<T extends { windowId: string }>(tabs: T[], order: string[]): T[] {
  const rank = new Map(order.map((windowId, index) => [windowId, index]))
  return [...tabs].sort(
    (a, b) => (rank.get(a.windowId) ?? Number.MAX_SAFE_INTEGER) - (rank.get(b.windowId) ?? Number.MAX_SAFE_INTEGER),
  )
}

function message(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

// A failed claim leaves state.rendered false, which is what makes showRenderer
// try again the next time the pane is shown (ADR terminal-renderer-claimed-on-activation).
function loadRenderer(state: PaneRuntime): void {
  claimAtlasRenderer(
    state.term,
    (addon) => state.disposers.push(addon),
    (rendered) => {
      state.rendered = rendered
    },
  )
}

// Re-exported so the pane's own tests keep reaching it through the composable
// they exercise.
export { resetTerminalFacesForTests }
