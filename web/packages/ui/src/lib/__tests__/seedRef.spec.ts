import { describe, expect, it } from 'vitest'
import { reactive } from 'vue'
import { seedRef } from '../seedRef'

describe('seedRef', () => {
  it('keeps the first value when the source changes', () => {
    const props = reactive({ name: 'first' })
    const name = seedRef(() => props.name)

    props.name = 'second'

    expect(name.value).toBe('first')
  })

  it('never writes an edit through to a reactive source object', () => {
    const props = reactive({ workspace: { mcps: ['github'], env: { A: '1' } } })
    const mcps = seedRef(() => props.workspace.mcps)
    const env = seedRef(() => props.workspace.env)

    mcps.value.push('grafana')
    env.value.A = '2'

    expect(props.workspace.mcps).toEqual(['github'])
    expect(props.workspace.env).toEqual({ A: '1' })
  })

  it('copies a value built from reactive pieces', () => {
    const props = reactive({ schedules: [{ id: 'a', tags: ['nightly'] }] })
    const cards = seedRef(() => props.schedules.map((s) => ({ id: s.id, tags: s.tags })))

    cards.value[0].tags.push('weekly')

    expect(props.schedules[0].tags).toEqual(['nightly'])
  })
})
