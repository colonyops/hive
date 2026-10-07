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
  canvasOpen: Ref<boolean>
  actionRunsOpen: Ref<boolean>
}

/**
 * Resolves window keydowns against the configurable keymap and runs the
 * matched command. Bare keys are ignored while typing, a command fires only in
 * its context, and an open overlay suppresses everything but the palette
 * toggle.
 */
export function useGlobalKeymap({
  runCommand,
  contextActive,
  paletteOpen,
  activityOpen,
  tasksOpen,
  canvasOpen,
  actionRunsOpen,
}: GlobalKeymapDeps) {
  const kb = useKeybindings()

  // Every dialog and drawer registers in openModalCount while mounted. The
  // palette and the hub overlays are not modals, so they are named here. Tasks
  // and Canvases are split out so each one's toggle can close its own overlay
  // while it stays suppressed under anything else.
  const openModalCount = useOpenModalCount()
  const toggledOverlays: Record<string, Ref<boolean>> = {
    'tasks.toggle': tasksOpen,
    'canvas.toggle': canvasOpen,
    'action-runs.toggle': actionRunsOpen,
  }
  const otherOverlayOpen = computed(() => paletteOpen.value || activityOpen.value || openModalCount.value > 0)
  const anyOverlayOpen = computed(
    () => otherOverlayOpen.value || Object.values(toggledOverlays).some((open) => open.value),
  )

  function closesOwnOverlay(id: string): boolean {
    if (otherOverlayOpen.value || !toggledOverlays[id]?.value) return false
    return Object.entries(toggledOverlays).every(([toggle, open]) => toggle === id || !open.value)
  }

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
    // A different modal (report, new-profile, a confirm stacked inside Tasks)
    // still swallows an overlay's own toggle like any other command.
    if (anyOverlayOpen.value && id !== 'palette.toggle' && !closesOwnOverlay(id)) return false
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

  // A command resolves only where it applies, so ⌘F reaches terminal.find over
  // a pane and Focus search everywhere else.
  function applies(id: string): boolean {
    const context = commandById.value.get(id)?.context
    return context !== 'terminal-pane' || contextActive(context)
  }

  // Shared with the pane: a sequence step either dispatches, extends the
  // pending sequence, or is swallowed. Returns false for 'pass', which leaves
  // the key to the caller.
  function applySequenceStep(e: KeyboardEvent, combo: string, allowStart: boolean): boolean {
    const transition = kb.stepSequence(kb.pendingSequence.value, combo)
    switch (transition.kind) {
      case 'run':
        resetSequence()
        if (dispatchIfActive(transition.commandId)) e.preventDefault()
        return true
      case 'extend': {
        // Only a sequence's start is barred from editable fields and overlays;
        // whatever opened either has already cleared a pending one. A barred
        // start falls through: the combo may also be a complete binding in its
        // own right (Zed's prefix rule).
        if (kb.pendingSequence.value === null && !allowStart) return false
        cancelSequenceTimer()
        kb.pendingSequence.value = transition.pending
        e.preventDefault()
        if (transition.deferredCommandId) armSequenceTimer(transition.deferredCommandId)
        return true
      }
      case 'swallow':
        resetSequence()
        e.preventDefault()
        return true
      case 'pass':
        if (kb.pendingSequence.value) resetSequence()
        return false
    }
  }

  // A focused pane owns every key a terminal can use, so the keymap only sees
  // the escape chord (⌘ on macOS, Ctrl+Shift elsewhere), the keys after one
  // that starts a sequence, and `piercesPane` bindings. That is what keeps a
  // bare `g` reaching the shell while ⌘G I can still leave the pane.
  function onPaneKeydown(e: KeyboardEvent): void {
    if (kb.recording.value || anyOverlayOpen.value) {
      resetSequence()
      return
    }

    // Only a sequence the pane started can continue there: one begun with a
    // bare key elsewhere has nothing to do with what is typed into the shell.
    if (kb.pendingSequence.value && !kb.paneSequencePending()) resetSequence()
    if (kb.pendingSequence.value) {
      const step = terminalEscapeCombo(e) ?? comboFromEvent(e)
      if (!step) return
      // xterm declined every key while the sequence was pending, so one that
      // does not continue it is dropped rather than typed.
      if (applySequenceStep(e, step, false)) {
        e.preventDefault()
        return
      }
    }

    // `piercesPane` commands are claimed on the binding alone: an overlay's
    // own combo has to close it, reaching the list is the pane's way out,
    // and window jumps are wanted from inside the window being left. A
    // launcher pierces for the pop-up's reason, but answers to its context
    // so its chord falls through with no terminal attached (ADR
    // quick-terminal-launchers-are-session-scoped).
    const pierced = kb.resolve(comboFromEvent(e) ?? '', applies)
    if (pierced && (commandPiercesPane(pierced) || launcherActionID(pierced) !== null)) {
      if (contextActive(commandById.value.get(pierced)?.context ?? 'global')) {
        resetSequence()
        e.preventDefault()
        runCommand(pierced)
        return
      }
    }

    const escaped = terminalEscapeCombo(e)
    if (!escaped) return
    if (applySequenceStep(e, escaped, true)) return
    const id = kb.resolve(escaped, applies)
    if (id && dispatchIfActive(id)) e.preventDefault()
  }

  function onKeydown(e: KeyboardEvent): void {
    if (isTerminalTarget(e.target)) {
      onPaneKeydown(e)
      return
    }

    // WebKit can treat an unhandled Backspace as browser Back.
    if (e.key === 'Backspace' && !isEditableTarget(e.target)) e.preventDefault()

    if (kb.recording.value) return // the settings editor is capturing this key
    const combo = comboFromEvent(e)
    if (!combo) return

    const allowStart = !isEditableTarget(e.target) && !anyOverlayOpen.value
    if (applySequenceStep(e, combo, allowStart)) return

    const id = kb.resolve(combo, applies)
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
