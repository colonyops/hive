import { ref } from 'vue'
import { Info } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/devtoolsservice'

// Module-scoped: the gate is a property of the running build, so the answer is
// fetched once and shared by the route guard, the command that opens the pane,
// and the pane itself.
//
// A Vite-served frontend is a development build by construction and needs no
// backend answer. Everything else asks, because the pane is reachable in a
// shipped build when development.devtools.enabled is on — which is the point:
// a dev build's memory profile is not the one that ships
// (ADR developer-tools-are-reachable-in-a-shipped-build-behind-a-setting).
const enabled = ref(import.meta.env.DEV)
let resolved: Promise<boolean> | null = null

export function useDevTools() {
  function resolve(): Promise<boolean> {
    if (import.meta.env.DEV) return Promise.resolve(true)
    resolved ??= Info()
      .then((info) => {
        enabled.value = info.enabled
        return info.enabled
      })
      .catch(() => false)
    return resolved
  }

  return { enabled, resolve }
}
