import { Clipboard } from '@wailsio/runtime'
import type { HiveUiPlatform } from '@hive/ui/platform'
import { usePerf } from '../composables/usePerf'
import { createLogger } from './log'

// The native Wails clipboard (Clipboard.SetText), NOT navigator.clipboard /
// vueuse's useClipboard, which are built on it: in the WKWebView that browser
// API silently no-ops when the document is not focused, so copy worked only on
// every other click. Routing through the Wails runtime writes to the OS
// clipboard regardless of webview focus.
export const hiveUiPlatform: HiveUiPlatform = {
  clipboard: { writeText: (text) => Clipboard.SetText(text) },
  log(name) {
    const logger = createLogger(name)
    return {
      debug: (msg, fields) => logger.debug(msg, undefined, fields as Record<string, unknown>),
      warn: (msg, fields) => logger.warn(msg, undefined, fields as Record<string, unknown>),
      error: (msg, fields) => logger.error(msg, undefined, fields as Record<string, unknown>),
    }
  },
  perf: {
    start: (name) => usePerf('ui').start(name),
    end: (handle) => (handle as () => void)(),
  },
}
