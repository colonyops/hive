import { computed, readonly, ref, watch } from 'vue'
import { List } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice'
import type { Applet } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { useWailsEvent } from '../composables/useWailsEvent'
import { appletCommandID, setAppletCommands } from '../keybindings/catalog'
import { launcherIconComponent } from '../lib/launcherIcons'
import { defineStore } from './defineStore'
import { useResource } from './useResource'

// The applets on disk and the one on screen. Each is a bindable command, so this
// is what turns a file an agent wrote into a palette row and a chord (ADR
// applets-are-web-pages-with-full-access-to-the-machine).
export const useApplets = defineStore('applets', () => {
  const catalog = useResource(async () => (await List()) ?? [], {
    initial: [] as Applet[],
    errorFallback: 'Could not load the applets.',
  })
  const openID = ref<string | null>(null)
  // Captured on open: an applet stays on the session it opened over, and the user
  // cannot switch sessions behind the overlay.
  const sessionSlug = ref('')

  const open = computed(() => catalog.data.value.find((applet) => applet.id === openID.value) ?? null)

  watch(
    catalog.data,
    (applets) => {
      setAppletCommands(
        applets.map((applet) => ({
          id: appletCommandID(applet.id),
          title: applet.runnable ? applet.title : `${applet.title} (invalid)`,
          group: 'Applets',
          keywords: ['applet', applet.id],
          icon: launcherIconComponent(applet.icon),
          // Unbound until the user says otherwise: a file an agent wrote must not
          // claim a chord the app never offered it.
          defaultCombos: [],
          context: 'global',
          scope: 'applets',
        })),
      )
    },
    { immediate: true },
  )

  useWailsEvent('applets:updated', () => {
    void catalog.reload()
  })
  void catalog.reload()

  function toggle(id: string, session: string): void {
    if (openID.value === id) {
      openID.value = null
      return
    }
    sessionSlug.value = session
    openID.value = id
  }

  function close(): void {
    openID.value = null
  }

  return {
    applets: readonly(catalog.data),
    open,
    sessionSlug: readonly(sessionSlug),
    reload: catalog.reload,
    toggle,
    close,
  }
})
