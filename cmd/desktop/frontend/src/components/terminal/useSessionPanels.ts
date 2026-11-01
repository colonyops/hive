import { useTimeoutFn } from '@vueuse/core'
import { computed, nextTick, ref, watch, type Ref } from 'vue'
import { moveTab, type PanelLayout } from '../../lib/panelLayout'
import { paneMayAutoFocus } from '../../lib/terminalTree'
import {
  focusedWindow,
  reconcileWindows,
  revealWindow,
  windowIdOfTab,
  windowOrder,
  windowsOnScreen,
} from '../../lib/windowTabs'
import { usePanelLayouts } from '../../stores/usePanelLayouts'

/** What a layout's window tabs mirror. The Code view builds one from its attach pool. */
export interface PanelWindowSource {
  /** The layout's key. Code keys a session by id, because a rename changes the slug. */
  key: Ref<string>
  windowIds: Ref<readonly string[]>
  activeWindow: Ref<string>
  /** The session ended for good (stopped, terminated, completed), so its layout goes with it. */
  closed: Ref<boolean>
  /** No window list yet, which says nothing: attaching, or an attach that failed while tmux may be fine. */
  pending: Ref<boolean>
  select(windowId: string): void
  moveWindow(windowId: string, position: number): void
  /** Draw the windows on screen: the active one and those beside it in other panels. */
  render(onScreen: Iterable<string>): void
  focus(): void
}

export type SessionPanelsCore = ReturnType<typeof useSessionPanels>

// A new window only has an id once tmux announces it. A request that never lands
// must not claim some later window.
const PENDING_WINDOW_MS = 5000

/**
 * Keeps a layout and its tmux windows in step. A layout change selects the
 * window it focuses, and a window tmux or a chord activated shows in the
 * layout. Both directions settle, because selecting the focused window changes
 * neither.
 */
export function useSessionPanels(source: PanelWindowSource, layouts = usePanelLayouts()) {
  const layout = computed(() => layouts.layoutOf(source.key.value))
  const onScreen = computed(() => windowsOnScreen(layout.value))

  function apply(rule: (layout: PanelLayout) => PanelLayout): void {
    layouts.apply(source.key.value, rule)
  }

  // A panel a "+" named for the window it is making.
  const newWindowPanel = ref<string | null>(null)
  // A window someone at this keyboard asked for, as opposed to one from an agent
  // or another client, which must not take the screen from a view.
  const windowExpected = ref(false)
  let revealedFromActive = false
  const { start: expirePending, stop: keepPending } = useTimeoutFn(
    () => {
      newWindowPanel.value = null
      windowExpected.value = false
    },
    PENDING_WINDOW_MS,
    { immediate: false },
  )

  watch(
    [source.key, source.windowIds, source.activeWindow, source.closed, source.pending],
    ([sessionKey, ids, active, closed, pending], before) => {
      // The session was closed, so its tabs go with it. A session merely not
      // running keeps its Drive, Diff and Tasks for when it starts.
      if (closed) {
        layouts.forget(sessionKey)
        return
      }
      if (pending) return
      const target = newWindowPanel.value ?? undefined
      apply((current) => reconcileWindows(current, ids, { active, target }))
      if (!before || before[0] !== sessionKey) return
      const known = before[1] ?? []
      const created = ids.find((id) => !known.includes(id))
      if (created) {
        if (windowExpected.value) syncWindowOrder(created)
        newWindowPanel.value = null
      }
      // A first active window is an attach landing, which must not pull the
      // restored tabs away from what they show.
      if (before[2] && active && active !== before[2]) {
        const was = focusedWindow(layout.value)
        if (was || windowExpected.value) apply((current) => revealWindow(current, active))
        revealedFromActive = focusedWindow(layout.value) !== was
      }
      if (created) {
        windowExpected.value = false
        keepPending()
      }
    },
    { immediate: true },
  )

  // Immediately after the layout, so select() runs before the DOM flip it waits
  // for. The window list is a source because a cold attach lands with the layout
  // unchanged, and tmux still has to be told which window it shows.
  watch([source.key, () => focusedWindow(layout.value), source.windowIds], ([sessionKey, windowId, ids], before) => {
    if (!windowId || !ids.includes(windowId)) return
    if (windowId !== source.activeWindow.value) {
      source.select(windowId)
      return
    }
    if (before && before[0] === sessionKey && before[1] === windowId) return
    // tmux already made this window active, and whoever did that does not get the keyboard.
    if (revealedFromActive) {
      revealedFromActive = false
      return
    }
    void nextTick(() => {
      source.render(onScreen.value.keys())
      if (paneMayAutoFocus.value) source.focus()
    })
  })

  function renderOnScreen(): void {
    source.render(onScreen.value.keys())
  }

  // The active window claims its renderer through select(); the one beside it
  // has to be asked.
  watch([onScreen, source.windowIds], renderOnScreen, { flush: 'post' })

  function expectWindowIn(panel?: string): void {
    newWindowPanel.value = panel ?? null
    windowExpected.value = true
    expirePending()
    paneMayAutoFocus.value = true
  }

  // tmux's numbering should agree with the tabs, so a tab that moves takes its window along.
  function syncWindowOrder(windowId: string): void {
    const position = windowOrder(layout.value).indexOf(windowId)
    if (position < 0) return
    if (source.windowIds.value.indexOf(windowId) !== position) source.moveWindow(windowId, position)
  }

  function moveTabTo(tabId: string, panel: string, gap: number): void {
    apply((current) => moveTab(current, tabId, panel, gap))
    const windowId = windowIdOfTab(layout.value, tabId)
    if (windowId) syncWindowOrder(windowId)
  }

  return { layout, onScreen, apply, renderOnScreen, expectWindowIn, moveTabTo, syncWindowOrder }
}
