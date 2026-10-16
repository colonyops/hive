import { createLogger } from '../lib/log'
import { connectManaged, disconnectManaged } from '../lib/managedTransport'
import { computed, onScopeDispose, readonly, ref, shallowRef } from 'vue'
import { useIntervalFn, useStorage } from '@vueuse/core'
import { defineStore } from './defineStore'
import { useResource, type Resource } from './useResource'
import { errorText } from '../lib/appError'
import { listRemoteSessions, validateRemoteHost, type RemoteSession } from '../lib/remoteConnection'
import type { TerminalEndpoint } from '../lib/terminalClient'

export interface RemoteProfile {
  id: string
  name: string
  host: string
  connectionFile: string
  color: string
}

export const useRemoteConnection = defineStore('remoteConnection', () => {
  const log = createLogger('remote-connection')
  const profiles = useStorage<RemoteProfile[]>('hive.remote.sshProfiles', [])
  const connection = shallowRef<{ id: string; endpoint: TerminalEndpoint; sessions: Resource<RemoteSession[]> } | null>(
    null,
  )
  const activeProfile = computed(() => profiles.value.find((profile) => profile.id === connection.value?.id) ?? null)
  const color = computed(() => activeProfile.value?.color ?? '#a78bfa')
  const connecting = ref(false)
  const connectError = ref<string | null>(null)
  const streamError = ref<string | null>(null)
  const status = computed(() => {
    if (connecting.value) return 'connecting'
    if (!connection.value) return 'disconnected'
    return streamError.value || connection.value.sessions.error.value ? 'disconnected' : 'connected'
  })
  let generation = 0

  async function connect(id: string): Promise<boolean> {
    const request = ++generation
    connecting.value = true
    connectError.value = null
    let pending: TerminalEndpoint | null = null
    try {
      const profile = profiles.value.find((row) => row.id === id)
      if (!profile) throw new Error('Choose a saved remote connection.')
      const endpoint = await connectManaged(profile.host, profile.connectionFile)
      pending = endpoint
      if (request !== generation) return false
      const sessions = useResource(() => listRemoteSessions(endpoint), {
        initial: [] as RemoteSession[],
        errorFallback: 'Could not connect to remote Hive.',
      })
      await sessions.reload()
      if (request !== generation) return false
      if (sessions.error.value) {
        connectError.value = sessions.error.value
        return false
      }
      if (connection.value) release(connection.value.endpoint)
      streamError.value = null
      connection.value = { id, endpoint, sessions }
      pending = null
      return true
    } catch (err) {
      if (request === generation) connectError.value = errorText(err, 'Could not connect to remote Hive.')
      return false
    } finally {
      if (pending) release(pending)
      if (request === generation) connecting.value = false
    }
  }

  let reconnecting: Promise<boolean> | null = null
  function reconnect(): Promise<boolean> {
    if (reconnecting) return reconnecting
    const id = connection.value?.id
    if (!id) return Promise.resolve(false)
    reconnecting = connect(id).finally(() => {
      reconnecting = null
    })
    return reconnecting
  }

  async function reload(): Promise<void> {
    await connection.value?.sessions.reload()
  }

  function reportStreamFailure(endpoint: TerminalEndpoint): void {
    if (connection.value?.endpoint === endpoint)
      streamError.value =
        'The terminal connection was interrupted. Reconnect the remote server to restore its terminals.'
  }
  useIntervalFn(() => void reload(), 10000)

  function release(endpoint: TerminalEndpoint): void {
    void disconnectManaged(endpoint).catch((error: unknown) => log.warn('Remote disconnect failed', error))
  }

  function disconnect(): void {
    generation++
    if (connection.value) release(connection.value.endpoint)
    connection.value = null
    streamError.value = null
    connecting.value = false
    connectError.value = null
  }

  function save(input: RemoteProfile): string {
    const host = validateRemoteHost(input.host)
    const connectionFile = input.connectionFile.trim()
    if (!connectionFile || ['\0', '\r', '\n'].some((character) => connectionFile.includes(character)))
      throw new Error('Enter the remote connection file path.')
    const name = input.name.trim()
    if (!name) throw new Error('Enter a connection name.')
    if (!/^#[0-9a-f]{6}$/i.test(input.color)) throw new Error('Choose a valid color.')
    const profile = {
      id: input.id || crypto.randomUUID(),
      name,
      host,
      connectionFile,
      color: input.color.toLowerCase(),
    }
    if (connecting.value) disconnect()
    const previous = profiles.value.find((row) => row.id === profile.id)
    if (
      previous &&
      (previous.host !== host || previous.connectionFile !== connectionFile) &&
      connection.value?.id === profile.id
    )
      disconnect()
    profiles.value = [...profiles.value.filter((row) => row.id !== profile.id), profile]
    return profile.id
  }

  function remove(id: string): void {
    if (connection.value?.id === id || connecting.value) disconnect()
    profiles.value = profiles.value.filter((row) => row.id !== id)
  }

  onScopeDispose(disconnect)
  return {
    profiles: readonly(profiles),
    activeProfile,
    status,
    reportStreamFailure,
    save,
    remove,
    color,
    endpoint: computed(() => connection.value?.endpoint ?? null),
    sessions: computed<readonly RemoteSession[]>(() => connection.value?.sessions.data.value ?? []),
    busy: computed(() => connecting.value || !!connection.value?.sessions.loading.value),
    error: computed(() => connectError.value ?? streamError.value ?? connection.value?.sessions.error.value ?? null),
    connect,
    reconnect,
    reload,
    disconnect,
  }
})
