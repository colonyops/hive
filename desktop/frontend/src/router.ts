import { defineComponent } from 'vue'
import {
  createRouter,
  createWebHashHistory,
  type Router,
  type RouterHistory,
} from 'vue-router'

export type AppRouteName = 'feed' | 'flows' | 'application-settings' | 'profile-settings'
export type ApplicationSettingsSection = 'appearance' | 'integrations'
export type ProfileSettingsSection = 'general' | 'danger'

// App.vue owns the persistent desktop shell and renders the matched page in
// its main slot. Vue Router still requires a component on leaf route records.
const ShellPage = defineComponent({ name: 'ShellPage', render: () => null })

export function createAppRouter(history: RouterHistory = createWebHashHistory()): Router {
  return createRouter({
    history,
    routes: [
      { path: '/', redirect: { name: 'feed' } },
      {
        path: '/feed/:profileId?',
        name: 'feed',
        component: ShellPage,
      },
      {
        path: '/flows/:profileId',
        name: 'flows',
        component: ShellPage,
      },
      {
        path: '/settings/:section(appearance|integrations)?',
        name: 'application-settings',
        component: ShellPage,
      },
      {
        path: '/profiles/:profileId/settings/:section(general|danger)?',
        name: 'profile-settings',
        component: ShellPage,
      },
      { path: '/:pathMatch(.*)*', redirect: { name: 'feed' } },
    ],
  })
}

export const router = createAppRouter()
