import { computed, onScopeDispose, watch, type Ref } from 'vue'
import { useEventListener } from '@vueuse/core'
import { comboFromEvent, SEQUENCE_TIMEOUT_MS, terminalEscapeCombo, useKeybindings } from '../composables/useKeybindings'
import { useOpenModalCount } from '../composables/useOpenModalCount'
import { commandById, commandPiercesPane, launcherActionID, type CommandContext } from '../keybindings/catalog'
import { isEditableTarget, isTerminalTarget } from '../lib/isEditableTarget'

export interface GlobalKeymapDeps {
  runCommand: (id: string) => void
  contextActive: (context: CommandContext) => boolean
  paletteOpen: Ref<boolean>
  activityOpen: Ref<boolean>
  tasksOpen: Ref<boolean>
}

/**
 * Resolves window keydowns against the configurable keymap and runs the
 * matched command. Bare keys are ignored while typing, a command fires only in
 * its context, and an open overlay suppresses everything but the palette
 * toggle.
 */
export function useGlobalKeymap({ runCommand, contextActive, paletteOpen, activityOpen, tasksOpen }: GlobalKeymapDeps) {
  const kb = useKeybindings()

  // Every dialog and drawer registers in openModalCount while mounted. The
  // palette and the two hub overlays are not modals, so they are named here.
  // Tasks is split out so tasks.toggle can close its own overlay while it stays
  // suppressed under anything else.
  const openModalCount = useOpenModalCount()
  const otherOverlayOpen = computed(() => paletteOpen.value || activityOpen.value || openModalCount.value > 0)
  const anyOverlayOpen = computed(() => otherOverlayOpen.value || tasksOpen.value)

  // One timer at a time, for a sequence prefix that is also a complete binding
  // (Zed's prefix rule). Its fire goes through the same gate as a keystroke
  // rather than trusting the state from when it was armed.
  let sequenceTimer: ReturnType<typeof setTimeout> | null = null

  function cancelSequenceTimer(): void {
    if (sequenceTimer === null) return
    clearTimeout(sequenceTimer)
    sequenceTimer = null
  }

  function resetSequence(): void {
    cancelSequenceTimer()
    kb.clearPendingSequence()
  }

  function dispatchIfActive(id: string): boolean {
    const command = commandById.value.get(id)
    if (!command) return false
    if (anyOverlayOpen.value && id !== 'palette.toggle') {
      // A different modal (report, new-profile, a confirm stacked inside Tasks)
      // still swallows tasks.toggle like any other command.
      const closesTasksOverlay = id === 'tasks.toggle' && tasksOpen.value && !otherOverlayOpen.value
      if (!closesTasksOverlay) return false
    }
    if (!contextActive(command.context)) return false
    runCommand(id)
    return true
  }

  function armSequenceTimer(deferredCommandId: string): void {
    sequenceTimer = setTimeout(() => {
      sequenceTimer = null
      kb.clearPendingSequence()
      dispatchIfActive(deferredCommandId)
    }, SEQUENCE_TIMEOUT_MS)
  }

  // A sequence started before the palette opened (by a chord, or a mouse click
  // that never reaches stepSequence) has nowhere to go once it does.
  watch(paletteOpen, (open) => {
    if (open) resetSequence()
  })

  function onKeydown(e: KeyboardEvent): void {
    if (isTerminalTarget(e.target) && !kb.recording.value && !anyOverlayOpen.value) {
      // `piercesPane` commands are claimed on the binding alone: an overlay's
      // own combo has to close it, reaching the list is the pane's way out,
      // and window jumps are wanted from inside the window being left. A
      // launcher pierces for the pop-up's reason, but answers to its context
      // so its chord falls through with no terminal attached (ADR
      // quick-terminal-launchers-are-session-scoped).
      const id = kb.resolve(comboFromEvent(e) ?? '')
      const pierces = !!id && (commandPiercesPane(id) || launcherActionID(id) !== null)
      if (id && pierces && contextActive(commandById.value.get(id)?.context ?? 'global')) {
        resetSequence()
        e.preventDefault()
        runCommand(id)
        return
      }
      // `escapesPane` commands fire over a pane only on modifiers a terminal
      // cannot use: a bare Ctrl+K stays readline's kill-to-end-of-line.
      const escaped = kb.resolve(terminalEscapeCombo(e) ?? '')
      const command = escaped ? commandById.value.get(escaped) : undefined
      if (escaped && command?.escapesPane && contextActive(command.context)) {
        e.preventDefault()
        runCommand(escaped)
        return
      }
    }

    // A focused terminal owns every key so tmux prefixes reach the pane. It
    // cannot host a pending sequence either: the pane would swallow whatever
    // completes it.
    if (isTerminalTarget(e.target)) {
      resetSequence()
      return
    }

    // WebKit can treat an unhandled Backspace as browser Back.
    if (e.key === 'Backspace' && !isEditableTarget(e.target)) e.preventDefault()

    if (kb.recording.value) return // the settings editor is capturing this key
    const combo = comboFromEvent(e)
    if (!combo) return

    const transition = kb.stepSequence(kb.pendingSequence.value, combo)
    switch (transition.kind) {
      case 'run':
        resetSequence()
        if (dispatchIfActive(transition.commandId)) e.preventDefault()
        return
      case 'extend': {
        // Only a sequence's start is barred from editable fields and overlays;
        // whatever opened either has already cleared a pending one. A barred
        // start falls through: the combo may also be a complete binding in its
        // own right (Zed's prefix rule).
        const isStart = kb.pendingSequence.value === null
        if (isStart && (isEditableTarget(e.target) || anyOverlayOpen.value)) break
        cancelSequenceTimer()
        kb.pendingSequence.value = transition.pending
        e.preventDefault()
        if (transition.deferredCommandId) armSequenceTimer(transition.deferredCommandId)
        return
      }
      case 'swallow':
        resetSequence()
        e.preventDefault()
        return
      case 'pass':
        if (kb.pendingSequence.value) resetSequence()
        break
    }

    const id = kb.resolve(combo)
    if (!id) return

    const mods = combo.split('+')
    const hasModifier = mods.includes('mod') || mods.includes('ctrl') || mods.includes('alt')
    if (isEditableTarget(e.target) && !hasModifier) return

    if (dispatchIfActive(id)) e.preventDefault()
  }

  useEventListener(window, 'keydown', onKeydown)
  // A pane or an editable field owns the next keystroke, so a sequence cannot
  // survive a focus change into either, keystroke or not.
  useEventListener(window, 'focusin', (e: FocusEvent) => {
    if (isTerminalTarget(e.target) || isEditableTarget(e.target)) resetSequence()
  })
  onScopeDispose(cancelSequenceTimer)
}
