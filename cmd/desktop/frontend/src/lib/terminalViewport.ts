import type { IBuffer, IBufferLine, IMarker, Terminal } from '@xterm/xterm'

interface ViewportAnchor {
  atTail: boolean
  viewportY: number
  logicalCellOffset: number
  logicalCellLength: number
  viewportMarker?: IMarker
  startMarker?: IMarker
  nextMarker?: IMarker
}

export function resizeTerminalPreservingViewport(term: Terminal, cols: number, rows: number): void {
  const anchor = captureViewportAnchor(term)
  try {
    term.resize(cols, rows)
    restoreViewportAnchor(term, anchor)
  } finally {
    anchor.viewportMarker?.dispose()
    anchor.startMarker?.dispose()
    anchor.nextMarker?.dispose()
  }
}

function captureViewportAnchor(term: Terminal): ViewportAnchor {
  const buffer = term.buffer.active
  const viewportY = buffer.viewportY
  const atTail = viewportY === buffer.baseY
  if (atTail || buffer.type !== 'normal') {
    return { atTail, viewportY, logicalCellOffset: 0, logicalCellLength: 0 }
  }

  const logicalStart = findLogicalStart(buffer, viewportY)
  const logicalEnd = findLogicalEnd(buffer, viewportY)
  const cursorLine = buffer.baseY + buffer.cursorY
  if (cursorLine >= logicalStart && cursorLine <= logicalEnd) {
    // xterm leaves the cursor's wrapped logical line for the program to redraw,
    // so its physical rows do not participate in reflow.
    return {
      atTail,
      viewportY,
      logicalCellOffset: 0,
      logicalCellLength: 0,
      viewportMarker: term.registerMarker(viewportY - cursorLine),
    }
  }

  return {
    atTail,
    viewportY,
    logicalCellOffset: cellOffsetToLine(buffer, logicalStart, viewportY, term.cols),
    logicalCellLength: logicalLineCellCount(buffer, logicalStart, logicalEnd, term.cols),
    startMarker: term.registerMarker(logicalStart - cursorLine),
    nextMarker: logicalEnd + 1 < buffer.length
      ? term.registerMarker(logicalEnd + 1 - cursorLine)
      : undefined,
  }
}

function restoreViewportAnchor(term: Terminal, anchor: ViewportAnchor): void {
  if (anchor.atTail) {
    term.scrollToBottom()
    return
  }

  const buffer = term.buffer.active
  const viewportMarkerLine = anchor.viewportMarker?.line ?? -1
  if (buffer.type === 'normal' && viewportMarkerLine >= 0) {
    term.scrollToLine(viewportMarkerLine)
    return
  }

  const startMarkerLine = anchor.startMarker?.line ?? -1
  if (buffer.type === 'normal' && startMarkerLine >= 0) {
    term.scrollToLine(lineAtCellOffset(buffer, startMarkerLine, anchor.logicalCellOffset, term.cols))
    return
  }

  const nextMarkerLine = anchor.nextMarker?.line ?? -1
  if (buffer.type === 'normal' && nextMarkerLine >= 0) {
    if (nextMarkerLine === 0) {
      term.scrollToLine(0)
      return
    }
    const survivingStart = findLogicalStart(buffer, nextMarkerLine - 1)
    const survivingLength = logicalLineCellCount(buffer, survivingStart, nextMarkerLine - 1, term.cols)
    const trimmedCells = Math.max(0, anchor.logicalCellLength - survivingLength)
    const survivingOffset = Math.max(0, anchor.logicalCellOffset - trimmedCells)
    term.scrollToLine(lineAtCellOffset(buffer, survivingStart, survivingOffset, term.cols))
    return
  }

  term.scrollToLine(0)
}

function findLogicalStart(buffer: IBuffer, line: number): number {
  while (line > 0 && buffer.getLine(line)?.isWrapped) line--
  return line
}

function findLogicalEnd(buffer: IBuffer, line: number): number {
  while (line + 1 < buffer.length && buffer.getLine(line + 1)?.isWrapped) line++
  return line
}

function lineAtCellOffset(buffer: IBuffer, start: number, offset: number, cols: number): number {
  let line = start
  while (offset > 0 && buffer.getLine(line + 1)?.isWrapped) {
    const rowCells = wrappedRowCellCount(buffer, line, cols)
    if (offset < rowCells) break
    offset -= rowCells
    line++
  }
  return line
}

function cellOffsetToLine(buffer: IBuffer, start: number, end: number, cols: number): number {
  let cells = 0
  for (let line = start; line < end; line++) cells += wrappedRowCellCount(buffer, line, cols)
  return cells
}

function logicalLineCellCount(buffer: IBuffer, start: number, end: number, cols: number): number {
  return cellOffsetToLine(buffer, start, end, cols) + trimmedRowCellCount(buffer.getLine(end), cols)
}

function wrappedRowCellCount(buffer: IBuffer, line: number, cols: number): number {
  const current = buffer.getLine(line)
  const next = buffer.getLine(line + 1)
  if (wideCharacterWrapsToNextRow(current, next, cols)) return cols - 1
  return cols
}

function trimmedRowCellCount(line: IBufferLine | undefined, cols: number): number {
  for (let column = cols - 1; column >= 0; column--) {
    const cell = line?.getCell(column)
    if (cell?.getChars()) return column + Math.max(1, cell.getWidth())
  }
  return 0
}

function wideCharacterWrapsToNextRow(
  current: IBufferLine | undefined,
  next: IBufferLine | undefined,
  cols: number,
): boolean {
  // xterm leaves the final cell empty when a double-width glyph cannot fit,
  // so that cell is not part of the logical line's offset across reflow.
  const finalCell = current?.getCell(cols - 1)
  const nextCell = next?.isWrapped ? next.getCell(0) : undefined
  return finalCell?.getChars() === '' && finalCell.getWidth() === 1 && nextCell?.getWidth() === 2
}
