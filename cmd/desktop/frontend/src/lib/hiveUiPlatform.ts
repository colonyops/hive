import { Clipboard } from '@wailsio/runtime'
import type { HiveUiPlatform } from '@hive/ui/platform'
import { useTheme } from '../composables/useTheme'
import { usePerf } from '../composables/usePerf'
import { useToasts } from '../composables/useToasts'
import { createLogger } from './log'

// The Wails clipboard, not navigator.clipboard: in WKWebView that API silently
// no-ops when the document is not focused.
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
    start: (name, attrs) => usePerf('ui').start(name, attrs as Record<string, unknown>),
    end: (handle, attrs) => (handle as (extra?: Record<string, unknown>) => void)(attrs as Record<string, unknown>),
  },
  theme: useTheme().theme,
  // Only failures toast: the one caller reports a clipboard write that failed.
  toast: (msg) => useToasts().showToast(msg, { severity: 'error' }),
}
