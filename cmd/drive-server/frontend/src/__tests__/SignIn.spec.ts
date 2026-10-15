import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import SignIn from '../pages/SignIn.vue'

// Stubbing fetch instead of api.ts also covers the error decoding the page relies on.
const fetchMock = vi.fn<typeof fetch>()
vi.stubGlobal('fetch', fetchMock)

async function mountSignIn() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/sign-in', name: 'sign-in', component: SignIn },
      { path: '/sign-in/code', name: 'verify', component: { template: '<div />' } },
    ],
  })
  await router.push('/sign-in')
  const wrapper = mount(SignIn, { global: { plugins: [router] } })
  return { router, wrapper }
}

async function submitEmail(wrapper: Awaited<ReturnType<typeof mountSignIn>>['wrapper'], email: string) {
  await wrapper.get('[data-testid="sign-in-email"]').setValue(email)
  await wrapper.get('[data-testid="sign-in-form"]').trigger('submit')
  await flushPromises()
}

describe('SignIn', () => {
  afterEach(() => fetchMock.mockReset())

  it('requests a code and moves to code entry with the email', async () => {
    fetchMock.mockResolvedValue(new Response('{}', { status: 202 }))
    const { router, wrapper } = await mountSignIn()

    await submitEmail(wrapper, 'ada@example.com')

    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/v1/auth/code')
    expect(init?.method).toBe('POST')
    expect(JSON.parse(init?.body as string)).toEqual({ email: 'ada@example.com' })
    expect(router.currentRoute.value.name).toBe('verify')
    expect(router.currentRoute.value.query.email).toBe('ada@example.com')
  })

  it('shows the retry delay when rate limited and stays on the page', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ kind: 'rate_limited', message: 'too many sign-in attempts' }), {
        status: 429,
        headers: { 'Retry-After': '42' },
      }),
    )
    const { router, wrapper } = await mountSignIn()

    await submitEmail(wrapper, 'ada@example.com')

    expect(wrapper.get('[data-testid="sign-in-error"]').text()).toContain('42 seconds')
    expect(router.currentRoute.value.name).toBe('sign-in')
  })
})
