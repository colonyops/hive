import type { Terminal } from '@xterm/xterm'

/** One image drawn on the layer, in CSS pixels relative to the screen's top left. */
export interface KittyDraw {
  bitmap: ImageBitmap
  source: { x: number; y: number; width: number; height: number }
  x: number
  y: number
  width: number
  height: number
}

/**
 * One canvas over xterm's screen, above the text and under search
 * decorations (z-index 6). It is created on the first paint that has
 * something to draw, so a terminal that never shows an image carries none.
 */
export class KittyLayer {
  private canvas: HTMLCanvasElement | null = null
  private context: CanvasRenderingContext2D | null = null
  private drawn = false

  constructor(private readonly term: Terminal) {}

  get showing(): boolean {
    return this.drawn
  }

  paint(draws: KittyDraw[], width: number, height: number): void {
    if (!draws.length && !this.drawn) return
    const context = this.attach()
    if (!context || !this.canvas) return

    // Device pixels throughout, with every edge rounded, so an image lands on
    // whole pixels at any ratio rather than being resampled across a seam.
    const ratio = window.devicePixelRatio || 1
    const deviceWidth = Math.round(width * ratio)
    const deviceHeight = Math.round(height * ratio)
    if (this.canvas.width !== deviceWidth) this.canvas.width = deviceWidth
    if (this.canvas.height !== deviceHeight) this.canvas.height = deviceHeight
    this.canvas.style.width = `${width}px`
    this.canvas.style.height = `${height}px`

    context.setTransform(1, 0, 0, 1, 0, 0)
    context.clearRect(0, 0, deviceWidth, deviceHeight)
    context.imageSmoothingEnabled = true
    context.imageSmoothingQuality = 'high'
    for (const draw of draws) {
      const left = Math.round(draw.x * ratio)
      const top = Math.round(draw.y * ratio)
      const right = Math.round((draw.x + draw.width) * ratio)
      const bottom = Math.round((draw.y + draw.height) * ratio)
      if (right <= left || bottom <= top) continue
      const { source } = draw
      context.drawImage(
        draw.bitmap,
        source.x,
        source.y,
        source.width,
        source.height,
        left,
        top,
        right - left,
        bottom - top,
      )
    }
    this.drawn = draws.length > 0
  }

  dispose(): void {
    this.canvas?.remove()
    this.canvas = null
    this.context = null
    this.drawn = false
  }

  private attach(): CanvasRenderingContext2D | null {
    const screen = this.term.element?.querySelector('.xterm-screen')
    if (!screen) return null
    if (this.canvas?.parentElement === screen) return this.context
    if (!this.canvas) {
      const canvas = document.createElement('canvas')
      canvas.className = 'xterm-kitty-graphics'
      canvas.setAttribute('aria-hidden', 'true')
      Object.assign(canvas.style, {
        position: 'absolute',
        left: '0',
        top: '0',
        zIndex: '1',
        pointerEvents: 'none',
      })
      this.canvas = canvas
      this.context = canvas.getContext('2d')
    }
    screen.appendChild(this.canvas)
    return this.context
  }
}
