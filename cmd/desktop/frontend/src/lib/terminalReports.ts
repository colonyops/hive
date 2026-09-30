import type { IDisposable, IFunctionIdentifier, Terminal } from '@xterm/xterm'

// Every CSI xterm answers by synthesizing pane input.
const CSI_QUERIES: IFunctionIdentifier[] = [
  { final: 'c' }, // primary device attributes
  { prefix: '>', final: 'c' }, // secondary
  { prefix: '=', final: 'c' }, // tertiary
  { final: 'n' }, // device status: operating status, cursor position
  { prefix: '?', final: 'n' },
  { intermediates: '$', final: 'p' }, // DECRQM
  { prefix: '?', intermediates: '$', final: 'p' },
]
// Absent: the window ops that report a size (CSI 14/16/18 t). Every one of them
// is gated behind a `windowOptions` entry, and no terminal here opts in — so
// enabling one is what would need to add it back.

// OSC colour operations. Each takes a set or a query on the same identifier.
const OSC_COLORS = [4, 5, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19]

/**
 * Stops xterm answering terminal queries on a pane tmux owns.
 *
 * tmux answers them itself and forwards the bytes on only so this renderer can
 * draw them, so an answer from xterm is a second one — and it arrives a
 * websocket round trip late, landing in whatever reads the pane next
 * (ADR the-embedded-emulator-never-answers-terminal-queries-on-a-tmux-pane).
 *
 * Not for a ptyterm pop-up: there xterm is the terminal and must keep answering.
 */
export function silenceDeviceReports(term: Terminal): IDisposable {
  const handlers: IDisposable[] = [
    ...CSI_QUERIES.map((id) => term.parser.registerCsiHandler(id, () => true)),
    term.parser.registerDcsHandler({ intermediates: '$', final: 'q' }, () => true), // DECRQSS
    ...OSC_COLORS.map((ident) => term.parser.registerOscHandler(ident, isColorQuery)),
  ]
  return { dispose: () => { for (const handler of handlers) handler.dispose() } }
}

// A '?' where a colour would go. This swallows the whole payload, so a request
// that both sets and queries loses its set half — the parser cannot express
// answering part of a sequence, and no program pairs them in practice.
function isColorQuery(data: string): boolean {
  return data.split(';').includes('?')
}
