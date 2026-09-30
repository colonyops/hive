import { useStorage } from '@vueuse/core'
import type { Ref } from 'vue'
import {
  AppearanceSettings as GetAppearanceSettings,
  SetTheme as PersistThemeSetting,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'

export const themes = [
  'dark',
  'light',
  'midnight',
  'gruvbox',
  'slate',
  'slate-light',
  'one-dark',
  'one-light',
  'tokyo-night',
  'tokyo-night-day',
  'catppuccin-mocha',
  'catppuccin-latte',
  'nord',
  'nord-light',
] as const
export type Theme = (typeof themes)[number]

export const themeLabels: Record<Theme, string> = {
  dark: 'Dark',
  light: 'Light',
  midnight: 'Midnight',
  gruvbox: 'Gruvbox',
  slate: 'Slate',
  'slate-light': 'Slate Light',
  'one-dark': 'One Dark',
  'one-light': 'One Light',
  'tokyo-night': 'Tokyo Night',
  'tokyo-night-day': 'Tokyo Night Day',
  'catppuccin-mocha': 'Catppuccin Mocha',
  'catppuccin-latte': 'Catppuccin Latte',
  nord: 'Nord',
  'nord-light': 'Nord Light',
}

function isTheme(value: string | null): value is Theme {
  return themes.includes(value as Theme)
}

// Read before the useStorage below, which writes its default into the cache
// when the key is absent and so would erase the distinction. A valid value
// here is the only proof of a theme the user actually chose under a previous
// build, which is what the one-time adoption into settings.yaml carries over.
const cachedThemeAtLoad = localStorage.getItem('hive.theme')

// The live theme. A module singleton (like useFlowsSession's shared session) so
// every caller — the command palette's theme:* commands, SettingsView's
// Appearance section — reads/drives the same value.
//
// The durable record is settings.yaml (SettingsService's appearance section);
// this localStorage entry is only a first-paint cache. Webview storage is
// partitioned by origin *and* by bundle id — on macOS that is
// `wails://localhost` (plus the dev server's port, which is a freshly picked
// free port every dev run) under ~/Library/WebKit/<CFBundleIdentifier> — so it
// is wiped by any bundle-id change and by every dev run, and macOS may purge it
// outright. Reading it synchronously is still what avoids a flash of the
// default theme, hence cache here, truth in settings.yaml.
const currentTheme: Ref<Theme> = useStorage<Theme>('hive.theme', 'dark')

// Incremented by every explicit selection so the startup reconciliation can
// tell whether the value it read is still newer than what the user has since
// picked (the same versioning idea as useNotificationSettings).
let themeVersion = 0

// Saves are chained rather than fired in parallel so two quick selections can
// not land out of order and leave settings.yaml disagreeing with the screen.
// The chain absorbs its own failures so it never settles rejected.
let persistChain: Promise<void> = Promise.resolve()

function applyTheme(theme: Theme): void {
  document.documentElement.dataset.theme = theme
  currentTheme.value = theme // cached by useStorage for the next first paint
}

function persistTheme(theme: Theme): void {
  persistChain = persistChain
    .then(async () => {
      await PersistThemeSetting(theme)
    })
    .catch((error: unknown) => {
      // The theme is applied and cached regardless; losing the durable write is
      // a degraded-but-working state, not a reason to revert what the user sees.
      console.warn('Unable to persist theme to settings.yaml', error)
    })
}

// Reconciles the first-paint cache against the durable record. settings.yaml
// wins when it holds a known theme. When it holds nothing — first run after
// this change, or a hand-edited value too garbled to use — a theme the user
// had already chosen is adopted and written through, carrying existing users
// over without re-selecting. Nothing is written when there was no prior
// choice: settings.yaml records decisions, and an absent key already means
// "use the default".
async function hydrateFromSettings(): Promise<void> {
  const version = themeVersion
  try {
    const settings = await GetAppearanceSettings()
    // A selection made while this read was in flight is newer than its result.
    if (themeVersion !== version) return
    if (isTheme(settings.theme)) {
      applyTheme(settings.theme)
      return
    }
    if (isTheme(cachedThemeAtLoad)) persistTheme(cachedThemeAtLoad)
  } catch (error) {
    // Keep the cached theme: an unavailable binding must not reset the UI.
    console.warn('Unable to load theme from settings.yaml', error)
  }
}

export function setTheme(nextTheme: Theme): void {
  themeVersion++
  applyTheme(nextTheme)
  persistTheme(nextTheme)
}

// Called once in main.ts before mount so the first paint uses the cached theme,
// and to heal a garbage cached value back to the default. The durable theme is
// reconciled asynchronously right after: it cannot be read synchronously, and
// blocking the first paint on it would cause the very flash this ordering
// avoids.
export function initializeTheme(): void {
  applyTheme(isTheme(currentTheme.value) ? currentTheme.value : 'dark')
  void hydrateFromSettings()
}

/** The live theme, kept in sync by every setTheme()/initializeTheme() call. */
export function useTheme(): { theme: Ref<Theme> } {
  return { theme: currentTheme }
}
