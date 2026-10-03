import { afterEach, describe, expect, it } from 'vitest'
import { nextTick, ref } from 'vue'
import { useTreeExpansion } from '../useTreeExpansion'

interface Node {
  key: string
  live: boolean
}

const KEY = 'test.tree.expansion'

function setup(nodes: Node[]) {
  const filter = ref('')
  const tree = useTreeExpansion<Node>(KEY, {
    key: (node) => node.key,
    defaultOpen: (node) => node.live,
    filtering: () => !!filter.value,
    nodes: () => nodes,
  })
  return { filter, ...tree }
}

afterEach(() => localStorage.clear())

describe('useTreeExpansion', () => {
  const live = { key: 'a', live: true }
  const dormant = { key: 'b', live: false }

  it('opens a never-toggled group by its default, and a toggle wins over it', async () => {
    const { expanded, toggle } = setup([live, dormant])
    expect(expanded(live)).toBe(true)
    expect(expanded(dormant)).toBe(false)
    toggle(live)
    expect(expanded(live)).toBe(false)
    await nextTick()
    expect(JSON.parse(localStorage.getItem(KEY) ?? '{}')).toEqual({ a: false })
  })

  it('forces every group open while filtering', () => {
    const { expanded, toggle, filter } = setup([live])
    toggle(live)
    filter.value = 'x'
    expect(expanded(live)).toBe(true)
  })

  it('writes an entry on unfold only for a folded group', async () => {
    const { unfold } = setup([live, dormant])
    unfold(live)
    unfold(dormant)
    await nextTick()
    expect(JSON.parse(localStorage.getItem(KEY) ?? '{}')).toEqual({ b: true })
  })

  it('folds every group, not only the ones on screen', () => {
    const { expanded, setAll } = setup([live, dormant])
    setAll(false)
    expect(expanded(live)).toBe(false)
    setAll(true)
    expect(expanded(dormant)).toBe(true)
  })
})
