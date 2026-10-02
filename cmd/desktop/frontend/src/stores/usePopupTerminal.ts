import { computed, readonly, ref } from 'vue'
import {
  createPopupTerminalClient,
  getPopupTerminalEndpoint,
  type PopupTerminalClient,
  type PopupTerminalRequest,
} from '../lib/popupTerminalClient'
import { Available } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/popupterminalservice'
import { defineStore } from './defineStore'
import { useResource } from './useResource'

interface PopupTerminalAvailability {
  available: boolean
  reason: string
  client: PopupTerminalClient | null
}

function sameLaunch(a: PopupTerminalRequest, b: PopupTerminalRequest): boolean {
  return a.launcher === b.launcher && a.sessionSlug === b.sessionSlug && a.dir === b.dir && a.command === b.command
}

// The pop-up terminal's panel state, shared by whatever asks for one — the
// command palette, a keybinding, a launcher — and the panel itself, which is
// mounted once at the app root. Hiding the panel leaves the shell running: the
// terminal is ephemeral in that it dies with the app, not in that it dies with
// a keystroke.
export const usePopupTerminal = defineStore('popupTerminal', () => {
  const visible = ref(false)
  // What the next terminal opens against. It is captured when the panel is asked
  // for rather than when the terminal is opened, because that is the moment the
  // caller's context — the session on screen — is known.
  const request = ref<PopupTerminalRequest>({})
  // Bumped whenever a *different* launch is asked for. One pop-up is open at a
  // time (ADR ephemeral-popup-terminals), so asking for lazygit while a shell is up replaces it rather
  // than opening beside it — and the panel watches this to know which it is.
  const launchSeq = ref(0)

  // The availability answer and the transport are resolved once per run: neither
  // can change while the app is running — there is no program to install — so a
  // second probe would only re-ask a settled question.
  const probe = useResource<PopupTerminalAvailability>(
    async () => {
      const availability = await Available()
      const client = availability.available ? createPopupTerminalClient(await getPopupTerminalEndpoint()) : null
      return { available: availability.available, reason: availability.reason, client }
    },
    { initial: { available: false, reason: '', client: null }, errorFallback: 'The pop-up terminal is unavailable.' },
  )
  let probing: Promise<void> | null = null

  /** Settles once availability and the transport are known. */
  function ready(): Promise<void> {
    return (probing ??= probe.reload())
  }

  function show(next: PopupTerminalRequest = {}): void {
    if (!sameLaunch(next, request.value)) {
      request.value = next
      launchSeq.value++
    }
    visible.value = true
    void ready()
  }

  function hide(): void {
    visible.value = false
  }

  // The combo that opens a launcher has to close it, so invoking what is already
  // on screen dismisses it. Invoking anything else — another launcher, or a plain
  // shell for a different session — is a request for a terminal the panel is not
  // showing, and answering that by returning to the old one would ignore what was
  // asked for.
  function toggle(next: PopupTerminalRequest = {}): void {
    if (visible.value && sameLaunch(next, request.value)) hide()
    else show(next)
  }

  return {
    visible: readonly(visible),
    request: readonly(request),
    launchSeq: readonly(launchSeq),
    checking: computed(() => !probe.loaded.value),
    available: computed(() => probe.data.value.available),
    reason: computed(() => probe.error.value ?? probe.data.value.reason),
    client: computed(() => probe.data.value.client),
    toggle,
    show,
    hide,
    ready,
  }
})
