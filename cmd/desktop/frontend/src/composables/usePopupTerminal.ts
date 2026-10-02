import { ref, shallowRef, type Ref, type ShallowRef } from 'vue'
import {
  createPopupTerminalClient,
  getPopupTerminalEndpoint,
  type PopupTerminalClient,
  type PopupTerminalRequest,
} from '../lib/popupTerminalClient'
import { Available } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/popupterminalservice'

// The pop-up terminal's panel state, shared by whatever asks for one — the
// command palette, a keybinding, a launcher — and the panel itself, which is
// mounted once at the app root. Hiding the panel leaves the shell running: the
// terminal is ephemeral in that it dies with the app, not in that it dies with
// a keystroke.

const visible = ref(false)
const checking = ref(true)
const available = ref(false)
const reason = ref('')
const client: ShallowRef<PopupTerminalClient | null> = shallowRef(null)

// What the next terminal opens against. It is captured when the panel is asked
// for rather than when the terminal is opened, because that is the moment the
// caller's context — the session on screen — is known.
const request = ref<PopupTerminalRequest>({})

// Bumped whenever a *different* launch is asked for. One pop-up is open at a
// time (ADR ephemeral-popup-terminals), so asking for lazygit while a shell is up replaces it rather
// than opening beside it — and the panel watches this to know which it is.
const launchSeq = ref(0)

let probe: Promise<void> | null = null

export function usePopupTerminal(): {
  visible: Ref<boolean>
  checking: Ref<boolean>
  available: Ref<boolean>
  reason: Ref<string>
  client: ShallowRef<PopupTerminalClient | null>
  request: Ref<PopupTerminalRequest>
  launchSeq: Ref<number>
  toggle: (next?: PopupTerminalRequest) => void
  show: (next?: PopupTerminalRequest) => void
  hide: () => void
  /** Settles once availability and the transport are known. */
  ready: () => Promise<void>
} {
  return { visible, checking, available, reason, client, request, launchSeq, toggle, show, hide, ready: ensureProbed }
}

function show(next: PopupTerminalRequest = {}): void {
  if (!sameLaunch(next, request.value)) {
    request.value = next
    launchSeq.value++
  }
  visible.value = true
  void ensureProbed()
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

function sameLaunch(a: PopupTerminalRequest, b: PopupTerminalRequest): boolean {
  return a.launcher === b.launcher && a.sessionSlug === b.sessionSlug && a.dir === b.dir && a.command === b.command
}

// The availability answer and the transport are resolved once per run: neither
// can change while the app is running — there is no program to install — so a
// second probe would only re-ask a settled question.
function ensureProbed(): Promise<void> {
  probe ??= (async () => {
    try {
      const availability = await Available()
      available.value = availability.available
      reason.value = availability.reason
      if (availability.available) client.value = createPopupTerminalClient(await getPopupTerminalEndpoint())
    } catch (error) {
      available.value = false
      reason.value = error instanceof Error ? error.message : 'The pop-up terminal is unavailable.'
    } finally {
      checking.value = false
    }
  })()
  return probe
}

export function resetPopupTerminalForTests(): void {
  visible.value = false
  checking.value = true
  available.value = false
  reason.value = ''
  client.value = null
  request.value = {}
  launchSeq.value = 0
  probe = null
}
