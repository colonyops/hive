import type { IBuffer, IDisposable, IMarker, Terminal } from '@xterm/xterm'
import { createLogger } from '../log'
import { terminalCellSize, type CellSize } from '../terminalGrid'
import { parseKittyCommand, type KittyCommand } from './kittyCommand'
import { decodeKittyImage } from './kittyDecode'
import { KITTY_OSC, KittyFilter } from './kittyFilter'
import { KittyLayer, type KittyDraw } from './kittyLayer'

const log = createLogger('kitty-graphics')

export interface KittyLimits {
  /** Stored images per terminal. */
  images: number
  /** Decoded pixel bytes per terminal; also the cap on one upload's base64. */
  bytes: number
  placements: number
}

const DEFAULT_LIMITS: KittyLimits = { images: 64, bytes: 64 << 20, placements: 256 }

// A chunk that carries only these keys continues an upload.
const CONTINUATION_KEYS = new Set(['m', 'q', 'i'])
const MAX_UPLOADS = 4
// Client ids are 32-bit, so ids assigned here cannot collide with one.
const INTERNAL_ID_BASE = 2 ** 32

export interface KittyGraphics extends IDisposable {
  /** Runs over the pane's bytes before term.write. */
  readonly filter: KittyFilter
  /** Drops every image and placement, for a resync or a term.reset(). */
  clear(): void
}

interface StoredImage {
  bitmap: ImageBitmap | null
  // The base64 size until the decode lands, then the decoded pixels.
  bytes: number
}

interface Placement {
  image: number
  id: number
  marker: IMarker
  col: number
  buffer: IBuffer['type']
  columns: number
  rows: number
  crop: { x: number; y: number; width: number; height: number }
  offsetX: number
  offsetY: number
  z: number
}

interface Upload {
  key: number
  command: KittyCommand
  parts: string[]
  size: number
}

interface Box {
  source: KittyDraw['source']
  x: number
  y: number
  width: number
  height: number
}

/**
 * Renders Kitty graphics commands from a pane tmux owns, drawing them on one
 * canvas over the screen. It never writes to the pane, never moves the cursor,
 * and never touches a cell: tmux does not parse the APC, so its cursor and grid
 * stay where they were, and the renderer has to agree with it.
 */
export function installKittyGraphics(term: Terminal, limits: Partial<KittyLimits> = {}): KittyGraphics {
  return new KittyRenderer(term, { ...DEFAULT_LIMITS, ...limits })
}

class KittyRenderer implements KittyGraphics {
  readonly filter: KittyFilter
  private readonly prefix: string
  private readonly layer: KittyLayer
  private readonly images = new Map<number, StoredImage>()
  private readonly numbers = new Map<number, number>()
  private readonly placements = new Set<Placement>()
  private readonly uploads = new Map<number, Upload>()
  private lastUpload: Upload | null = null
  private nextInternalId = INTERNAL_ID_BASE
  private frame = 0
  private disposed = false
  private readonly handlers: IDisposable[]

  constructor(
    private readonly term: Terminal,
    private readonly limits: KittyLimits,
  ) {
    // A program may write OSC 71137 itself, and it reaches the same handler.
    // The nonce is what marks a command the filter rewrote: it is random per
    // terminal and never leaves the renderer, so a program cannot forge it.
    const nonce = Array.from(crypto.getRandomValues(new Uint8Array(8)), (byte) =>
      byte.toString(16).padStart(2, '0'),
    ).join('')
    this.prefix = `${nonce};`
    this.filter = new KittyFilter(nonce)
    this.layer = new KittyLayer(term)
    this.handlers = [
      term.parser.registerOscHandler(KITTY_OSC, (data) => this.receive(data)),
      // false lets xterm run its own erase or reset after this one. xterm's
      // erase already disposes markers on the lines it blanks; this also takes
      // an image anchored in the scrollback that reaches onto the screen.
      term.parser.registerCsiHandler({ final: 'J' }, (params) => {
        const mode = params.length ? params[0] : 0
        if (mode === 2 || mode === 3) this.removePlacements((placement) => this.onScreen(placement))
        return false
      }),
      term.parser.registerEscHandler({ final: 'c' }, () => {
        this.clear()
        return false
      }),
      // Leaving the alternate screen needs no cleanup here: xterm disposes its
      // markers, and with them the placements anchored there.
      term.buffer.onBufferChange(() => this.scheduleDraw()),
      // In step with the text: a scheduled frame would trail a scroll by one.
      term.onRender(() => this.draw()),
      term.onScroll(() => this.scheduleDraw()),
      term.onResize(() => this.scheduleDraw()),
    ]
  }

  clear(): void {
    for (const placement of this.placements) placement.marker.dispose()
    this.placements.clear()
    for (const image of this.images.values()) image.bitmap?.close()
    this.images.clear()
    this.numbers.clear()
    this.uploads.clear()
    this.lastUpload = null
    this.scheduleDraw()
  }

  dispose(): void {
    if (this.disposed) return
    this.disposed = true
    for (const handler of this.handlers) handler.dispose()
    this.clear()
    cancelAnimationFrame(this.frame)
    this.layer.dispose()
  }

  private receive(data: string): boolean {
    if (!data.startsWith(this.prefix)) return true
    try {
      this.execute(parseKittyCommand(data.slice(this.prefix.length)))
    } catch (error) {
      log.warn('kitty graphics command failed', error)
    }
    return true
  }

  private execute(command: KittyCommand): void {
    if ([...command.keys].every((key) => CONTINUATION_KEYS.has(key))) {
      const upload = this.uploads.get(command.id) ?? this.lastUpload
      if (upload) {
        this.append(upload, command)
        return
      }
      if (command.keys.has('m')) return
    }
    switch (command.action) {
      case 't':
      case 'T':
        this.transmit(command)
        break
      case 'p':
        this.place(command, this.resolve(command))
        break
      case 'd':
        this.delete(command)
        break
    }
  }

  private transmit(command: KittyCommand): void {
    if (!command.more) {
      this.store(command, command.payload)
      return
    }
    this.dropUpload(this.uploads.get(command.id))
    const upload: Upload = { key: command.id, command, parts: [], size: 0 }
    this.uploads.set(upload.key, upload)
    if (this.uploads.size > MAX_UPLOADS) this.dropUpload(this.uploads.values().next().value)
    this.lastUpload = upload
    this.append(upload, { ...command, more: true })
  }

  private append(upload: Upload, chunk: KittyCommand): void {
    upload.parts.push(chunk.payload)
    upload.size += chunk.payload.length
    if (upload.size > this.limits.bytes) {
      log.debug('kitty upload dropped past the byte limit', undefined, { id: upload.command.id })
      this.dropUpload(upload)
      return
    }
    if (chunk.more) return
    this.dropUpload(upload)
    this.store(upload.command, upload.parts.join(''))
  }

  private dropUpload(upload: Upload | undefined): void {
    if (!upload) return
    if (this.uploads.get(upload.key) === upload) this.uploads.delete(upload.key)
    if (this.lastUpload === upload) this.lastUpload = null
  }

  private store(command: KittyCommand, base64: string): void {
    // Other media name files or shared memory on the host running the program.
    if (command.medium !== 'd') return
    const id = command.id || this.nextInternalId++
    // A retransmission under an id replaces the image and its placements.
    this.removeImage(id)
    const image: StoredImage = { bitmap: null, bytes: base64.length }
    this.images.set(id, image)
    if (command.number) this.numbers.set(command.number, id)
    this.enforceLimits(id)
    if (!this.images.has(id)) return
    // Placed now, not when the decode lands: the cursor is only here now.
    if (command.action === 'T') this.place(command, id)

    decodeKittyImage(command, base64, this.limits.bytes).then(
      (bitmap) => {
        if (this.disposed || this.images.get(id) !== image) {
          bitmap.close()
          return
        }
        image.bitmap = bitmap
        image.bytes = bitmap.width * bitmap.height * 4
        this.enforceLimits(id)
        this.scheduleDraw()
      },
      (error: unknown) => {
        log.debug('kitty image did not decode', error, { id: command.id, format: command.format })
        if (this.images.get(id) === image) this.removeImage(id)
      },
    )
  }

  private resolve(command: KittyCommand): number {
    if (command.id) return command.id
    return command.number ? (this.numbers.get(command.number) ?? 0) : 0
  }

  private place(command: KittyCommand, id: number): void {
    // Unicode placeholders are out of scope; they live in cells this renderer does not own.
    if (!id || !this.images.has(id) || command.unicode) return
    if (command.placement) {
      this.removePlacements((placement) => placement.image === id && placement.id === command.placement)
    }
    const buffer = this.term.buffer.active
    const placement: Placement = {
      image: id,
      id: command.placement,
      marker: this.term.registerMarker(0),
      col: buffer.cursorX,
      buffer: buffer.type,
      columns: command.columns,
      rows: command.rows,
      crop: { x: command.cropX, y: command.cropY, width: command.cropWidth, height: command.cropHeight },
      offsetX: command.offsetX,
      offsetY: command.offsetY,
      z: command.z,
    }
    placement.marker.onDispose(() => this.removePlacement(placement))
    this.placements.add(placement)
    if (this.placements.size > this.limits.placements) {
      const oldest = this.placements.values().next().value
      if (oldest) this.removePlacement(oldest)
    }
    this.scheduleDraw()
  }

  private delete(command: KittyCommand): void {
    const target = command.target
    const freeData = target !== target.toLowerCase()
    let affected: Set<number>
    switch (target.toLowerCase()) {
      case 'a':
        affected = this.removePlacements((placement) => this.onScreen(placement))
        break
      case 'i':
        if (!command.id) return
        this.removePlacements(
          (placement) => placement.image === command.id && (!command.placement || placement.id === command.placement),
        )
        affected = new Set([command.id])
        break
      default:
        return
    }
    if (!freeData) return
    const placed = this.placedImages()
    for (const id of affected) if (!placed.has(id)) this.removeImage(id)
  }

  /** Removes matching placements and returns the images they showed. */
  private removePlacements(matches: (placement: Placement) => boolean): Set<number> {
    const images = new Set<number>()
    for (const placement of [...this.placements]) {
      if (!matches(placement)) continue
      images.add(placement.image)
      this.removePlacement(placement)
    }
    return images
  }

  private removePlacement(placement: Placement): void {
    if (!this.placements.delete(placement)) return
    placement.marker.dispose()
    this.scheduleDraw()
  }

  private removeImage(id: number): void {
    const image = this.images.get(id)
    if (!image) return
    this.removePlacements((placement) => placement.image === id)
    image.bitmap?.close()
    this.images.delete(id)
    for (const [number, numbered] of this.numbers) if (numbered === id) this.numbers.delete(number)
  }

  private placedImages(): Set<number> {
    return new Set([...this.placements].map((placement) => placement.image))
  }

  // Unplaced images go first, then the oldest; Map order is insertion order,
  // and a replaced image is re-inserted, so the first entry is the oldest.
  private enforceLimits(keep: number): void {
    while (this.images.size > this.limits.images || this.storedBytes() > this.limits.bytes) {
      const placed = this.placedImages()
      const ids = [...this.images.keys()].filter((id) => id !== keep)
      const victim = ids.find((id) => !placed.has(id)) ?? ids[0] ?? keep
      log.debug('kitty image evicted', undefined, { id: victim })
      this.removeImage(victim)
    }
  }

  private storedBytes(): number {
    let total = 0
    for (const image of this.images.values()) total += image.bytes
    return total
  }

  private onScreen(placement: Placement): boolean {
    const buffer = this.term.buffer.active
    if (placement.buffer !== buffer.type) return false
    const first = placement.marker.line
    const last = first + this.footprintRows(placement) - 1
    return last >= buffer.baseY && first < buffer.baseY + this.term.rows
  }

  private footprintRows(placement: Placement): number {
    if (placement.rows) return placement.rows
    const cell = terminalCellSize(this.term)
    const bitmap = this.images.get(placement.image)?.bitmap
    if (!cell || !bitmap) return 1
    const box = this.box(placement, bitmap, cell)
    return box ? Math.max(1, Math.ceil((box.y + box.height) / cell.height)) : 1
  }

  // onScroll fires per line feed on a streaming pane, so a terminal with no
  // images must not pay a frame for it.
  private scheduleDraw(): void {
    if (this.frame || this.disposed) return
    if (!this.placements.size && !this.layer.showing) return
    this.frame = requestAnimationFrame(() => {
      this.frame = 0
      this.draw()
    })
  }

  private draw(): void {
    if (this.disposed) return
    if (this.frame) {
      cancelAnimationFrame(this.frame)
      this.frame = 0
    }
    const cell = terminalCellSize(this.term)
    if (!cell) {
      this.layer.paint([], 0, 0)
      return
    }
    this.layer.paint(this.layout(cell), cell.width * this.term.cols, cell.height * this.term.rows)
  }

  private layout(cell: CellSize): KittyDraw[] {
    if (!this.placements.size) return []
    const buffer = this.term.buffer.active
    const screenHeight = cell.height * this.term.rows
    const visible = [...this.placements].filter((placement) => placement.buffer === buffer.type)
    visible.sort((a, b) => a.z - b.z)
    const draws: KittyDraw[] = []
    for (const placement of visible) {
      const bitmap = this.images.get(placement.image)?.bitmap
      if (!bitmap) continue
      const box = this.box(placement, bitmap, cell)
      if (!box) continue
      const x = placement.col * cell.width + box.x
      const y = (placement.marker.line - buffer.viewportY) * cell.height + box.y
      if (y >= screenHeight || y + box.height <= 0) continue
      draws.push({ bitmap, source: box.source, x, y, width: box.width, height: box.height })
    }
    return draws
  }

  // With c and r the image fits inside that many cells from the top-left,
  // keeping its aspect ratio, where Kitty would stretch it. A program sizes the
  // box against the cell size it was told, and on a tmux pane that is tmux's
  // 16x32 guess, not this renderer's cell, so a stretch distorts the image.
  // With one of them the other follows the aspect ratio; with neither the
  // image keeps its pixel size, which Kitty counts in device pixels.
  private box(placement: Placement, bitmap: ImageBitmap, cell: CellSize): Box | null {
    const ratio = window.devicePixelRatio || 1
    const x = Math.min(Math.max(placement.crop.x, 0), bitmap.width)
    const y = Math.min(Math.max(placement.crop.y, 0), bitmap.height)
    const sourceWidth = placement.crop.width ? Math.min(placement.crop.width, bitmap.width - x) : bitmap.width - x
    const sourceHeight = placement.crop.height ? Math.min(placement.crop.height, bitmap.height - y) : bitmap.height - y
    if (sourceWidth <= 0 || sourceHeight <= 0) return null
    let width: number
    let height: number
    if (placement.columns && placement.rows) {
      const scale = Math.min(
        (placement.columns * cell.width) / sourceWidth,
        (placement.rows * cell.height) / sourceHeight,
      )
      width = sourceWidth * scale
      height = sourceHeight * scale
    } else if (placement.columns) {
      width = placement.columns * cell.width
      height = (width * sourceHeight) / sourceWidth
    } else if (placement.rows) {
      height = placement.rows * cell.height
      width = (height * sourceWidth) / sourceHeight
    } else {
      width = sourceWidth / ratio
      height = sourceHeight / ratio
    }
    return {
      source: { x, y, width: sourceWidth, height: sourceHeight },
      x: placement.offsetX / ratio,
      y: placement.offsetY / ratio,
      width,
      height,
    }
  }
}
