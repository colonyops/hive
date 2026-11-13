import { afterEach, describe, expect, it, vi } from 'vitest'
import { findOccluders } from '../nativeOcclusion'

function box(el: Element, x: number, y: number, w: number, h: number): void {
  vi.spyOn(el, 'getBoundingClientRect').mockReturnValue({
    left: x,
    top: y,
    width: w,
    height: h,
    right: x + w,
    bottom: y + h,
    x,
    y,
    toJSON: () => ({}),
  })
}

describe('findOccluders', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.restoreAllMocks()
  })

  it('finds floating UI by role and what is teleported beside the app, skipping hidden boxes', () => {
    document.body.innerHTML = `
      <div id="app">
        <div id="menu" role="menu"></div>
        <div id="closed" role="listbox"></div>
        <div id="plain"></div>
      </div>
      <div id="teleported"></div>`
    box(document.getElementById('app')!, 0, 0, 1000, 800)
    box(document.getElementById('menu')!, 10.4, 20.6, 100.2, 50)
    box(document.getElementById('closed')!, 0, 0, 0, 0)
    box(document.getElementById('plain')!, 0, 0, 500, 500)
    box(document.getElementById('teleported')!, 300, 300, 40, 20)

    expect(findOccluders()).toEqual([
      { x: 10, y: 20, w: 101, h: 50 },
      { x: 300, y: 300, w: 40, h: 20 },
    ])
  })
})
