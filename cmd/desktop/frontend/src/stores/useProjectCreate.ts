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
import { useWailsEvent } from '../composables/useWailsEvent'
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

  async function refresh(): Promise<void> {
    try {
      const next = await ProjectCreateProgress()
      if (request.value && next.name === request.value.name) progress.value = next
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
    progress.value = { name: req.name, slug: '', steps: [], done: false, error: '' }
    watching.value = true
    await refresh()
  }

  function close(): void {
    watching.value = false
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
  }
})
