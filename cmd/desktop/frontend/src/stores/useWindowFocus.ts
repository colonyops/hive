import { readonly, ref } from 'vue'
import { Focused } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/windowservice'
import { useWailsEvent } from '../composables/useWailsEvent'
import { defineStore } from './defineStore'

// Native focus events are the source of truth once subscribed; the initial
// RPC only seeds the state before the first event arrives.
export const useWindowFocus = defineStore('windowFocus', () => {
  const focused = ref(true)
  let stateVersion = 0

  useWailsEvent('window:focus', () => {
    stateVersion++
    focused.value = true
  })
  useWailsEvent('window:blur', () => {
    stateVersion++
    focused.value = false
  })

  // Subscribe before reading the native state. If an event arrives while the
  // RPC is pending, its newer state must win over this initial snapshot.
  const seedVersion = stateVersion
  void Focused()
    .then((isFocused) => {
      if (stateVersion === seedVersion) focused.value = isFocused
    })
    .catch((error: unknown) => {
      console.error('load window focus state failed', error)
    })

  return { focused: readonly(focused) }
})
