import { computed, readonly, ref, shallowRef, watch } from 'vue'
import type { SessionLaunchOptions } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import type { SessionStatus } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { createRemoteCodeClient, type RemoteCodeInput } from '../lib/remoteCodeClient'
import { createTerminalClient } from '../lib/terminalClient'
import { useRemoteConnection } from './useRemoteConnection'
import { defineStore } from './defineStore'
import { useResource } from './useResource'

export const useRemoteCode = defineStore('remoteCode', () => {
  const remote = useRemoteConnection()
  const selectedWindow = ref('')
  const resources = shallowRef<ReturnType<typeof createResources> | null>(null)
  function createResources(endpoint: NonNullable<typeof remote.endpoint.value>) {
    const api = createRemoteCodeClient(endpoint)
    const client = createTerminalClient(endpoint)
    return {
      api,
      client,
      options: useResource(() => api.options(), {
        initial: null as SessionLaunchOptions | null,
        errorFallback: 'Could not list remote repositories.',
      }),
      tree: useResource(
        async () => {
          const [windows, statuses] = await Promise.all([
            client.listWindows(remote.sessions.value.filter((row) => row.state === 'active').map((row) => row.slug)),
            api.statuses(),
          ])
          return { windows, statuses: statuses.items ?? [] }
        },
        {
          initial: {
            windows: {},
            statuses: [] as SessionStatus[],
          },
          errorFallback: 'Could not read remote terminals.',
        },
      ),
    }
  }
  watch(
    remote.endpoint,
    (endpoint) => {
      selectedWindow.value = ''
      resources.value = endpoint ? createResources(endpoint) : null
      if (resources.value) void resources.value.options.reload()
    },
    { immediate: true },
  )

  async function reload(): Promise<void> {
    await resources.value?.tree.reload()
  }
  async function reloadOptions(): Promise<void> {
    await resources.value?.options.reload()
  }
  function selectWindow(id: string): void {
    selectedWindow.value = id
  }
  async function create(profileID: string, input: RemoteCodeInput): Promise<void> {
    const current = resources.value
    if (!current || remote.activeProfile.value?.id !== profileID)
      throw new Error('Reconnect to the selected remote before creating this session.')
    const { jobId } = await current.api.create(input)
    const deadline = Date.now() + 120000
    while (Date.now() < deadline) {
      const job = await current.api.job(jobId)
      if (job.status === 'failed' || job.status === 'cancelled')
        throw new Error(job.error || 'Remote session creation failed.')
      if (job.status === 'done') {
        if (resources.value === current) {
          await remote.reload()
          await reload()
        }
        return
      }
      await new Promise((resolve) => setTimeout(resolve, 1000))
    }
    throw new Error('The remote is still creating this session. Check the session list before trying again.')
  }
  return {
    options: computed(() => resources.value?.options.data.value ?? null),
    optionsError: computed(() => resources.value?.options.error.value ?? null),
    loadingOptions: computed(() => resources.value?.options.loading.value ?? false),
    windows: computed(() => resources.value?.tree.data.value.windows ?? {}),
    statuses: computed(() => resources.value?.tree.data.value.statuses ?? []),
    error: computed(() => resources.value?.tree.error.value ?? null),
    client: computed(() => resources.value?.client ?? null),
    selectedWindow: readonly(selectedWindow),
    selectWindow,
    reload,
    reloadOptions,
    create,
  }
})
