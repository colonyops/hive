import { computed, readonly, ref } from 'vue'
import {
  CreateProject,
  ProjectCreateProgress,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type {
  CreateProjectRequest,
  ProjectRepoRequest,
  ProjectCreateProgress as Progress,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { useToasts } from '../composables/useToasts'
import { useWailsEvent } from '../composables/useWailsEvent'
import { router } from '../router'
import { errorText } from '../lib/appError'
import { defineStore } from './defineStore'

/**
 * The project create in flight: the request, and the progress per repository
 * the core reports. The progress dialog is open while `watching` is set; the
 * create itself runs as a job whether anyone watches it or not.
 */
export const useProjectCreate = defineStore('projectCreate', () => {
  const progress = ref<Progress | null>(null)
  const watching = ref(false)
  const request = ref<CreateProjectRequest | null>(null)
  const error = ref<string | null>(null)
  // The repositories a retried New project form starts with.
  const retryDraft = ref<{ definition: string; repos: ProjectRepoRequest[] } | null>(null)

  const { showToast } = useToasts()

  async function refresh(): Promise<void> {
    try {
      const next = await ProjectCreateProgress()
      if (!request.value || next.name !== request.value.name) return
      const finished = next.done && !progress.value?.done
      progress.value = next
      if (finished && !watching.value) announce(next)
    } catch (e) {
      error.value = errorText(e, 'Could not read the progress of the project.')
    }
  }

  useWailsEvent('projects:progress', () => void refresh())

  /** Starts the create. A refusal (a bad name, no repositories) is thrown to the form. */
  async function start(req: CreateProjectRequest): Promise<void> {
    await CreateProject(req)
    request.value = req
    error.value = null
    progress.value = { name: req.name, server: req.server, slug: '', steps: [], done: false, error: '' }
    watching.value = true
    await refresh()
  }

  // With the dialog in the background, a toast is what reports the end.
  function announce(done: Progress): void {
    if (done.error) {
      showToast(`Could not create project ${done.name}`, {
        severity: 'error',
        body: done.error,
        duration: 0,
        actions: [{ label: 'Show', onClick: show }],
      })
      return
    }
    showToast(`Project ${done.name} is ready`, {
      severity: 'success',
      actions: done.slug
        ? [{ label: 'Open', onClick: () => void router.push({ name: 'terminal', params: { slug: done.slug } }) }]
        : [],
    })
  }

  function close(): void {
    watching.value = false
  }

  function show(): void {
    if (progress.value) watching.value = true
  }

  function retry(draft: { definition: string; repos: ProjectRepoRequest[] }): void {
    retryDraft.value = draft
  }

  function clearRetry(): void {
    retryDraft.value = null
  }

  const succeeded = computed(() => !!progress.value?.done && !progress.value.error)
  const failed = computed(() => !!progress.value?.done && !!progress.value.error)

  return {
    progress: readonly(progress),
    request: readonly(request),
    watching: readonly(watching),
    error: readonly(error),
    retryDraft: readonly(retryDraft),
    retry,
    clearRetry,
    succeeded,
    failed,
    start,
    close,
    show,
  }
})
