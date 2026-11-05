import { computed, shallowRef } from 'vue'
import type { SessionViewKind } from '../components/terminal/panelViews'
import { defineStore } from './defineStore'

/** The session area the user is in, which can open its session's views as tabs. */
export interface SessionTabHost {
  /** The session's name, which groups its palette rows. */
  sessionName: () => string
  /** The views the session on screen offers. */
  views: () => readonly SessionViewKind[]
  open: (kind: SessionViewKind) => void
}

export const useSessionTabHost = defineStore('sessionTabHost', () => {
  // Both areas stay mounted, so only the one the user is in holds this.
  const host = shallowRef<SessionTabHost | null>(null)

  function setHost(next: SessionTabHost, active: boolean): void {
    if (active) host.value = next
    else if (host.value === next) host.value = null
  }

  const views = computed(() => host.value?.views() ?? [])
  const sessionName = computed(() => host.value?.sessionName() ?? '')

  function open(kind: SessionViewKind): void {
    if (views.value.includes(kind)) host.value?.open(kind)
  }

  return { views, sessionName, setHost, open }
})
