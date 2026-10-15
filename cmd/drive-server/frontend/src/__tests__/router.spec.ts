import { describe, expect, it, vi } from 'vitest'
import { router } from '../router'

vi.mock('../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api')>()),
  api: { me: vi.fn().mockResolvedValue({ email: 'ada@example.com' }) },
}))

describe('router', () => {
  it('lands on Recent from the root', async () => {
    await router.push('/')

    expect(router.currentRoute.value.fullPath).toBe('/recent')
  })
})
