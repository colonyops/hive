import { computed, onScopeDispose, readonly, ref, shallowRef } from 'vue'
import { useStorage } from '@vueuse/core'
import { defineStore } from './defineStore'
import { useResource, type Resource } from './useResource'
import { errorText } from '../lib/appError'
import { listRemoteSessions, remoteEndpoint, type RemoteSession } from '../lib/remoteConnection'
import type { TerminalEndpoint } from '../lib/terminalClient'

export interface RemoteProfile {
  id: string
  name: string
  address: string
  color: string
}

export const useRemoteConnection = defineStore('remoteConnection', () => {
  const profiles = useStorage<RemoteProfile[]>('hive.remote.profiles', [])
  const connection = shallowRef<{ id: string; endpoint: TerminalEndpoint; sessions: Resource<RemoteSession[]> } | null>(
    null,
  )
  const activeProfile = computed(() => profiles.value.find((profile) => profile.id === connection.value?.id) ?? null)
  const color = computed(() => activeProfile.value?.color ?? '#a78bfa')
  const connecting = ref(false)
  const connectError = ref<string | null>(null)
  let generation = 0

  async function connect(id: string, token: string): Promise<boolean> {
    const request = ++generation
    connecting.value = true
    connectError.value = null
    try {
      const profile = profiles.value.find((row) => row.id === id)
      if (!profile) throw new Error('Choose a saved remote connection.')
      const endpoint = remoteEndpoint(profile.address, token)
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
      connection.value = { id, endpoint, sessions }
      return true
    } catch (err) {
      if (request === generation) connectError.value = errorText(err, 'Could not connect to remote Hive.')
      return false
    } finally {
      if (request === generation) connecting.value = false
    }
  }

  async function reload(): Promise<void> {
    await connection.value?.sessions.reload()
  }

  function disconnect(): void {
    generation++
    connection.value = null
    connecting.value = false
    connectError.value = null
  }

  function save(input: RemoteProfile): string {
    const address = remoteEndpoint(input.address, 'validate-address').httpBaseURL
    const name = input.name.trim()
    if (!name) throw new Error('Enter a connection name.')
    if (!/^#[0-9a-f]{6}$/i.test(input.color)) throw new Error('Choose a valid color.')
    const profile = { id: input.id || crypto.randomUUID(), name, address, color: input.color.toLowerCase() }
    if (connecting.value) disconnect()
    const previous = profiles.value.find((row) => row.id === profile.id)
    if (previous && previous.address !== address && connection.value?.id === profile.id) disconnect()
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
    save,
    remove,
    color,
    endpoint: computed(() => connection.value?.endpoint ?? null),
    sessions: computed<readonly RemoteSession[]>(() => connection.value?.sessions.data.value ?? []),
    busy: computed(() => connecting.value || !!connection.value?.sessions.loading.value),
    error: computed(() => connectError.value ?? connection.value?.sessions.error.value ?? null),
    connect,
    reload,
    disconnect,
  }
})
