import { computed, readonly, ref, shallowReadonly, shallowRef, watch } from 'vue'
import { List } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice'
import {
  SetTitlebarApplets,
  TitlebarApplets,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import type { Applet } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import type { AppletSurface } from '../components/applets/useAppletBridge'
import type { Command } from '../composables/useCommands'
import { useWailsEvent } from '../composables/useWailsEvent'
import { appletCommandID, setAppletCommands } from '../keybindings/catalog'
import { launcherIconComponent } from '../lib/launcherIcons'
import { defineStore } from './defineStore'
import { usePersistedSetting } from './usePersistedSetting'
import { useResource } from './useResource'

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
  window: () => Window | null | undefined
  deliverCommand: (id: string) => void
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

  const titlebarIDs = usePersistedSetting<string[]>({
    initial: [],
    read: async () => (await TitlebarApplets()) ?? [],
    write: (ids) => SetTitlebarApplets(ids),
    label: 'the title bar applets',
  })
  const titlebar = computed(() =>
    titlebarIDs.value.value.flatMap((id) => {
      const applet = catalog.data.value.find((candidate) => candidate.id === id)
      return applet?.runnable && applet.titlebar ? [applet] : []
    }),
  )

  function isInTitlebar(id: string): boolean {
    return titlebarIDs.value.value.includes(id)
  }

  function setInTitlebar(id: string, shown: boolean): void {
    const others = titlebarIDs.value.value.filter((other) => other !== id)
    titlebarIDs.set(shown ? [...others, id] : others)
  }

  const dropdown = shallowRef<AppletDropdown | null>(null)

  function toggleDropdown(next: AppletDropdown): void {
    const current = dropdown.value
    dropdown.value = current?.applet.id === next.applet.id && current.entry === next.entry ? null : next
  }

  function closeDropdown(): void {
    dropdown.value = null
  }

  // The session on screen now. A title bar page outlives any one session, so it follows
  // this where a panel stays on the session it opened over.
  const liveSessionSlug = ref('')

  function setLiveSession(slug: string): void {
    liveSessionSlug.value = slug
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
    if (handle.surface === 'page') {
      commandsForPanel.get(id)?.forEach((command) => handle.deliverCommand(command))
      commandsForPanel.delete(id)
    }
    return () => {
      connected.delete(handle)
      setFrameRows(handle, id, [])
    }
  }

  function otherFrames(id: string, self: Window): Window[] {
    return [...(frames.get(id) ?? [])].flatMap((handle) => {
      const frame = handle.window()
      return frame && frame !== self ? [frame] : []
    })
  }

  // The title bar page first, because it is the one that keeps running: a panel over it
  // may come and go, and two frames acting on one command would run it twice.
  function deliverCommand(id: string, command: string): boolean {
    const connected = [...(frames.get(id) ?? [])]
    const target =
      connected.find((handle) => handle.surface === 'titlebar') ?? connected.find((handle) => handle.surface === 'page')
    target?.deliverCommand(command)
    return target !== undefined
  }

  function openForCommand(id: string, command: string, session: string): void {
    commandsForPanel.set(id, [...(commandsForPanel.get(id) ?? []), command])
    sessionSlug.value = session
    openID.value = id
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
    const byID = new Map<string, Command>()
    for (const [handle, { applet, rows }] of frameRows.value) {
      const title = catalog.data.value.find((candidate) => candidate.id === applet)?.title ?? applet
      for (const row of rows) {
        const id = `applet:${applet}:${row.id}`
        byID.set(id, {
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
    return [...byID.values()]
  })

  watch(
    catalog.data,
    (applets) => {
      // Unbound until the user says otherwise: a file an agent wrote must not claim a
      // chord the app never offered it.
      setAppletCommands(
        applets.flatMap((applet) => [
          {
            id: appletCommandID(applet.id),
            title: applet.runnable ? applet.title : `${applet.title} (invalid)`,
            group: 'Applets',
            keywords: ['applet', applet.id],
            icon: launcherIconComponent(applet.icon),
            defaultCombos: [],
            context: 'global' as const,
            scope: 'applets' as const,
          },
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
    titlebar,
    isInTitlebar,
    setInTitlebar,
    // Shallow: the anchor is a DOM node, whose methods refuse a readonly proxy.
    dropdown: shallowReadonly(dropdown),
    toggleDropdown,
    closeDropdown,
    liveSessionSlug: readonly(liveSessionSlug),
    setLiveSession,
    setCommandRunner,
    runCommand,
    connectFrame,
    otherFrames,
    deliverCommand,
    openForCommand,
    setFrameRows,
    pageRows,
  }
})
