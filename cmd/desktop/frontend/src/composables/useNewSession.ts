import { ref } from 'vue'
import {
  CreateSession,
  DismissFailedSession,
  FailedSessionDraft,
  SessionDraftFromActivity,
  SessionLaunchOptions,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import { NewSessionDraft } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/pipelineservice'
import type {
  SessionCreateFailure,
  SessionDraft,
  SessionLaunchOptions as SessionLaunchOptionsView,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import type { InboxItem } from '../types/feed'
import { useToasts } from './useToasts'

interface Draft {
  repository: string
  workspace: string
  name: string
  prompt: string
  agent: string
}
export type SessionTarget = 'repository' | 'workspace'

// An activity row's metadata as the bindings give it. Forwarded, never read.
type ActivityMetadata = { [_ in string]?: string } | null

// Module-scoped so the global command, the per-item menu, and the mounted-once
// dialog all drive the same form (mirrors useReportDialog).
const open = ref(false)
const loading = ref(false)
const busy = ref(false)
const error = ref<string | null>(null)
const options = ref<SessionLaunchOptionsView | null>(null)
const initial = ref<Draft>({ repository: '', workspace: '', name: '', prompt: '', agent: '' })
const initialTarget = ref<SessionTarget>('repository')

// The render copy of the failure shown against the form. The backend holds the
// authority, and this is re-read rather than remembered so a reload keeps it.
const failure = ref<SessionCreateFailure | null>(null)

// The dialog's fields are refs seeded from props at setup, so reopening a
// restored draft over an open form needs a remount to reach the inputs.
const formKey = ref(0)

// The items the open form was drafted from. They are not form fields: the
// backend resolves their identities and the user cannot retarget a draft.
const itemIDs = ref<number[]>([])

// SessionLaunchOptions scans every configured workspace directory (a git call
// per repo), slow enough that a click blocked on it visibly lags. The last
// result opens the dialog instantly; the refresh lands behind it.
let cachedOptions: SessionLaunchOptionsView | null = null

async function fetchOptions(): Promise<SessionLaunchOptionsView> {
  const opts = await SessionLaunchOptions()
  cachedOptions = opts
  return opts
}

function resolveOptions(): Promise<SessionLaunchOptionsView> {
  if (!cachedOptions) return fetchOptions()
  void fetchOptions()
    .then((opts) => {
      if (open.value) options.value = opts
    })
    .catch(() => {})
  return Promise.resolve(cachedOptions)
}

// A read failure and an empty answer are the same thing to a caller.
async function pendingDraft(): Promise<SessionDraft | null> {
  try {
    const draft = await FailedSessionDraft()
    return draft?.failure ? draft : null
  } catch {
    return null
  }
}

export function resetNewSessionForTests(): void {
  cachedOptions = null
  open.value = false
  loading.value = false
  busy.value = false
  error.value = null
  options.value = null
  initialTarget.value = 'repository'
  failure.value = null
  formKey.value = 0
  itemIDs.value = []
}

function message(e: unknown, fallback: string): string {
  return e instanceof Error && e.message ? e.message : fallback
}

function failureSummary(detail: SessionCreateFailure): string {
  return detail.step ? `${detail.step} ${detail.reason}` : detail.reason
}

function draftItemIDs(draft: SessionDraft): number[] {
  if (draft.itemIds?.length) return draft.itemIds
  const legacyID = (draft as SessionDraft & { itemId?: number }).itemId ?? 0
  return legacyID > 0 ? [legacyID] : []
}

export function useNewSession() {
  const { showToast } = useToasts()

  function prefetch(): void {
    if (!cachedOptions) void fetchOptions().catch(() => {})
  }

  function show(
    draft: Draft,
    opts: SessionLaunchOptionsView,
    items: number[],
    detail: SessionCreateFailure | null,
    target: SessionTarget = draft.workspace ? 'workspace' : 'repository',
  ): void {
    options.value = opts
    initial.value = draft
    initialTarget.value = target
    itemIDs.value = [...items]
    failure.value = detail
    formKey.value += 1
    open.value = true
  }

  function restored(draft: SessionDraft, opts: SessionLaunchOptionsView): Draft {
    return {
      repository: draft.workspace ? '' : draft.repository || opts.defaultRepository || '',
      workspace: draft.workspace ?? '',
      name: draft.name,
      prompt: draft.prompt,
      // Empty included: "" is the form's own "Default agent", not an absent
      // value to fill in.
      agent: draft.agent ?? '',
    }
  }

  // `preferred` is the repository of whatever session is on screen. Starting a
  // second session on the repo you are already in is the common case, and the
  // backend default — the first configured workspace — is almost never it.
  //
  // A failed attempt outranks both: it is the only copy of what was typed.
  async function openBlank(preferred = '', target: SessionTarget = 'repository'): Promise<void> {
    if (open.value || loading.value) return
    error.value = null
    loading.value = true
    try {
      const [opts, pending] = await Promise.all([resolveOptions(), pendingDraft()])
      if (pending?.failure) {
        show(restored(pending, opts), opts, draftItemIDs(pending), pending.failure)
        return
      }
      show(
        {
          repository: preferred || opts.defaultRepository || '',
          workspace: '',
          name: '',
          prompt: '',
          agent: opts.defaultAgent,
        },
        opts,
        [],
        null,
        target,
      )
    } catch (e) {
      showToast(message(e, 'Could not load session options.'), { severity: 'error' })
    } finally {
      loading.value = false
    }
  }

  async function openFromItems(items: InboxItem[], target: SessionTarget = 'repository'): Promise<void> {
    if (open.value || loading.value || items.length === 0) return
    error.value = null
    loading.value = true
    const ids = items.map((item) => item.id)
    try {
      const [opts, draft, pending] = await Promise.all([resolveOptions(), NewSessionDraft(ids), pendingDraft()])
      const pendingItemIDs = pending ? draftItemIDs(pending) : []
      const sameItems = pendingItemIDs.length === ids.length && pendingItemIDs.every((id, index) => id === ids[index])
      if (pending?.failure && sameItems) {
        show(restored(pending, opts), opts, ids, pending.failure)
        return
      }
      const draftOptions = items.length > 1 && !draft.repository ? { ...opts, defaultRepository: '' } : opts
      show(restored({ ...draft, agent: opts.defaultAgent }, draftOptions), draftOptions, ids, null, target)
    } catch (e) {
      showToast(message(e, 'Could not prepare the session.'), { severity: 'error' })
    } finally {
      loading.value = false
    }
  }

  async function openFromItem(item: InboxItem, target: SessionTarget = 'repository'): Promise<void> {
    await openFromItems([item], target)
  }

  // Unlike openBlank this replaces a form that is already open: the user asked
  // for this one by name, from the toast.
  async function openFailure(): Promise<void> {
    if (busy.value || loading.value) return
    error.value = null
    loading.value = true
    try {
      const [opts, pending] = await Promise.all([resolveOptions(), pendingDraft()])
      if (!pending?.failure) return
      show(restored(pending, opts), opts, draftItemIDs(pending), pending.failure)
    } catch (e) {
      showToast(message(e, 'Could not reopen the session form.'), { severity: 'error' })
    } finally {
      loading.value = false
    }
  }

  // The path that still works tomorrow: the toast is gone and the slot died
  // with the process, but the row and the draft on it are persisted.
  async function openFromActivity(metadata: ActivityMetadata): Promise<void> {
    if (busy.value || loading.value) return
    error.value = null
    loading.value = true
    try {
      const [opts, draft] = await Promise.all([resolveOptions(), SessionDraftFromActivity(metadata)])
      show(restored(draft, opts), opts, draftItemIDs(draft), draft.failure ?? null)
    } catch (e) {
      showToast(message(e, 'Could not reopen the session form.'), { severity: 'error' })
    } finally {
      loading.value = false
    }
  }

  async function dismissFailure(): Promise<void> {
    failure.value = null
    try {
      await DismissFailedSession()
    } catch {
      // The pending attempt is a convenience, so failing to drop it is not
      // worth a second error on top of the one the user just dismissed.
    }
  }

  // The dialog is long closed by the time this fires, so a toast that does not
  // time out is the surface that has to reach the user.
  //
  // It deliberately leaves `failure` alone: that accompanies a restored draft,
  // and a form since opened on something else must not show this one.
  async function onCreateFailed(): Promise<void> {
    const pending = await pendingDraft()
    if (!pending?.failure) return
    showToast(`Could not create session ${pending.name || ''}`.trim(), {
      severity: 'error',
      body: failureSummary(pending.failure),
      duration: 0,
      actions: [
        {
          label: 'Retry',
          onClick: () => {
            void openFailure()
          },
        },
        {
          label: 'Dismiss',
          onClick: () => {
            void dismissFailure()
          },
        },
      ],
    })
  }

  function cancel(): void {
    if (busy.value) return
    open.value = false
    options.value = null
    error.value = null
    itemIDs.value = []
  }

  async function submit(input: {
    repository?: string
    workspace?: string
    name: string
    prompt: string
    agent?: string
  }): Promise<void> {
    if (busy.value) return
    busy.value = true
    error.value = null
    try {
      // Creation (including any clone) runs as a background job. A failure
      // arrives later through sessions:create-failed, which is what hands the
      // form back; only validation errors reject here.
      await CreateSession({
        repository: input.repository ?? '',
        workspace: input.workspace ?? '',
        name: input.name,
        prompt: input.prompt,
        agent: input.agent ?? '',
        itemIds: [...itemIDs.value],
      })
      showToast(`Creating session ${input.name}…`, { severity: 'info' })
      open.value = false
      options.value = null
      failure.value = null
      itemIDs.value = []
    } catch (e) {
      error.value = message(e, 'Could not start the session.')
    } finally {
      busy.value = false
    }
  }

  return {
    open,
    options,
    initial,
    initialTarget,
    busy,
    loading,
    error,
    failure,
    formKey,
    prefetch,
    openBlank,
    openFromItem,
    openFromItems,
    openFailure,
    openFromActivity,
    dismissFailure,
    onCreateFailed,
    cancel,
    submit,
  }
}
