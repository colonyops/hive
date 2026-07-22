import { readonly, ref, type Ref } from 'vue'
import { Events } from '@wailsio/runtime'
import { Focused } from '../../bindings/github.com/colonyops/hive/desktop/windowservice'

// useWindowFocus is an app-lifetime singleton. Native focus events are the
// source of truth once subscribed; the initial RPC only seeds the state before
// the first event arrives.
const focused = ref(true)
const readonlyFocused = readonly(focused)

let started = false
let stateVersion = 0

function start(): void {
  if (started) return
  started = true

  Events.On('window:focus', () => {
    stateVersion++
    focused.value = true
  })
  Events.On('window:blur', () => {
    stateVersion++
    focused.value = false
  })

  // Subscribe before reading the native state. If an event arrives while the
  // RPC is pending, its newer state must win over this initial snapshot.
  const seedVersion = stateVersion
  void Focused().then((isFocused) => {
    if (stateVersion === seedVersion) focused.value = isFocused
  }).catch((error: unknown) => {
    console.error('load window focus state failed', error)
  })
}

export function useWindowFocus(): { focused: Readonly<Ref<boolean>> } {
  start()
  return { focused: readonlyFocused }
}
