import { ref, type Ref } from 'vue'
import { Fonts as GetFonts } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'

// Every family the OS scan found, and the fixed-pitch subset. One module
// singleton so the app font pickers and the terminal's share a single scan —
// the Go side caches it for the process, but two composables holding their own
// copy is two round trips and two places for the list to be stale.
const all: Ref<string[]> = ref([])
const monospace: Ref<string[]> = ref([])

// Scanning every font file on the machine takes tens of milliseconds, so this
// runs once, lazily, when a picker first needs it — never on the path a
// terminal opens through.
let requested = false

export function loadInstalledFonts(): void {
  if (requested) return
  requested = true
  void GetFonts()
    .then((families) => {
      all.value = families.all ?? []
      monospace.value = families.monospace ?? []
    })
    .catch((error: unknown) => {
      // A failed scan leaves the pickers offering the bundled faces alone,
      // which is still a working app.
      console.warn('Unable to list installed fonts', error)
    })
}

export function useInstalledFonts(): { all: Ref<string[]>; monospace: Ref<string[]> } {
  return { all, monospace }
}

export function resetInstalledFontsForTests(): void {
  all.value = []
  monospace.value = []
  requested = false
}
