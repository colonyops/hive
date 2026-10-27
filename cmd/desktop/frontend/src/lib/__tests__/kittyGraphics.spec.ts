import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Terminal } from '@xterm/xterm'
import { installKittyGraphics, type KittyGraphics, type KittyLimits } from '../kitty/kittyGraphics'

const encoder = new TextEncoder()

interface FakeBitmap {
  label: string
  width: number
  height: number
  close: ReturnType<typeof vi.fn>
}

interface Drawn {
  label: string
  source: number[]
  x: number
  y: number
  width: number
  height: number
}

// Every paint starts with a clearRect, so the frame on screen is whatever was
// drawn since the last one.
class FakeContext {
  frames: Drawn[][] = [[]]
  imageSmoothingEnabled = false
  imageSmoothingQuality = 'low'
  setTransform = vi.fn()
  clearRect = vi.fn(() => {
    this.frames.push([])
  })
  drawImage = vi.fn((bitmap: FakeBitmap, sx: number, sy: number, sw: number, sh: number, ...dest: number[]) => {
    const [x, y, width, height] = dest
    this.frames[this.frames.length - 1].push({ label: bitmap.label, source: [sx, sy, sw, sh], x, y, width, height })
  })

  get frame(): Drawn[] {
    return this.frames[this.frames.length - 1]
  }
}

// The payload's own bytes name the bitmap it decodes to, and its size comes
// from `sizes` so a test can give one image an aspect ratio.
const sizes = new Map<string, { width: number; height: number }>()
const decoded: string[] = []

async function fakeCreateImageBitmap(source: Blob | ImageData): Promise<FakeBitmap> {
  const label = source instanceof Blob ? await source.text() : `raw:${Array.from(source.data.slice(0, 8)).join(',')}`
  decoded.push(label)
  const size = sizes.get(label) ?? { width: 40, height: 20 }
  return { label, ...size, close: vi.fn() }
}

const CELL = { width: 10, height: 20 }

interface Harness {
  term: Terminal
  graphics: KittyGraphics
  context: FakeContext
  host: HTMLElement
  write: (data: string) => Promise<void>
  sent: () => string
  cursor: () => [number, number]
}

let cell = { ...CELL }
const harnesses: Harness[] = []

function harness(limits: Partial<KittyLimits> = {}, options = { cols: 20, rows: 5, scrollback: 50 }): Harness {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const term = new Terminal(options)
  term.open(host)
  // happy-dom measures no cell; the renderer reads the same private field
  // terminalGrid.ts does.
  const renderService = (term as unknown as { _core: { _renderService: object } })._core._renderService
  Object.defineProperty(renderService, 'dimensions', {
    get: () => ({
      css: { cell, canvas: { width: cell.width * term.cols, height: cell.height * term.rows } },
      device: { cell, canvas: { width: cell.width * term.cols, height: cell.height * term.rows } },
    }),
  })
  const context = new FakeContext()
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(context as unknown as RenderingContext)
  const graphics = installKittyGraphics(term, limits)
  let sent = ''
  term.onData((data) => {
    sent += data
  })
  const created: Harness = {
    term,
    graphics,
    context,
    host,
    write: (data) => new Promise((resolve) => term.write(graphics.filter.rewrite(encoder.encode(data)), resolve)),
    sent: () => sent,
    cursor: () => [term.buffer.active.cursorX, term.buffer.active.cursorY],
  }
  harnesses.push(created)
  return created
}

// Lets the decode land and the frame it schedules paint.
async function settle(): Promise<void> {
  for (let i = 0; i < 3; i++) await new Promise((resolve) => setTimeout(resolve, 0))
  await new Promise((resolve) => requestAnimationFrame(resolve))
}

function apc(control: string, payload = ''): string {
  return `\x1b_G${control}${payload ? `;${payload}` : ''}\x1b\\`
}

function png(label: string): string {
  return btoa(label)
}

function labels(context: FakeContext): string[] {
  return context.frame.map((drawn) => drawn.label)
}

describe('installKittyGraphics', () => {
  beforeEach(() => {
    cell = { ...CELL }
    sizes.clear()
    decoded.length = 0
    vi.stubGlobal('createImageBitmap', fakeCreateImageBitmap)
  })

  afterEach(() => {
    for (const { graphics, term, host } of harnesses.splice(0)) {
      graphics.dispose()
      term.dispose()
      host.remove()
    }
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it("draws Pi's image at the cursor cell, fitted inside its cells", async () => {
    const { write, context, cursor } = harness()
    await write('line one\r\nline two\r\n  ')
    await write(apc('a=T,f=100,q=2,C=1,c=4,r=2,i=7', png('cat')))
    await settle()

    expect(context.frame).toEqual([{ label: 'cat', source: [0, 0, 40, 20], x: 20, y: 40, width: 40, height: 20 }])
    expect(cursor()).toEqual([2, 2])
  })

  // Pi sizes the box against tmux's 16x32 cell; stretching it to this
  // renderer's cells is what squished a real screenshot.
  it('keeps the aspect ratio when the box is wider than the image', async () => {
    const { write, context } = harness()
    await write(apc('a=T,f=100,q=2,C=1,c=8,r=1,i=7', png('cat')))
    await settle()

    expect(context.frame).toEqual([{ label: 'cat', source: [0, 0, 40, 20], x: 0, y: 0, width: 40, height: 20 }])
  })

  it('adds a canvas inside the screen, above the text and out of the way of the pointer', async () => {
    const { write, host } = harness()
    await write(apc('a=T,f=100,c=1,r=1,i=1', png('a')))
    await settle()

    const canvas = host.querySelector<HTMLCanvasElement>('.xterm-screen > canvas.xterm-kitty-graphics')
    expect(canvas).not.toBeNull()
    expect(canvas!.style.pointerEvents).toBe('none')
    expect(canvas!.style.width).toBe('200px')
    expect(canvas!.style.height).toBe('100px')
  })

  it('leaves the cursor where it was with C=1 and without it', async () => {
    const { write, cursor } = harness()
    await write('abc')
    await write(apc('a=T,f=100,C=1,c=4,r=2,i=1', png('a')))
    expect(cursor()).toEqual([3, 0])
    await write(apc('a=T,f=100,c=4,r=2,i=2', png('b')))
    expect(cursor()).toEqual([3, 0])
    await write(apc('a=T,f=100,i=3', png('c')))
    expect(cursor()).toEqual([3, 0])
  })

  it('never sends anything to the pane', async () => {
    const { write, sent } = harness()
    await write(apc('a=q,i=31,s=1,v=1,f=24', 'AAAA'))
    await write(apc('a=T,f=100,i=7,c=2,r=2', png('a')))
    await write(apc('a=t,f=100,i=8', png('b')))
    await write(apc('a=p,i=8'))
    await write(apc('a=d,d=I,i=7'))
    await write(apc('a=T,f=100,i=9,m=1', 'not'))
    await write(apc('m=0', 'base64!!'))
    await settle()

    expect(sent()).toBe('')
  })

  it('assembles chunks under the id they name, else the last upload begun', async () => {
    const { write, context } = harness()
    const first = png('first image!')
    const second = png('second image')
    await write(apc('a=t,f=100,i=1,m=1', first.slice(0, 8)))
    await write(apc('a=t,f=100,i=2,m=1', second.slice(0, 8)))
    await write(apc('i=1,m=0', first.slice(8)))
    await write(apc('m=0', second.slice(8)))
    await write(apc('a=p,i=1,c=1,r=1'))
    await write(apc('a=p,i=2,c=1,r=1'))
    await settle()

    expect(decoded).toEqual(['first image!', 'second image'])
    expect(labels(context)).toEqual(['first image!', 'second image'])
  })

  it("assembles Pi's chunks, which carry only m after the first", async () => {
    const { write, context } = harness()
    const payload = png('a picture that spans three chunks')
    await write(apc('a=T,f=100,q=2,C=1,c=4,r=2,i=7,m=1', payload.slice(0, 16)))
    await write(apc('m=1', payload.slice(16, 32)))
    await write(apc('m=0', payload.slice(32)))
    await settle()

    expect(decoded).toEqual(['a picture that spans three chunks'])
    expect(labels(context)).toEqual(['a picture that spans three chunks'])
  })

  it('places a chunked image where the cursor is when the last chunk lands', async () => {
    const { write, context } = harness()
    const payload = png('chunked')
    await write(apc('a=T,f=100,c=1,r=1,i=1,m=1', payload.slice(0, 4)))
    await write('\r\n\r\n')
    await write(apc('m=0', payload.slice(4)))
    await settle()

    expect(context.frame[0]).toMatchObject({ x: 0, y: 40 })
  })

  it('moves an image with its line as output scrolls and the viewport moves', async () => {
    const { term, write, context } = harness()
    await write('\r\n\r\n\r\n\r\n')
    await write(apc('a=T,f=100,c=2,r=1,i=1', png('a')))
    await settle()
    expect(context.frame[0]).toMatchObject({ y: 80 })

    await write('\r\n\r\n')
    await settle()
    expect(context.frame[0]).toMatchObject({ y: 40 })

    term.scrollLines(-2)
    await settle()
    expect(context.frame[0]).toMatchObject({ y: 80 })

    await write('\r\n'.repeat(10))
    term.scrollToBottom()
    await settle()
    expect(context.frame).toEqual([])
  })

  it('drops a placement whose line is trimmed out of the scrollback', async () => {
    const { term, write, context } = harness({}, { cols: 20, rows: 5, scrollback: 2 })
    await write(apc('a=T,f=100,c=2,r=1,i=1', png('a')))
    await write('\r\n'.repeat(20))
    term.scrollToTop()
    await settle()

    expect(context.frame).toEqual([])
  })

  it('derives the missing dimension from the aspect ratio, and uses pixel size with neither', async () => {
    sizes.set('wide', { width: 80, height: 20 })
    const { write, context } = harness()
    await write(apc('a=T,f=100,c=4,i=1', png('wide')))
    await write('\r\n')
    await write(apc('a=T,f=100,r=2,i=2', png('wide')))
    await write('\r\n')
    await write(apc('a=T,f=100,i=3', png('wide')))
    await settle()

    expect(context.frame.map(({ width, height }) => [width, height])).toEqual([
      [40, 10],
      [160, 40],
      [80, 20],
    ])
  })

  it('draws the cropped rectangle of the image', async () => {
    const { write, context } = harness()
    await write(apc('a=T,f=100,c=1,r=1,x=5,y=2,w=10,h=8,i=1', png('a')))
    await settle()

    expect(context.frame[0].source).toEqual([5, 2, 10, 8])
  })

  it('draws on whole device pixels at a device pixel ratio of 2', async () => {
    vi.stubGlobal('devicePixelRatio', 2)
    cell = { width: 7.75, height: 17.5 }
    const { write, context, host } = harness()
    await write(' ')
    await write(apc('a=T,f=100,c=3,r=2,i=1', png('a')))
    await settle()

    const canvas = host.querySelector<HTMLCanvasElement>('canvas.xterm-kitty-graphics')!
    expect(canvas.width).toBe(310)
    expect(canvas.height).toBe(175)
    expect(canvas.style.width).toBe('155px')
    expect(context.frame[0]).toMatchObject({ x: 16, y: 0, width: 46, height: 23 })
  })

  it('redraws at the new cell size after a font change', async () => {
    const { term, write, context } = harness()
    await write('\r\n ')
    await write(apc('a=T,f=100,c=2,r=1,i=1', png('a')))
    await settle()
    expect(context.frame[0]).toMatchObject({ x: 10, y: 20, width: 20, height: 10 })

    cell = { width: 12, height: 24 }
    term.options.fontSize = 20
    await settle()
    expect(context.frame[0]).toMatchObject({ x: 12, y: 24, width: 24, height: 12 })
  })

  describe('delete', () => {
    async function twoImages(): Promise<Harness> {
      const created = harness()
      await created.write(apc('a=T,f=100,c=1,r=1,i=1', png('one')))
      await created.write('\r\n')
      await created.write(apc('a=T,f=100,c=1,r=1,i=2', png('two')))
      await settle()
      expect(labels(created.context)).toEqual(['one', 'two'])
      return created
    }

    it('d=a removes the placements on screen and keeps the image', async () => {
      const { write, context } = await twoImages()
      await write(apc('a=d,d=a,q=2'))
      await settle()
      expect(context.frame).toEqual([])

      await write(apc('a=p,i=1,c=1,r=1'))
      await settle()
      expect(labels(context)).toEqual(['one'])
    })

    it('d=a leaves a placement that has scrolled off the screen', async () => {
      const { term, write, context } = await twoImages()
      await write('\r\n'.repeat(4))
      await write(apc('a=d,d=a'))
      term.scrollToTop()
      await settle()
      expect(labels(context)).toEqual(['one'])
    })

    it('d=A removes the placements on screen and frees their images', async () => {
      const { write, context } = await twoImages()
      await write(apc('a=d,d=A,q=2'))
      await write(apc('a=p,i=1,c=1,r=1'))
      await settle()
      expect(context.frame).toEqual([])
    })

    it('d=i removes one image’s placements and keeps its data', async () => {
      const { write, context } = await twoImages()
      await write(apc('a=d,d=i,i=1'))
      await settle()
      expect(labels(context)).toEqual(['two'])

      await write(apc('a=p,i=1,c=1,r=1'))
      await settle()
      expect(labels(context)).toEqual(['two', 'one'])
    })

    it('d=I removes one image’s placements and frees it', async () => {
      const { write, context } = await twoImages()
      await write(apc('a=d,d=I,i=2,q=2'))
      await write(apc('a=p,i=2,c=1,r=1'))
      await settle()
      expect(labels(context)).toEqual(['one'])
    })

    it('d=i with p removes only that placement', async () => {
      const { write, context } = harness()
      await write(apc('a=t,f=100,i=1', png('one')))
      await write(apc('a=p,i=1,p=1,c=1,r=1'))
      await write(apc('a=p,i=1,p=2,c=1,r=1'))
      await write(apc('a=d,d=i,i=1,p=1'))
      await settle()
      expect(labels(context)).toEqual(['one'])
    })
  })

  it('moves a placement re-placed under the same placement id', async () => {
    const { write, context } = harness()
    await write(apc('a=t,f=100,i=1', png('one')))
    await write(apc('a=p,i=1,p=3,c=1,r=1'))
    await write(' ')
    await write(apc('a=p,i=1,p=3,c=1,r=1'))
    await settle()

    expect(context.frame.map((drawn) => drawn.x)).toEqual([10])
  })

  it('replaces an image retransmitted under the same id, with its placement', async () => {
    const { write, context } = harness()
    await write(apc('a=T,f=100,c=1,r=1,i=1', png('old')))
    await write('\r\n')
    await write(apc('a=T,f=100,c=1,r=1,i=1', png('new')))
    await settle()

    expect(context.frame).toEqual([expect.objectContaining({ label: 'new', y: 20 })])
  })

  it('drops the images on screen on ED2 and lets xterm erase', async () => {
    const { term, write, context } = harness()
    await write('text')
    await write(apc('a=T,f=100,c=1,r=1,i=1', png('a')))
    await write('\x1b[2J')
    await settle()

    expect(context.frame).toEqual([])
    expect(term.buffer.active.getLine(0)?.translateToString(true)).toBe('')
  })

  it('drops on ED2 an image anchored in the scrollback that reaches onto the screen', async () => {
    const { term, write, context } = harness()
    await write(apc('a=T,f=100,c=1,r=4,i=1', png('tall')))
    await write('\r\n'.repeat(6))
    await write('\x1b[2J')
    term.scrollToTop()
    await settle()

    expect(context.frame).toEqual([])
  })

  it('stacks placements by z, then by arrival', async () => {
    const { write, context } = harness()
    await write(apc('a=T,f=100,c=1,r=1,z=1,i=1', png('top')))
    await write(apc('a=T,f=100,c=1,r=1,z=-1,i=2', png('bottom')))
    await write(apc('a=T,f=100,c=1,r=1,i=3', png('middle')))
    await settle()

    expect(labels(context)).toEqual(['bottom', 'middle', 'top'])
  })

  it('drops every image on a full reset', async () => {
    const { write, context } = harness()
    await write(apc('a=T,f=100,c=1,r=1,i=1', png('a')))
    await write('\x1bc')
    await write(apc('a=p,i=1,c=1,r=1'))
    await settle()

    expect(context.frame).toEqual([])
  })

  it('clear() drops images and placements', async () => {
    const { graphics, write, context } = harness()
    await write(apc('a=T,f=100,c=1,r=1,i=1', png('a')))
    await settle()
    graphics.clear()
    await write(apc('a=p,i=1,c=1,r=1'))
    await settle()

    expect(context.frame).toEqual([])
  })

  it('draws only the active buffer and drops the alternate screen’s images on leaving it', async () => {
    const { write, context } = harness()
    await write(apc('a=T,f=100,c=1,r=1,i=1', png('main')))
    await write('\x1b[?1049h')
    await write(apc('a=T,f=100,c=1,r=1,i=2', png('alt')))
    await settle()
    expect(labels(context)).toEqual(['alt'])

    await write('\x1b[?1049l')
    await settle()
    expect(labels(context)).toEqual(['main'])

    await write('\x1b[?1049h')
    await settle()
    expect(context.frame).toEqual([])
  })

  it('ignores an OSC 71137 the program wrote itself', async () => {
    const { term, write, context } = harness()
    await new Promise<void>((resolve) => term.write(`\x1b]71137;a=T,f=100,c=1,r=1,i=1;${png('forged')}\x1b\\`, resolve))
    await write(`\x1b]71137;0000000000000000;a=T,f=100,c=1,r=1,i=2;${png('guessed')}\x1b\\`)
    await settle()

    expect(decoded).toEqual([])
    expect(context.frame).toEqual([])
  })

  it('decodes raw RGB by widening it to RGBA', async () => {
    const { write, context } = harness()
    const rgb = String.fromCharCode(1, 2, 3, 4, 5, 6)
    await write(apc('a=T,f=24,s=2,v=1,c=1,r=1,i=1', btoa(rgb)))
    await settle()

    expect(labels(context)).toEqual(['raw:1,2,3,255,4,5,6,255'])
  })

  it('inflates a zlib-compressed payload', async () => {
    const compressed = new Uint8Array(
      await new Response(new Blob(['zipped']).stream().pipeThrough(new CompressionStream('deflate'))).arrayBuffer(),
    )
    const { write, context } = harness()
    await write(apc('a=T,f=100,o=z,c=1,r=1,i=1', btoa(String.fromCharCode(...compressed))))
    await settle()
    await settle()

    expect(labels(context)).toEqual(['zipped'])
  })

  it('ignores media other than direct data', async () => {
    const { write } = harness()
    await write(apc('a=T,f=100,t=f,c=1,r=1,i=1', btoa('/etc/passwd')))
    await settle()

    expect(decoded).toEqual([])
  })

  describe('limits', () => {
    it('evicts unplaced images first, then the oldest', async () => {
      const { write, context } = harness({ images: 2 })
      await write(apc('a=T,f=100,c=1,r=1,i=1', png('one')))
      await write(apc('a=t,f=100,i=2', png('two')))
      await write(apc('a=t,f=100,i=3', png('three')))
      await write('\r\n')
      await write(apc('a=p,i=2,c=1,r=1'))
      await write(apc('a=p,i=3,c=1,r=1'))
      await settle()
      expect(labels(context)).toEqual(['one', 'three'])

      await write('\r\n')
      await write(apc('a=T,f=100,c=1,r=1,i=4', png('four')))
      await settle()
      expect(labels(context)).toEqual(['three', 'four'])
    })

    it('evicts by decoded bytes', async () => {
      // 40x20 RGBA is 3200 bytes, so two do not fit in 5000.
      const { write, context } = harness({ bytes: 5000 })
      await write(apc('a=T,f=100,c=1,r=1,i=1', png('one')))
      await settle()
      await write(apc('a=T,f=100,c=1,r=1,i=2', png('two')))
      await settle()

      expect(labels(context)).toEqual(['two'])
    })

    it('drops an upload that outgrows the byte limit', async () => {
      const { write, context } = harness({ bytes: 16 })
      const payload = png('far too large for the limit')
      await write(apc('a=T,f=100,c=1,r=1,i=1,m=1', payload.slice(0, 12)))
      await write(apc('m=1', payload.slice(12, 24)))
      await write(apc('m=0', payload.slice(24)))
      await settle()

      expect(decoded).toEqual([])
      expect(context.frame).toEqual([])
    })

    it('caps placements per terminal, dropping the oldest', async () => {
      const { write, context } = harness({ placements: 2 })
      await write(apc('a=t,f=100,i=1', png('one')))
      await write(apc('a=p,i=1,c=1,r=1'))
      await write(' ')
      await write(apc('a=p,i=1,c=1,r=1'))
      await write(' ')
      await write(apc('a=p,i=1,c=1,r=1'))
      await settle()

      expect(context.frame.map((drawn) => drawn.x)).toEqual([10, 20])
    })
  })

  it('removes its canvas and closes its bitmaps on dispose', async () => {
    const created = harness()
    await created.write(apc('a=T,f=100,c=1,r=1,i=1', png('a')))
    await settle()
    const bitmap = created.context.drawImage.mock.calls[0][0]

    created.graphics.dispose()

    expect(created.host.querySelector('canvas.xterm-kitty-graphics')).toBeNull()
    expect(bitmap.close).toHaveBeenCalled()
  })
})
