import { createRouter, createWebHistory } from 'vue-router'
import { api, ApiError, onUnauthorized } from './api'
import { redirectQuery } from './lib/redirect'
import type { Me } from './types'

let signedInAs: Me | null = null

export function forgetSignIn(): void {
  signedInAs = null
}

/** The account the route guard signed in, so the layout does not ask again. */
export function signedInUser(): Me | null {
  return signedInAs
}

export function rememberSignIn(me: Me): void {
  signedInAs = me
}

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/sign-in',
      name: 'sign-in',
      component: () => import('./pages/SignIn.vue'),
      meta: { public: true },
      // The sign-in mail links here with the address filled in.
      beforeEnter: (to) => (to.query.email ? { name: 'verify', query: to.query } : true),
    },
    {
      path: '/sign-in/code',
      name: 'verify',
      component: () => import('./pages/VerifyCode.vue'),
      meta: { public: true },
    },
    {
      path: '/s/:slug',
      name: 'share',
      component: () => import('./pages/ShareView.vue'),
      props: true,
      meta: { public: true },
    },
    {
      path: '/',
      component: () => import('./pages/AppLayout.vue'),
      children: [
        { path: '', redirect: { name: 'recent' } },
        {
          path: 'recent',
          name: 'recent',
          component: () => import('./pages/DocumentFeed.vue'),
          props: { feed: 'recent' },
        },
        { path: 'links', name: 'links', component: () => import('./pages/DocumentFeed.vue'), props: { feed: 'links' } },
        { path: 'tokens', name: 'tokens', component: () => import('./pages/Tokens.vue') },
        { path: 'repos/:owner/:repo/:path(.*)*', name: 'repo', component: () => import('./pages/SpaceRoute.vue') },
        { path: 'workspaces/:name/:path(.*)*', name: 'workspace', component: () => import('./pages/SpaceRoute.vue') },
        { path: 'shared/:path(.*)*', name: 'shared', component: () => import('./pages/SpaceRoute.vue') },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.public || signedInAs) return true
  try {
    signedInAs = await api.me()
    return true
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) return { name: 'sign-in', query: redirectQuery(to.fullPath) }
    throw error
  }
})

// Before the guard has signed anyone in, a 401 is the guard's to handle.
onUnauthorized(() => {
  if (!signedInAs) return
  forgetSignIn()
  void router.push({ name: 'sign-in', query: redirectQuery(router.currentRoute.value.fullPath) })
})
