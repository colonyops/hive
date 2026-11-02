import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { useScaleTarget } from '../useScaleTarget'

function element(html: string): HTMLElement {
  const host = document.createElement('div')
  host.innerHTML = html
  document.body.append(host)
  return host.firstElementChild as HTMLElement
}

function setup(): () => string {
  let target: () => string = () => ''
  mount(
    defineComponent({
      setup() {
        target = useScaleTarget().scaleTarget
        return () => null
      },
    }),
  )
  return target
}

const hidden = new Set<Element>()

describe('useScaleTarget', () => {
  beforeEach(() => {
    hidden.clear()
    vi.spyOn(Element.prototype, 'getClientRects').mockImplementation(function (this: Element) {
      return (hidden.has(this) ? [] : [{}]) as unknown as DOMRectList
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  const click = (el: Element): void => {
    el.dispatchEvent(new Event('pointerdown', { bubbles: true }))
  }

  it('is the interface when no surface is on screen', () => {
    expect(setup()()).toBe('interface')
  })

  it('follows a click into a surface that never takes focus', () => {
    const diff = element('<div data-scale-surface="code"><span>row</span></div>')
    const target = setup()
    click(diff.firstElementChild!)
    expect(target()).toBe('code')
  })

  it('follows focus', () => {
    const pane = element('<div data-scale-surface="terminal"><textarea></textarea></div>')
    const target = setup()
    pane.firstElementChild!.dispatchEvent(new Event('focusin', { bubbles: true }))
    expect(target()).toBe('terminal')
  })

  it('moves to the interface on a click outside every surface, even beside a terminal', () => {
    element('<div data-scale-surface="terminal"></div>')
    const sidebar = element('<nav></nav>')
    const target = setup()
    click(sidebar)
    expect(target()).toBe('interface')
  })

  it('ignores an overlay, which hands focus back to the surface under it', () => {
    const diff = element('<div data-scale-surface="code"></div>')
    const palette = element('<div role="dialog"><input /></div>')
    const target = setup()
    click(diff)
    palette.firstElementChild!.dispatchEvent(new Event('focusin', { bubbles: true }))
    click(palette.firstElementChild!)
    expect(target()).toBe('code')
  })

  it('counts a surface inside a dialog, as the diffs in Changes are', () => {
    const changes = element('<div role="dialog"><div data-scale-surface="code"><span>row</span></div><nav></nav></div>')
    const target = setup()
    click(changes.querySelector('span')!)
    expect(target()).toBe('code')
    click(changes.querySelector('nav')!)
    expect(target()).toBe('code')
  })

  it('falls back to what is on screen when the last surface is hidden', () => {
    const popup = element('<div data-scale-surface="terminal"></div>')
    const doc = element('<div data-scale-surface="document"></div>')
    const target = setup()
    click(popup)
    hidden.add(popup)
    expect(target()).toBe('document')
    hidden.add(doc)
    expect(target()).toBe('interface')
  })

  it('prefers a terminal on screen before anything was clicked', () => {
    element('<div data-scale-surface="document"></div>')
    element('<div data-scale-surface="terminal"></div>')
    expect(setup()()).toBe('terminal')
  })
})
