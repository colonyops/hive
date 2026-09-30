import { useStorage } from '@vueuse/core'
import type { Ref } from 'vue'
import {
  AppearanceSettings as GetAppearanceSettings,
  SetFontFamily as PersistFontFamily,
  SetMonoFontFamily as PersistMonoFontFamily,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import { appFontStacks } from '../lib/appFaces'

/** The bundled faces, offered as the empty selection so the shipped default is
 * tracked by role rather than pinned by name into a user's settings.yaml. */
export const BUNDLED_SANS = 'Inter'
export const BUNDLED_MONO = 'JetBrains Mono'

/** The zero-payload platform stacks, persisted as the CSS keyword itself. */
export const SYSTEM_SANS = 'system-ui'
export const SYSTEM_MONO = 'ui-monospace'

// Module singletons like useTheme's currentTheme, shared by the Appearance
// pane and anything else that reads the choice.
//
// The durable record is settings.yaml; these localStorage entries are only a
// first-paint cache, and they carry the same caveats useTheme documents —
// webview storage is partitioned per origin and bundle id, so it is wiped by a
// bundle-id change and by every dev run. Reading it synchronously is what
// avoids a flash of the bundled face on every launch.
const currentSans: Ref<string> = useStorage<string>('hive.font.sans', '')
const currentMono: Ref<string> = useStorage<string>('hive.font.mono', '')

// Incremented by every explicit selection so the startup reconciliation can
// tell whether the value it read is still newer than what the user has since
// picked (the same versioning idea as useTheme).
let version = 0

// Saves are chained rather than fired in parallel so two quick selections can
// not land out of order. The chain absorbs its own failures so it never settles
// rejected.
let persistChain: Promise<void> = Promise.resolve()

function apply(): void {
  const stacks = appFontStacks(currentSans.value, currentMono.value)
  // Inline on the root element, which outranks the :root rule Tailwind's
  // @theme block emits for these two tokens.
  document.documentElement.style.setProperty('--font-sans', stacks.sans)
  document.documentElement.style.setProperty('--font-mono', stacks.mono)
}

function persist(write: () => Promise<void>): void {
  version++
  persistChain = persistChain.then(write).catch((error: unknown) => {
    // The choice is applied and cached regardless; losing the durable write is
    // a degraded-but-working state, not a reason to revert what the user sees.
    console.warn('Unable to persist font settings to settings.yaml', error)
  })
}

// Reconciles the first-paint cache against the durable record, which wins
// outright — including when it is empty. Unlike the theme there is no earlier
// build whose localStorage-only choice has to be adopted, and empty is a real
// selection here (the bundled face), so there is nothing for "nothing
// persisted" to be distinguished from.
async function hydrate(): Promise<void> {
  const started = version
  try {
    const settings = await GetAppearanceSettings()
    // A selection made while this read was in flight is newer than its result.
    if (version !== started) return
    currentSans.value = settings.fontFamily ?? ''
    currentMono.value = settings.monoFontFamily ?? ''
    apply()
  } catch (error) {
    // Keep the cached choice: an unavailable binding must not reset the UI.
    console.warn('Unable to load font settings from settings.yaml', error)
  }
}

export function setAppFontFamily(family: string): void {
  currentSans.value = family
  apply()
  persist(() => PersistFontFamily(family))
}

export function setAppMonoFontFamily(family: string): void {
  currentMono.value = family
  apply()
  persist(() => PersistMonoFontFamily(family))
}

// Called once in main.ts before mount so the first paint uses the cached
// choice. The durable one is reconciled asynchronously right after: it cannot
// be read synchronously, and blocking the first paint on it would cause the
// very flash this ordering avoids.
export function initializeAppFont(): void {
  apply()
  void hydrate()
}

/** The live choices, kept in sync by every setter and by initializeAppFont(). */
export function useAppFont(): { sans: Ref<string>; mono: Ref<string> } {
  return { sans: currentSans, mono: currentMono }
}
