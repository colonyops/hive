import { computed, readonly, ref, shallowReadonly, shallowRef, watch } from 'vue'
import {
  Delete,
  List,
  SetEnabled,
  SetSurface,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice'
import type { Applet } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import type { Command } from '../composables/useCommands'
import { useToasts } from '../composables/useToasts'
import { useWailsEvent } from '../composables/useWailsEvent'
import { appletCommandID, setAppletCommands } from '../keybindings/catalog'
import { appletIdOf, appletTabId } from '../lib/appletTabs'
import { launcherIconComponent } from '../lib/launcherIcons'
import type { PanelTab } from '../lib/panelLayout'
import { SURFACES, type AppletSurface } from './appletSurfaces'
import { defineStore } from './defineStore'
import { useChatPanelLayouts, usePanelLayouts } from './usePanelLayouts'
import { useResource } from './useResource'

export const BUILDER_ID = 'applet-builder'

/** The session area the user is in, which can show an applet as a tab. */
export interface AppletTabHost {
  open: (id: string) => void
  sessionSlug: () => string
}

export interface AppletDropdown {
  applet: Applet
  entry: string
  width: number
  height: number
  anchor: HTMLElement
}

// One open frame of an applet, as the rest of the applet and the palette reach it.
export interface AppletFrameHandle {
  surface: AppletSurface
  post: (message: object) => void
  deliverCommand: (id: string) => void
  /** A tab behind another, or in the area the user left, is running but not on screen. */
  onScreen: () => boolean
}

// A row a running page asked for through hive.setCommands.
export interface AppletPageRow {
  id: string
  title: string
  group?: string
  keywords?: string[]
  icon?: string
  hint?: string
  kind?: string
  scope?: 'goto' | 'actions'
}

/** A broken file still opens, to show its error. An applet only in the title bar has nothing to open. */
function opens(applet: Applet): boolean {
  return !applet.runnable || applet.surfaces.tab || applet.surfaces.popup !== null
}

function appletPaletteTitle(applet: Applet): string {
  if (!applet.runnable) return `${applet.title} (invalid)`
  return applet.warnings?.length ? `${applet.title} (warnings)` : applet.title
}

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

  const open = computed(() => {
    const applet = openID.value === null ? undefined : byID(openID.value)
    return applet?.enabled ? applet : null
  })

  const usable = computed(() => catalog.data.value.filter((applet) => applet.runnable && applet.enabled))
  const sessionMenu = computed(() => usable.value.filter((applet) => applet.surfaces.tab))
  const titlebarApplets = computed(() => usable.value.filter((applet) => applet.surfaces.titlebar))

  // The session area on screen, which can show an applet as a tab. Both areas stay
  // mounted, so only the one the user is in holds this.
  const tabHost = shallowRef<AppletTabHost | null>(null)

  function setTabHost(host: AppletTabHost, active: boolean): void {
    if (active) tabHost.value = host
    else if (tabHost.value === host) tabHost.value = null
  }

  // The session on screen, in Code or Chats. A title bar page outlives any one session, so it
  // follows this where a pop-up stays on the session it opened over.
  const routeSessionSlug = ref('')
  const liveSessionSlug = computed(() => tabHost.value?.sessionSlug() ?? routeSessionSlug.value)

  /** What the route names while no area can answer yet, as during a cold attach. */
  function setRouteSession(slug: string): void {
    routeSessionSlug.value = slug
  }

  function openTabFor(id: string): boolean {
    if (!tabHost.value || !sessionMenu.value.some((applet) => applet.id === id)) return false
    // A pop-up left open would cover the tab the user just asked for.
    openID.value = null
    tabHost.value.open(id)
    return true
  }

  // A tab comes first where one can show, and an applet shows only where its file says.
  function openApplet(id: string): boolean {
    const applet = byID(id)
    if (!applet) return false
    if (openTabFor(id)) return true
    if (!applet.runnable || applet.surfaces.popup) {
      openPopup(id)
      return true
    }
    useToasts().showToast(
      applet.surfaces.tab
        ? `${applet.title} opens as a session tab. Open a session first.`
        : `${applet.title} has nothing to open.`,
      { severity: 'info' },
    )
    return false
  }

  async function setEnabled(id: string, enabled: boolean): Promise<void> {
    await SetEnabled(id, enabled)
    await catalog.reload()
  }

  async function setSurface(id: string, surface: AppletSurface, on: boolean): Promise<void> {
    await SetSurface(id, surface, on)
    await catalog.reload()
  }

  // A saved tab opens its page with the session and nobody asking. A later applet that
  // takes this id is someone else's code, so it must not inherit one.
  function closeSavedTabs(id: string): void {
    usePanelLayouts().closeTabEverywhere(appletTabId(id))
    useChatPanelLayouts().closeTabEverywhere(appletTabId(id))
  }

  async function remove(id: string): Promise<void> {
    await Delete(id)
    closeSavedTabs(id)
    await catalog.reload()
  }

  function byID(id: string): Applet | undefined {
    return catalog.data.value.find((applet) => applet.id === id)
  }

  function titleOf(id: string): string {
    return byID(id)?.title ?? id
  }

  /** What an applet's tab is called, or nothing for any other tab. */
  function tabTitle(tab: PanelTab): string | undefined {
    const id = appletIdOf(tab)
    return id ? titleOf(id) : undefined
  }

  const dropdown = shallowRef<AppletDropdown | null>(null)

  function toggleDropdown(next: AppletDropdown): void {
    const current = dropdown.value
    dropdown.value = current?.applet.id === next.applet.id && current.entry === next.entry ? null : next
  }

  function closeDropdown(): void {
    dropdown.value = null
  }

  let commandRunner: (id: string) => void = () => {}

  function setCommandRunner(run: (id: string) => void): void {
    commandRunner = run
  }

  function runCommand(id: string): void {
    commandRunner(id)
  }

  // An applet's open frames, so one can reach the others. Windows are not state a view
  // renders, so this stays out of the reactive graph.
  const frames = new Map<string, Set<AppletFrameHandle>>()
  // Commands waiting for the panel that opened to run them.
  const commandsForPanel = new Map<string, string[]>()

  function connectFrame(id: string, handle: AppletFrameHandle): () => void {
    const connected = frames.get(id) ?? new Set()
    frames.set(id, connected)
    connected.add(handle)
    frameShown(id, handle)
    return () => {
      connected.delete(handle)
      setFrameRows(handle, id, [])
    }
  }

  // A frame that comes on screen takes what was waiting for the applet to open. A saved tab
  // that connects behind another has not opened for anyone yet.
  function frameShown(id: string, handle: AppletFrameHandle): void {
    if (!SURFACES[handle.surface].panel || !handle.onScreen()) return
    commandsForPanel.get(id)?.forEach((command) => handle.deliverCommand(command))
    commandsForPanel.delete(id)
  }

  function otherFrames(id: string, self: AppletFrameHandle): AppletFrameHandle[] {
    return [...(frames.get(id) ?? [])].filter((handle) => handle !== self)
  }

  // The running persistent surface first, because it is the one that keeps running: a panel
  // over it may come and go, and two frames acting on one command would run it twice. A
  // tab that is not on screen does not count: it can belong to another session or area, so
  // the command would act on what the user is not looking at.
  function deliverCommand(id: string, command: string): boolean {
    const connected = [...(frames.get(id) ?? [])]
    const target =
      connected.find((handle) => SURFACES[handle.surface].persistent) ??
      connected.find((handle) => SURFACES[handle.surface].panel && handle.onScreen())
    target?.deliverCommand(command)
    return target !== undefined
  }

  function openForCommand(id: string, command: string): void {
    commandsForPanel.set(id, [...(commandsForPanel.get(id) ?? []), command])
    if (!openApplet(id)) commandsForPanel.delete(id)
  }

  const frameRows = shallowRef(new Map<AppletFrameHandle, { applet: string; rows: AppletPageRow[] }>())

  function setFrameRows(handle: AppletFrameHandle, applet: string, rows: AppletPageRow[]): void {
    const next = new Map(frameRows.value)
    if (rows.length > 0) next.set(handle, { applet, rows })
    else next.delete(handle)
    frameRows.value = next
  }

  // Data rather than commands, so nothing binds to them: they exist while the page does.
  const pageRows = computed<Command[]>(() => {
    const rowsByID = new Map<string, Command>()
    for (const [handle, { applet, rows }] of frameRows.value) {
      // A row acts on its page, so a page the user cannot see offers none.
      if (!handle.onScreen()) continue
      const title = titleOf(applet)
      for (const row of rows) {
        const id = `applet:${applet}:${row.id}`
        rowsByID.set(id, {
          id,
          title: row.title,
          group: row.group ?? title,
          keywords: ['applet', applet, title, ...(row.keywords ?? [])],
          icon: row.icon ? launcherIconComponent(row.icon) : undefined,
          hint: row.hint,
          kind: row.kind,
          scope: row.scope,
          run: () => handle.deliverCommand(row.id),
        })
      }
    }
    return [...rowsByID.values()]
  })

  watch(
    catalog.data,
    (applets) => {
      const shown = applets.find((applet) => applet.id === openID.value)
      if (!shown?.enabled || !(shown.surfaces.popup || !shown.runnable)) openID.value = null
      // An edit replaces the title bar frame the dropdown is anchored to and serves new pages.
      const open = dropdown.value?.applet
      if (open && !applets.some((a) => a.id === open.id && a.enabled && a.revision === open.revision)) {
        dropdown.value = null
      }
      // Unbound until the user says otherwise: a file an agent wrote must not claim a
      // chord the app never offered it.
      setAppletCommands(
        applets
          .filter((applet) => applet.enabled)
          .flatMap((applet) => [
            ...(opens(applet)
              ? [
                  {
                    id: appletCommandID(applet.id),
                    title: appletPaletteTitle(applet),
                    group: 'Applets',
                    keywords: ['applet', applet.id],
                    icon: launcherIconComponent(applet.icon),
                    defaultCombos: [],
                    context: 'global' as const,
                    scope: 'applets' as const,
                  },
                ]
              : []),
            ...(applet.runnable ? (applet.commands ?? []) : []).map((command) => ({
              id: appletCommandID(applet.id, command.id),
              title: command.title,
              group: applet.title,
              keywords: ['applet', applet.id, applet.title, ...(command.keywords ?? [])],
              icon: launcherIconComponent(command.icon || applet.icon),
              defaultCombos: [],
              context: 'global' as const,
              scope: 'applets' as const,
            })),
          ]),
      )
    },
    { immediate: true },
  )

  useWailsEvent('applets:updated', () => {
    void catalog.reload()
  })
  void catalog.reload()

  function toggle(id: string): void {
    if (openID.value === id) {
      openID.value = null
      return
    }
    openApplet(id)
  }

  function openPopup(id: string): void {
    sessionSlug.value = liveSessionSlug.value
    openID.value = id
  }

  function close(): void {
    openID.value = null
  }

  return {
    applets: readonly(catalog.data),
    loaded: catalog.loaded,
    open,
    sessionSlug: readonly(sessionSlug),
    reload: catalog.reload,
    toggle,
    close,
    sessionMenu,
    byID,
    tabTitle,
    setTabHost,
    setRouteSession,
    titlebarApplets,
    closeSavedTabs,
    setEnabled,
    setSurface,
    remove,
    // Shallow: the anchor is a DOM node, whose methods refuse a readonly proxy.
    dropdown: shallowReadonly(dropdown),
    toggleDropdown,
    closeDropdown,
    liveSessionSlug,
    setCommandRunner,
    runCommand,
    connectFrame,
    frameShown,
    otherFrames,
    deliverCommand,
    openForCommand,
    setFrameRows,
    pageRows,
  }
})
