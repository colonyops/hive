import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api'
import { forgetSignIn, router, signedInUser } from '../router'

// Stubbing fetch instead of api.ts runs the real 401 path the router hooks into.
const fetchMock = vi.fn<typeof fetch>()
vi.stubGlobal('fetch', fetchMock)

const me = { email: 'ada@example.com', policy: 'default', quota: { used: 0, limit: null } }
const ok = (body: unknown) => new Response(JSON.stringify(body), { status: 200 })
const unauthorized = () =>
  new Response(JSON.stringify({ kind: 'unauthenticated', message: 'sign in' }), { status: 401 })

describe('router', () => {
  beforeEach(async () => {
    fetchMock.mockReset()
    forgetSignIn()
    fetchMock.mockImplementation(() => Promise.resolve(unauthorized()))
    await router.push('/sign-in')
  })

  it('lands on Recent from the root and keeps the account it fetched', async () => {
    fetchMock.mockImplementation(() => Promise.resolve(ok(me)))

    await router.push('/')

    expect(router.currentRoute.value.fullPath).toBe('/recent')
    expect(signedInUser()).toEqual(me)
  })

  it('sends a signed-out visitor to sign-in with the page they asked for', async () => {
    await router.push('/repos/acme/notes/docs/guide.md?x=1')

    expect(router.currentRoute.value.name).toBe('sign-in')
    expect(router.currentRoute.value.query.redirect).toBe('/repos/acme/notes/docs/guide.md?x=1')
  })

  it('sends the user back to sign-in when the session ends mid-visit', async () => {
    fetchMock.mockImplementation(() => Promise.resolve(ok(me)))
    await router.push('/tokens')

    fetchMock.mockImplementation(() => Promise.resolve(unauthorized()))
    await expect(api.tokens()).rejects.toMatchObject({ status: 401 })
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('sign-in')
    expect(router.currentRoute.value.query.redirect).toBe('/tokens')
    expect(signedInUser()).toBeNull()
  })

  it('leaves a wrong sign-in code to the page', async () => {
    fetchMock.mockImplementation(() => Promise.resolve(ok(me)))
    await router.push('/tokens')

    fetchMock.mockImplementation(() => Promise.resolve(unauthorized()))
    await expect(api.verifySignIn('ada@example.com', '0')).rejects.toMatchObject({ status: 401 })
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('tokens')
  })
})
