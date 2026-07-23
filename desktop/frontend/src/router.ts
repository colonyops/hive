import { defineComponent } from 'vue'
import {
  createRouter,
  createWebHashHistory,
  type RouteRecordRaw,
  type Router,
  type RouterHistory,
} from 'vue-router'

export type AppRouteName = 'feed' | 'flows' | 'activity' | 'application-settings' | 'profile-settings' | 'dev'
export type ApplicationSettingsSection = 'appearance' | 'integrations' | 'actions' | 'keybindings' | 'system' | 'notifications'
export type ProfileSettingsSection = 'general' | 'danger'

// App.vue owns the persistent desktop shell and renders the matched page in
// its main slot. Vue Router still requires a component on leaf route records.
const ShellPage = defineComponent({ name: 'ShellPage', render: () => null })

export function createAppRouter(history: RouterHistory = createWebHashHistory()): Router {
  const routes: RouteRecordRaw[] = [
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
      // Activity is app-global (the audit log spans every profile), so it
      // takes no profileId param.
      path: '/activity',
      name: 'activity',
      component: ShellPage,
    },
    {
      path: '/settings/:section(appearance|integrations|actions|keybindings|system|notifications)?',
      name: 'application-settings',
      component: ShellPage,
    },
    {
      path: '/profiles/:profileId/settings/:section(general|danger)?',
      name: 'profile-settings',
      component: ShellPage,
    },
  ]

  // Keep this record out of production bundles entirely; the catchall below
  // handles /dev there just like any other unknown route.
  if (import.meta.env.DEV) {
    routes.push({ path: '/dev', name: 'dev', component: ShellPage })
  }

  routes.push({ path: '/:pathMatch(.*)*', redirect: { name: 'feed' } })

  return createRouter({ history, routes })
}

export const router = createAppRouter()
