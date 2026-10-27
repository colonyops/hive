import { describe, expect, it } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useListKeyboardNav, type ListKeyboardNavOptions } from '../useListKeyboardNav'

function setup(count: number, options: Partial<ListKeyboardNavOptions> = {}) {
  const active = ref(0)
  const size = ref(count)
  const nav = effectScope().run(() => useListKeyboardNav({ active, count: () => size.value, ...options }))!
  return { active, size, nav }
}

function key(name: string): KeyboardEvent {
  return new KeyboardEvent('keydown', { key: name, cancelable: true })
}

describe('useListKeyboardNav', () => {
  it('wraps by default and stops at the ends when wrap is off', () => {
    const wrapping = setup(3)
    wrapping.nav.step(-1)
    expect(wrapping.active.value).toBe(2)
    wrapping.nav.step(1)
    expect(wrapping.active.value).toBe(0)

    const clamped = setup(3, { wrap: false })
    clamped.nav.step(-1)
    expect(clamped.active.value).toBe(0)
    clamped.nav.step(5)
    expect(clamped.active.value).toBe(2)
  })

  it('skips disabled rows on a step and on a jump', () => {
    const { active, nav } = setup(4, { disabled: (index) => index === 1 || index === 3 })
    nav.step(1)
    expect(active.value).toBe(2)
    nav.jump('end')
    expect(active.value).toBe(2)
    nav.jump('start')
    expect(active.value).toBe(0)
  })

  it('takes the arrows, and Home/End only when enabled', () => {
    const { active, nav } = setup(3)
    const down = key('ArrowDown')
    expect(nav.onKeydown(down)).toBe(true)
    expect(down.defaultPrevented).toBe(true)
    expect(active.value).toBe(1)
    expect(nav.onKeydown(key('End'))).toBe(false)
    expect(nav.onKeydown(key('Enter'))).toBe(false)

    const withEdges = setup(3, { homeEnd: () => true })
    expect(withEdges.nav.onKeydown(key('End'))).toBe(true)
    expect(withEdges.active.value).toBe(2)
  })

  it('pulls the index back in range when an open list shrinks', async () => {
    const open = ref(true)
    const { active, size } = setup(5, { open: () => open.value })
    active.value = 4
    size.value = 2
    await nextTick()
    expect(active.value).toBe(1)

    open.value = false
    active.value = 4
    size.value = 1
    await nextTick()
    expect(active.value).toBe(4)
  })

  it('scrolls the active row into view while open', async () => {
    const scrolled: number[] = []
    const rows = [0, 1, 2].map((index) => ({ scrollIntoView: () => scrolled.push(index) }) as unknown as Element)
    const { nav } = setup(3, { row: (index) => rows[index] })
    nav.step(1)
    await nextTick()
    await nextTick()
    expect(scrolled).toEqual([1])
  })
})
