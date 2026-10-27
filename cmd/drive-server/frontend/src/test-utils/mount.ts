import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { vi } from 'vitest'
import { ref, type Component } from 'vue'
import { createMemoryHistory, createRouter, type Router, type RouteRecordRaw } from 'vue-router'
import { meKey, type MeContext } from '../lib/me'
import { createToast, toastKey, type Toast } from '../lib/toast'

// happy-dom would fetch an iframe's src for real, so specs read the src from a stand-in.
export const frameStub = {
  props: ['src', 'html', 'testid'],
  template: '<div data-testid="frame" :data-src="src" />',
}

/** A component that renders the router, for specs that mount a whole route tree. */
export const routerShell: Component = { template: '<RouterView />' }

export interface MountOptions {
  /** Where the router starts. */
  path?: string
  /** Defaults to one catch-all route, enough for code that only pushes paths. */
  routes?: RouteRecordRaw[]
  props?: Record<string, unknown>
}

export interface Mounted {
  router: Router
  wrapper: VueWrapper
  /** The toast the component sees; `show` is a spy. */
  toast: Toast
  /** The account context's `refresh`, a spy. */
  refresh: MeContext['refresh']
}

/** Mounts `component` behind a memory router with the app's injected contexts, then settles its requests. */
export async function mountWithRouter(component: Component, options: MountOptions = {}): Promise<Mounted> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: options.routes ?? [{ path: '/:path(.*)*', component: {} }],
  })
  await router.push(options.path ?? '/')
  const toast = createToast()
  toast.show = vi.fn(toast.show)
  const refresh = vi.fn<MeContext['refresh']>().mockResolvedValue(undefined)
  const wrapper = mount(component, {
    props: options.props,
    attachTo: document.body,
    global: {
      plugins: [router],
      stubs: { HtmlDocumentFrame: frameStub },
      provide: { [toastKey as symbol]: toast, [meKey as symbol]: { me: ref(null), refresh } },
    },
  })
  await flushPromises()
  return { router, wrapper, toast, refresh }
}
