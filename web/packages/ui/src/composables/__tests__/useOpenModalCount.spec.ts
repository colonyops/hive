import { describe, expect, it } from 'vitest'
import { effectScope } from 'vue'
import { useOpenModalCount, useRegisterOpenModal } from '../useOpenModalCount'

describe('useOpenModalCount', () => {
  it('counts registrations while their scope is running and drops them on stop', () => {
    const count = useOpenModalCount()
    expect(count.value).toBe(0)

    const first = effectScope()
    first.run(() => useRegisterOpenModal())
    expect(count.value).toBe(1)

    const second = effectScope()
    second.run(() => useRegisterOpenModal())
    expect(count.value).toBe(2)

    first.stop()
    expect(count.value).toBe(1)

    second.stop()
    expect(count.value).toBe(0)
  })
})
