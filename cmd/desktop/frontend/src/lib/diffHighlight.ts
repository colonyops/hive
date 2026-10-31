import { registerCustomTheme } from '@pierre/diffs'
import { getOrCreateWorkerPoolSingleton } from '@pierre/diffs/worker'
import diffsWorkerUrl from '@pierre/diffs/worker/worker-portable.js?url'
import { codeThemes } from '@hive/ui/lib/codeThemes'
import type { Theme } from '@hive/ui/lib/themes'
import type { HiveUiPlatform } from '@hive/ui/platform'

type Logger = ReturnType<HiveUiPlatform['log']>

// A review scrolls one file at a time, so a few workers keep up. The library
// defaults to eight, and each one holds its own engine and grammars.
const POOL_SIZE = 3

for (const theme of Object.values(codeThemes)) registerCustomTheme(theme.name, theme.load)

/**
 * The worker pool that highlights diffs. It lives as long as the app: its cache
 * makes reopening a review immediate, and starting it costs more than idling.
 *
 * Oniguruma, for the reason lib/highlight.ts gives: the JavaScript regex engine
 * backtracks catastrophically on some grammars. The portable worker script is
 * one file because the pool sends it each grammar and theme; the bundled worker
 * entry would ship a second copy of every grammar.
 */
export function diffWorkerPool(theme: Theme, log: Logger) {
  return getOrCreateWorkerPoolSingleton({
    poolOptions: {
      workerFactory: () => new Worker(diffsWorkerUrl, { type: 'module' }),
      poolSize: POOL_SIZE,
      // The pool falls back to the main thread, which works but janks large diffs.
      onWorkerError: (event) => {
        log.warn('a diff highlight worker failed', {
          message: event instanceof ErrorEvent ? event.message : event.type,
        })
        event.preventDefault()
      },
    },
    highlighterOptions: { theme: codeThemes[theme].name, preferredHighlighter: 'shiki-wasm' },
  })
}

export function diffThemeName(theme: Theme): string {
  return codeThemes[theme].name
}
