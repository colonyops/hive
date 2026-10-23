import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { RouteRecordRaw } from 'vue-router'
import { safeRedirect } from '../lib/redirect'
import SignIn from '../pages/SignIn.vue'
import VerifyCode from '../pages/VerifyCode.vue'
import { mountWithRouter, routerShell } from '../test-utils/mount'

// Stubbing fetch instead of api.ts also covers the error decoding the pages rely on.
const fetchMock = vi.fn<typeof fetch>()
vi.stubGlobal('fetch', fetchMock)

const routes: RouteRecordRaw[] = [
  { path: '/sign-in', name: 'sign-in', component: SignIn },
  { path: '/sign-in/code', name: 'verify', component: VerifyCode },
  { path: '/recent', name: 'recent', component: { template: '<div />' } },
  { path: '/:path(.*)*', component: { template: '<div />' } },
]

function mountAt(path: string) {
  return mountWithRouter(routerShell, { path, routes })
}

async function submitEmail(wrapper: VueWrapper, email: string) {
  await wrapper.get('[data-testid="sign-in-email"]').setValue(email)
  await wrapper.get('[data-testid="sign-in-form"]').trigger('submit')
  await flushPromises()
}

async function submitCode(wrapper: VueWrapper, code: string) {
  await wrapper.get('[data-testid="verify-code"]').setValue(code)
  await wrapper.get('[data-testid="verify-form"]').trigger('submit')
  await flushPromises()
}

const rateLimited = () =>
  new Response(JSON.stringify({ kind: 'rate_limited', message: 'too many sign-in attempts' }), {
    status: 429,
    headers: { 'Retry-After': '42' },
  })

describe('SignIn', () => {
  afterEach(() => fetchMock.mockReset())

  it('requests a code and moves to code entry with the email', async () => {
    fetchMock.mockResolvedValue(new Response('{}', { status: 202 }))
    const { router, wrapper } = await mountAt('/sign-in')

    await submitEmail(wrapper, 'ada@example.com')

    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/v1/auth/code')
    expect(init?.method).toBe('POST')
    expect(JSON.parse(init?.body as string)).toEqual({ email: 'ada@example.com' })
    expect(router.currentRoute.value.name).toBe('verify')
    expect(router.currentRoute.value.query).toEqual({ email: 'ada@example.com' })
  })

  it('shows the retry delay when rate limited and stays on the page', async () => {
    fetchMock.mockResolvedValue(rateLimited())
    const { router, wrapper } = await mountAt('/sign-in')

    await submitEmail(wrapper, 'ada@example.com')

    expect(wrapper.get('[data-testid="sign-in-error"]').text()).toContain('42 seconds')
    expect(router.currentRoute.value.name).toBe('sign-in')
  })
})

describe('VerifyCode', () => {
  afterEach(() => fetchMock.mockReset())

  it('lands on Recent without a redirect', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }))
    const { router, wrapper } = await mountAt('/sign-in/code?email=ada@example.com')

    await submitCode(wrapper, '12345678')

    expect(router.currentRoute.value.name).toBe('recent')
  })

  it('shows the same retry delay as sign-in when rate limited', async () => {
    fetchMock.mockResolvedValue(rateLimited())
    const { wrapper } = await mountAt('/sign-in/code?email=ada@example.com')

    await submitCode(wrapper, '12345678')

    expect(wrapper.get('[data-testid="verify-error"]').text()).toContain('42 seconds')
  })
})

describe('redirect after sign-in', () => {
  afterEach(() => fetchMock.mockReset())

  it('carries the destination from sign-in through code entry and lands there', async () => {
    fetchMock.mockResolvedValueOnce(new Response('{}', { status: 202 }))
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }))
    const destination = '/repos/acme/notes/docs/guide.md?x=1'
    const { router, wrapper } = await mountAt(`/sign-in?redirect=${encodeURIComponent(destination)}`)

    await submitEmail(wrapper, 'ada@example.com')
    expect(router.currentRoute.value.query.redirect).toBe(destination)
    await submitCode(wrapper, '12345678')

    expect(router.currentRoute.value.fullPath).toBe(destination)
  })

  it.each(['https://evil.example/x', '//evil.example/x', '/\\evil.example', '/\t/evil.example', 'javascript:alert(1)'])(
    'ignores the external redirect %j and lands on Recent',
    async (redirect) => {
      fetchMock.mockResolvedValue(new Response(null, { status: 204 }))
      const { router, wrapper } = await mountAt(
        `/sign-in/code?email=ada@example.com&redirect=${encodeURIComponent(redirect)}`,
      )

      await submitCode(wrapper, '12345678')

      expect(router.currentRoute.value.name).toBe('recent')
    },
  )

  it('accepts only same-app paths', () => {
    expect(safeRedirect('/tokens')).toBe('/tokens')
    expect(safeRedirect('/repos/a/b/c.md?q=1#h')).toBe('/repos/a/b/c.md?q=1#h')
    expect(safeRedirect('/sign-in/code')).toBeNull()
    expect(safeRedirect('//evil.example')).toBeNull()
    expect(safeRedirect('https://evil.example')).toBeNull()
    expect(safeRedirect(['/tokens'])).toBeNull()
    expect(safeRedirect(undefined)).toBeNull()
  })
})
