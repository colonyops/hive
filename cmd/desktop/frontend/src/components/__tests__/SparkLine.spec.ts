import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SparkLine from '../SparkLine.vue'

function points(wrapper: ReturnType<typeof mount>): Array<[number, number]> {
  const raw = wrapper.get('polyline').attributes('points') ?? ''
  return raw.split(' ').map((pair) => {
    const [x, y] = pair.split(',').map(Number)
    return [x, y] as [number, number]
  })
}

describe('SparkLine', () => {
  it('has nothing to draw until there are two samples', () => {
    expect(mount(SparkLine, { props: { values: [] } }).find('polyline').exists()).toBe(false)
    expect(mount(SparkLine, { props: { values: [5] } }).find('polyline').exists()).toBe(false)
  })

  // A poller appends a sample every couple of seconds. Spacing points across
  // the width they happen to occupy re-draws the whole curve every tick; fixed
  // slots make it scroll instead.
  it('fills fixed slots from the right, so a new sample scrolls rather than re-spaces', () => {
    const two = points(mount(SparkLine, { props: { values: [10, 12], capacity: 5 } }))
    expect(two.at(-1)?.[0]).toBe(100)
    expect(two[0][0]).toBe(75)

    const three = points(mount(SparkLine, { props: { values: [10, 12, 11], capacity: 5 } }))
    expect(three.at(-1)?.[0]).toBe(100)
    expect(three[0][0]).toBe(50)
  })

  it('holds its y band while the samples stay inside it', async () => {
    const wrapper = mount(SparkLine, { props: { values: [10, 12], capacity: 5 } })
    const before = points(wrapper)

    await wrapper.setProps({ values: [10, 12, 11] })
    const after = points(wrapper)

    // Same values, same heights: only the x positions moved.
    expect(after[0][1]).toBe(before[0][1])
    expect(after[1][1]).toBe(before[1][1])
  })

  it('centres a flat series instead of pinning it to an edge, where it reads as a rule', () => {
    const flat = points(mount(SparkLine, { props: { values: [175, 175, 175] } }))
    for (const [, y] of flat) expect(y).toBeCloseTo(16, 1)
  })
})
