import { describe, expect, it } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useSelectionRail } from '../useSelectionRail'

function row(top: number, height: number, selected: boolean): HTMLElement {
  const el = document.createElement('div')
  el.dataset.selected = String(selected)
  el.getBoundingClientRect = () => ({ top }) as DOMRect
  Object.defineProperty(el, 'offsetHeight', { value: height })
  return el
}

describe('useSelectionRail', () => {
  it('measures each rail off its selected row, relative to the content box', async () => {
    const content = document.createElement('div')
    content.getBoundingClientRect = () => ({ top: 100 }) as DOMRect
    content.append(row(110, 30, false), row(140, 28, true))
    const selection = ref(0)
    const { rails } = effectScope().run(() =>
      useSelectionRail(ref(content), ['[data-selected="true"]', '[data-missing]'], { sources: [selection] }),
    )!
    await nextTick()
    await nextTick()
    expect(rails.value[0]).toEqual({ y: 40, height: 28, shown: true })
    expect(rails.value[1].shown).toBe(false)
  })

  it('hides a rail where it stands when its row goes away', () => {
    const content = document.createElement('div')
    content.getBoundingClientRect = () => ({ top: 0 }) as DOMRect
    const selected = row(60, 30, true)
    content.append(selected)
    const { rails, update } = effectScope().run(() => useSelectionRail(ref(content), ['[data-selected="true"]']))!
    update()
    selected.remove()
    update()
    expect(rails.value[0]).toEqual({ y: 60, height: 30, shown: false })
  })
})
