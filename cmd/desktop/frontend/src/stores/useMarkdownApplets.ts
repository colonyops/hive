import { computed, defineComponent, h, onScopeDispose, watch, type Component } from 'vue'
import { registerFence } from '@hive/ui/components/fences/registry'
import type { FenceProps } from '@hive/ui/components/fences/registry'
import AppletFence from '../components/applets/AppletFence.vue'
import { defineStore } from './defineStore'
import { useApplets } from './useApplets'

// Looked up by id on every render, so an edited applet (a new revision) reloads its blocks
// without a new registration.
function fenceComponent(appletID: string, lang: string): Component {
  return defineComponent({
    name: 'AppletFenceBound',
    props: ['data', 'source', 'ctx', 'testid'],
    setup(props) {
      const applets = useApplets()
      return () => {
        const applet = applets.byID(appletID)
        return applet ? h(AppletFence, { ...(props as FenceProps), applet, lang }) : null
      }
    },
  })
}

// Keeps the fence registry in step with the applets that declare a markdown surface
// (ADR applets-are-web-pages-with-full-access-to-the-machine).
export const useMarkdownApplets = defineStore('markdown-applets', () => {
  const applets = useApplets()

  // One entry per fence, keyed so a change of fences, body or the applet itself shows up as a difference.
  const wanted = computed(() => {
    const out = new Map<string, { applet: string; body: 'yaml' | 'text' }>()
    for (const applet of applets.applets.value) {
      const markdown = applet.surfaces.markdown
      if (!applet.runnable || !applet.enabled || !markdown?.enabled) continue
      for (const lang of markdown.fences ?? []) {
        out.set(lang, { applet: applet.id, body: markdown.body === 'text' ? 'text' : 'yaml' })
      }
    }
    return out
  })

  const registered = new Map<string, { applet: string; body: string; unregister: () => void }>()

  watch(
    wanted,
    (next) => {
      for (const [lang, entry] of registered) {
        const now = next.get(lang)
        if (now?.applet === entry.applet && now.body === entry.body) continue
        entry.unregister()
        registered.delete(lang)
      }
      for (const [lang, { applet, body }] of next) {
        if (registered.has(lang)) continue
        const unregister = registerFence({ lang, builtin: false, body, component: fenceComponent(applet, lang) })
        registered.set(lang, { applet, body, unregister })
      }
    },
    { immediate: true },
  )
  onScopeDispose(() => registered.forEach((entry) => entry.unregister()))

  return {}
})
