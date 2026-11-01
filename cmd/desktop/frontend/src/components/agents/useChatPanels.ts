import { computed, ref, type Ref } from 'vue'
import { isEditableTarget } from '../../lib/isEditableTarget'
import { terminalPanel } from '../../lib/windowTabs'
import { useChatPanelLayouts } from '../../stores/usePanelLayouts'
import { useSessionPanels, type PanelWindowSource } from '../terminal/useSessionPanels'

export interface ChatPaneState {
  status: Ref<'idle' | 'opening' | 'live'>
  /** The chat the pane holds. Null until a launch answers, and after the pane closes. */
  chatId: Ref<string | null>
  /** A route names a chat that is on its way to the pane. */
  resuming: Ref<boolean>
  routeChatId: Ref<string | null>
  /** The tmux window the pane draws, once attached. */
  windowId: Ref<string>
  focus(): void
}

/**
 * The open chat as a panel layout of one window tab, the chat's terminal, and
 * whatever views the user opens beside it. A chat has one window, so there is
 * nothing to select, move or render: the pane draws itself.
 */
export function useChatPanels(pane: ChatPaneState) {
  const layouts = useChatPanelLayouts()

  const key = computed(() => {
    if (pane.status.value !== 'idle') return pane.chatId.value ?? ''
    return pane.resuming.value ? (pane.routeChatId.value ?? '') : ''
  })
  const live = computed(() => pane.status.value === 'live' && pane.windowId.value !== '')
  const windowIds = computed(() => (live.value ? [pane.windowId.value] : []))
  const source: PanelWindowSource = {
    key,
    windowIds,
    activeWindow: computed(() => (live.value ? pane.windowId.value : '')),
    // The layout outlives a stopped pane; closing the chat in the app forgets it.
    closed: ref(false),
    // A resume has no window yet, and reconciling now would drop the restored window tab.
    pending: computed(() => pane.status.value === 'opening' || pane.resuming.value),
    select: () => {},
    moveWindow: () => {},
    render: () => {},
    focus() {
      // A launch that lands while the user types elsewhere must not take that focus.
      if (!isEditableTarget(document.activeElement)) pane.focus()
    },
  }
  const core = useSessionPanels(source, layouts)

  const windowPanel = computed(() => (live.value ? core.onScreen.value.get(pane.windowId.value) : undefined))
  // Before the window is known, or with its tab behind another, the pane's states still need a place.
  const statusPanel = computed(() => terminalPanel(core.layout.value))

  return { ...core, key, windowPanel, statusPanel, forget: layouts.forget }
}
