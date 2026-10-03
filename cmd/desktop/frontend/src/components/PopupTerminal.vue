<script setup lang="ts">
import InlineError from './ui/InlineError.vue'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useWindowSize } from '@vueuse/core'
import type { Terminal } from '@xterm/xterm'
import IconTerminal from '~icons/lucide/terminal'
import IconX from '~icons/lucide/x'
import IconPower from '~icons/lucide/power'
import { usePopupTerminal } from '../stores/usePopupTerminal'
import { loadTerminalFont, useXtermPane } from '../composables/useXtermPane'
import { decodeFrame, encodeInputFrames, type PopupTerminalState } from '../lib/popupTerminalClient'
import { installTerminalImages } from '../lib/terminalImages'
import '@xterm/xterm/css/xterm.css'

// The floating pop-up terminal: one PTY this process owns, rendered over
// whatever is on screen (ADR ephemeral-popup-terminals). Hiding it leaves the shell running — the
// panel is a view of the terminal, not the terminal's lifetime — so the only
// things that end it are the process exiting, End, and quitting Hive.

const { visible, checking, available, reason, client, request, launchSeq, hide, ready } = usePopupTerminal()

const MIN_WIDTH = 380
const MIN_HEIGHT = 220

// The share of the window a pop-up opens at, centred. It is a constant until
// there is a setting for it: the size is a preference, and 85% is the one that
// leaves the app readable behind a terminal big enough to work in.
const DEFAULT_SIZE_FRACTION = 0.85

const panel = ref<HTMLElement | null>(null)
const host = ref<HTMLElement | null>(null)
const pane = useXtermPane(host, () => pane.fitToHost())
const { term } = pane
const terminal = ref<PopupTerminalState | null>(null)
const status = ref<'idle' | 'opening' | 'live' | 'ended'>('idle')
const error = ref('')
const endedReason = ref('')

// The launch the pane on screen belongs to. Behind launchSeq means someone has
// asked for a different terminal since, and the pane is showing the wrong one.
let renderedSeq = -1
// Where focus was when the pop-up took it, so dismissing the panel puts the
// caller back where they were rather than on the document body.
let focusReturn: HTMLElement | null = null

const title = computed(() => terminal.value?.title || 'Terminal')
const subtitle = computed(() => terminal.value?.dir ?? '')

// The pane is laid out from the moment a terminal is being opened, not from the
// moment one is live: it has to be measured before the PTY is spawned, and a
// host inside a display:none subtree has no box to measure.
const paneLaidOut = computed(() => status.value === 'opening' || term.value !== null)

// The window's own size, tracked so the panel follows it. The box is derived
// from the viewport and nothing else: it cannot be dragged or resized, so there
// is no remembered geometry to go stale against a window that changed since.
const viewport = useWindowSize()

const panelStyle = computed(() => {
  const width = Math.max(MIN_WIDTH, Math.round(viewport.width.value * DEFAULT_SIZE_FRACTION))
  const height = Math.max(MIN_HEIGHT, Math.round(viewport.height.value * DEFAULT_SIZE_FRACTION))
  return {
    width: `${width}px`,
    height: `${height}px`,
    left: `${Math.max(8, Math.round((viewport.width.value - width) / 2))}px`,
    top: `${Math.max(40, Math.round((viewport.height.value - height) / 2))}px`,
  }
})

async function openTerminal(): Promise<void> {
  if (!client.value || status.value === 'opening') return
  // A terminal that ended leaves its pane mounted so the last screen stays
  // readable; opening the next one is what releases it. The one before it is
  // closed rather than abandoned: a stream that dropped for a reason other than
  // the process exiting would otherwise leave a shell running with no view of
  // it and no way to reach it again.
  const previous = terminal.value?.id
  teardown()
  terminal.value = null
  if (previous) void client.value.close(previous).catch(() => {})

  status.value = 'opening'
  error.value = ''
  renderedSeq = launchSeq.value
  try {
    await loadTerminalFont()
    // Built and measured before the process exists, so the PTY is spawned on
    // the grid it will be drawn on. Sizing it afterwards is what a TUI sees as
    // a full screen at the fallback grid followed by a SIGWINCH reflow — the
    // pop-up appearing small and snapping wider a moment later.
    //
    // The tick is what gives the host its box, and xterm measures its cell on
    // open and never re-measures: a pane built a frame early measures nothing
    // and keeps that cell for the rest of its life.
    await nextTick()
    const created = pane.build()
    if (!created) {
      error.value = 'The terminal could not be rendered.'
      status.value = 'idle'
      return
    }
    const size = pane.measure()
    if (size) created.resize(size.cols, size.rows)

    const opened = await client.value.open({ ...request.value, ...size })
    terminal.value = opened
    attachStream(created, opened)
    status.value = 'live'
    created.focus()
  } catch (failure) {
    teardown()
    status.value = 'idle'
    error.value = failure instanceof Error ? failure.message : 'The terminal could not be opened.'
  }
}

function attachStream(created: Terminal, state: PopupTerminalState): void {
  if (!client.value || !host.value) return

  pane.track(created.onData((data) => pane.send(encodeInputFrames(data))))
  // The server applies whatever size it is told, so the grid xterm measured is
  // the grid the process is given — there is nothing to reconcile afterwards.
  pane.track(
    created.onResize(({ cols, rows }) => {
      void client.value?.resize(state.id, cols, rows).catch(() => {})
    }),
  )

  const opened = client.value.openStream(state.id)
  pane.track({
    dispose: installTerminalImages(host.value, {
      pasteText: (text) => created.paste(text),
      capture: () => ({
        current: () =>
          visible.value &&
          pane.socket() === opened &&
          opened.readyState === WebSocket.OPEN &&
          terminal.value?.id === state.id,
        paste: (text) => created.paste(text),
        focus: () => created.focus(),
      }),
    }),
  })
  opened.onmessage = (event: MessageEvent<ArrayBuffer>) => {
    const frame = decodeFrame(event.data)
    if (!frame) return
    if (frame.type === 'output') created.write(frame.data)
    else exited()
  }
  opened.onerror = () => fail('The terminal connection dropped.')
  opened.onclose = () => {
    if (status.value === 'live') fail('The terminal connection closed.')
  }
  pane.attach(opened)
}

// The shell exited, so the pop-up goes with it. Typing `exit` is how a terminal
// is dismissed, and leaving a dead pane on screen would make the quick way in
// the slow way out. Nothing is closed server-side: a process that exited took
// its terminal with it.
function exited(): void {
  teardown()
  status.value = 'idle'
  terminal.value = null
  hide()
}

// A stream that dropped for any other reason keeps the panel up: the shell may
// still be alive on the far side, and vanishing without saying so would look
// like the terminal simply closed itself.
function fail(why: string): void {
  if (status.value === 'ended') return
  status.value = 'ended'
  endedReason.value = why
  pane.closeStream()
}

// Ends the terminal on purpose, the deliberate counterpart to typing `exit`:
// the process is killed and the panel goes with it.
async function endTerminal(): Promise<void> {
  const id = terminal.value?.id
  exited()
  if (id) await client.value?.close(id).catch(() => {})
}

function teardown(): void {
  pane.teardown()
  endedReason.value = ''
}

// What being shown means: a terminal, focused, in the box the panel opens at.
async function reveal(): Promise<void> {
  rememberFocus()
  // The first show races the availability probe, and a terminal cannot be
  // opened before the transport it opens over is known.
  await ready()
  if (!available.value || !visible.value) return
  await nextTick()
  // Asking for the pop-up is asking for a terminal, so a panel with none live
  // opens one rather than showing an empty box with a button in it. A live one
  // that belongs to an earlier launch is replaced: the caller asked for a
  // different terminal, and one pop-up is open at a time (ADR ephemeral-popup-terminals).
  if (status.value !== 'live' || renderedSeq !== launchSeq.value) {
    if (status.value !== 'opening') await openTerminal()
    return
  }
  // A renderer claim that failed earlier gets another chance every time the
  // pane comes back on screen (ADR terminal-renderer-claimed-on-activation).
  pane.claimRenderer()
  term.value?.focus()
}

// launchSeq is watched beside visible because a launcher invoked while the
// panel is already up changes what was asked for without changing whether the
// panel is on screen, and reveal is what notices.
watch([visible, launchSeq], ([open]) => {
  if (open) void reveal()
  else restoreFocus()
})

// Only what was focused outside the panel is worth returning to; a re-reveal
// while the pane already has focus must not record the pane itself.
function rememberFocus(): void {
  const active = document.activeElement
  if (!(active instanceof HTMLElement) || panel.value?.contains(active)) return
  focusReturn = active
}

function restoreFocus(): void {
  const target = focusReturn
  focusReturn = null
  if (target?.isConnected) target.focus()
}

onMounted(() => {
  // The panel is mounted lazily by the same action that shows it, so it comes up
  // with `visible` already true and the watcher above has nothing to react to.
  if (visible.value) void reveal()
})
onBeforeUnmount(teardown)
</script>

<template>
  <Teleport to="body">
    <!-- v-show, not v-if: hiding the panel must not tear down the shell behind
         it, and remounting xterm would lose the screen either way. -->
    <section
      v-show="visible"
      ref="panel"
      class="fixed z-50 flex flex-col overflow-hidden rounded-xl border border-strong bg-pane shadow-2xl"
      :style="panelStyle"
      role="dialog"
      aria-label="Terminal"
      data-testid="popup-terminal"
    >
      <header
        class="flex h-9 shrink-0 select-none items-center gap-2 border-b border-row bg-raised px-3"
        data-testid="popup-terminal-header"
      >
        <IconTerminal class="size-3.5 shrink-0 text-text-4" />
        <span class="shrink-0 font-mono text-[12px] text-text">{{ title }}</span>
        <span class="truncate font-mono text-[11px] text-text-4">{{ subtitle }}</span>
        <div class="ml-auto flex shrink-0 items-center gap-1">
          <button
            v-if="status === 'live'"
            class="cursor-pointer rounded-[5px] p-1 text-text-4 hover:bg-chip hover:text-severity-error"
            title="End this terminal"
            aria-label="End this terminal"
            data-testid="popup-terminal-end"
            @click="endTerminal"
          >
            <IconPower class="size-3.5" />
          </button>
          <button
            class="cursor-pointer rounded-[5px] p-1 text-text-4 hover:bg-chip hover:text-text"
            title="Hide — the shell keeps running"
            aria-label="Hide the terminal"
            data-testid="popup-terminal-hide"
            @click="hide"
          >
            <IconX class="size-3.5" />
          </button>
        </div>
      </header>

      <div class="relative min-h-0 flex-1 bg-app">
        <!-- data-terminal-input-scope hands every key to the pane, so app
             shortcuts do not steal keys from whatever is running in it. -->
        <div
          v-show="paneLaidOut"
          ref="host"
          class="absolute inset-0 p-1.5"
          data-terminal-input-scope
          data-testid="popup-terminal-pane"
          @click="term?.focus()"
        />

        <div v-if="!term" class="flex h-full flex-col items-center justify-center gap-2 px-6 text-center">
          <template v-if="checking">
            <p class="font-mono text-xs text-text-4">Checking…</p>
          </template>
          <template v-else-if="!available">
            <p class="max-w-[420px] text-xs leading-relaxed text-text-3" data-testid="popup-terminal-unavailable">
              {{ reason }}
            </p>
          </template>
          <template v-else-if="status === 'opening'">
            <p class="font-mono text-xs text-text-4">Opening a shell…</p>
          </template>
          <template v-else>
            <InlineError
              v-if="error"
              testid="popup-terminal-error"
              variant="line"
              class="max-w-[420px] leading-relaxed"
              :message="error"
            />
            <button
              class="cursor-pointer rounded-[7px] bg-chip px-2.5 py-1 font-mono text-[11.5px] text-text hover:bg-strong"
              data-testid="popup-terminal-new"
              @click="openTerminal"
            >
              Open a terminal
            </button>
          </template>
        </div>

        <!-- The pane stays behind this strip so the last screen a command left
             is still readable after its process is gone. -->
        <div
          v-if="status === 'ended'"
          class="absolute inset-x-0 bottom-0 flex items-center gap-3 border-t border-row bg-raised px-3 py-2"
          data-testid="popup-terminal-ended"
        >
          <span class="truncate font-mono text-[11px] text-text-4">{{ endedReason }}</span>
          <button
            class="ml-auto shrink-0 cursor-pointer rounded-[5px] bg-chip px-2 py-0.5 font-mono text-[11.5px] text-text hover:bg-strong"
            data-testid="popup-terminal-new"
            @click="openTerminal"
          >
            New terminal
          </button>
        </div>
      </div>
    </section>
  </Teleport>
</template>
