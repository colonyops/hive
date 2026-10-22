import { computed, ref } from 'vue'
import type { HiveUiPlatform } from '@hive/ui/platform'
import type { Theme } from '@hive/ui/lib/themes'
import type { Toast } from './lib/toast'

const light: Theme = 'light'
const dark: Theme = 'dark'

/** Follows the OS color scheme and mirrors it onto the document so the CSS tokens switch with it. */
export function createBrowserPlatform(toast: Toast): HiveUiPlatform {
  const query = window.matchMedia('(prefers-color-scheme: dark)')
  const prefersDark = ref(query.matches)
  const apply = (isDark: boolean): void => {
    prefersDark.value = isDark
    document.documentElement.dataset.theme = isDark ? dark : light
  }
  apply(query.matches)
  query.addEventListener('change', (event) => apply(event.matches))

  return {
    clipboard: { writeText: (text) => navigator.clipboard.writeText(text) },
    // The server UI has no log sink; warnings and errors surface as page errors instead.
    log: () => ({ debug: () => {}, warn: () => {}, error: () => {} }),
    theme: computed(() => (prefersDark.value ? dark : light)),
    toast: (message) => toast.show(message),
  }
}
