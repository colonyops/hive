import { readonly, ref } from 'vue'
import { defineStore } from './defineStore'

/**
 * The server the New Session form creates on, '' for this machine, and the
 * servers it offers. The form is one module-scoped instance (useNewSession),
 * so its server choice is shared state too.
 */
export const useNewSessionServer = defineStore('newSessionServer', () => {
  const server = ref('')
  const servers = ref<string[]>([])

  function offer(names: string[], chosen: string): void {
    servers.value = chosen && !names.includes(chosen) ? [...names, chosen] : names
    server.value = chosen
  }

  function choose(name: string): void {
    server.value = name
  }

  return { server: readonly(server), servers: readonly(servers), offer, choose }
})
