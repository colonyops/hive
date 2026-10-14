import { computed, readonly, ref, shallowRef, watch } from 'vue'
import { useIntervalFn } from '@vueuse/core'
import { defineStore } from './defineStore'
import { useResource } from './useResource'
import { useRemoteConnection } from './useRemoteConnection'
import { createRemoteInboxClient } from '../lib/remoteInboxClient'
import { createAgentWorkspacesClient, type AgentSession } from '../lib/agentWorkspacesClient'
import { errorText } from '../lib/appError'

export const useRemoteCatalog = defineStore('remoteCatalog', () => {
  const remote = useRemoteConnection()
  const inboxId = ref('')
  const workspace = ref('')
  const codeId = ref('')
  const chat = shallowRef<AgentSession | null>(null)
  const chatBusy = ref(false)
  const chatError = ref<string | null>(null)
  const catalog = shallowRef<ReturnType<typeof createCatalog> | null>(null)
  let generation = 0

  function createCatalog(endpoint: NonNullable<typeof remote.endpoint.value>) {
    const agents = createAgentWorkspacesClient(endpoint)
    const inbox = createRemoteInboxClient(endpoint)
    return {
      agents,
      inbox,
      inboxes: useResource(() => inbox.inboxes(), { initial: [], errorFallback: 'Could not list remote inboxes.' }),
      workspaces: useResource(
        async () => {
          const result = await agents.workspaces()
          if (!result.available) throw new Error(result.error || 'Remote chats are unavailable.')
          if (result.rootProblem) throw new Error(result.rootProblem)
          return result.workspaces
        },
        { initial: [], errorFallback: 'Could not list remote workspaces.' },
      ),
      chats: useResource(() => agents.allSessions(), { initial: [], errorFallback: 'Could not list remote chats.' }),
    }
  }
  async function reload(): Promise<void> {
    const current = catalog.value
    if (current) await Promise.all([current.inboxes.reload(), current.workspaces.reload(), current.chats.reload()])
  }
  function selectInbox(id: string): void {
    inboxId.value = id
  }
  function selectWorkspace(dir: string): void {
    workspace.value = dir
    chat.value = null
    chatError.value = null
    generation++
    chatBusy.value = false
  }
  function selectCode(id: string): void {
    codeId.value = id
  }

  async function openChat(name: string, id?: string): Promise<void> {
    const current = catalog.value
    const dir = workspace.value
    if (!current || !dir || chatBusy.value) return
    const request = ++generation
    chatBusy.value = true
    chatError.value = null
    try {
      await current.agents.openWorkspace(dir)
      if (request !== generation) return
      const result = id
        ? await current.agents.resumeSession({ id })
        : await current.agents.startSession({ workspace: dir, name: name.trim() || 'New chat' })
      if (request !== generation) return
      chat.value = result
      await current.chats.reload()
    } catch (err) {
      if (request === generation) chatError.value = errorText(err, 'Could not open remote chat.')
    } finally {
      if (request === generation) chatBusy.value = false
    }
  }
  watch(
    remote.endpoint,
    (endpoint) => {
      generation++
      inboxId.value = ''
      workspace.value = ''
      codeId.value = ''
      chat.value = null
      chatBusy.value = false
      chatError.value = null
      catalog.value = endpoint ? createCatalog(endpoint) : null
      void reload()
    },
    { immediate: true, flush: 'sync' },
  )
  useIntervalFn(() => void reload(), 10000)
  return {
    inboxId: readonly(inboxId),
    workspace: readonly(workspace),
    codeId: readonly(codeId),
    chat: readonly(chat),
    chatBusy: readonly(chatBusy),
    chatError: readonly(chatError),
    inboxes: computed(() => catalog.value?.inboxes.data.value ?? []),
    workspaces: computed(() => catalog.value?.workspaces.data.value ?? []),
    chats: computed(() => catalog.value?.chats.data.value ?? []),
    inboxError: computed(() => catalog.value?.inboxes.error.value ?? null),
    workspaceError: computed(() => catalog.value?.workspaces.error.value ?? catalog.value?.chats.error.value ?? null),
    inboxClient: computed(() => catalog.value?.inbox ?? null),
    selectInbox,
    selectWorkspace,
    selectCode,
    openChat,
    reload,
  }
})
