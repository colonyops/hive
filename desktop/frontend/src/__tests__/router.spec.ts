import { describe, expect, it } from 'vitest'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../router'

describe('createAppRouter', () => {
  it('registers the developer tools route in dev mode', () => {
    const router = createAppRouter(createMemoryHistory())

    expect(router.resolve('/dev').name).toBe('dev')
  })
})
