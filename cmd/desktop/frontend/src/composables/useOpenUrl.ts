import { Browser } from '@wailsio/runtime'
import { createLogger } from '../lib/log'
import { useToasts } from './useToasts'

const log = createLogger('open-url')

/** Opens a URL in the user's browser, and toasts when the OS refuses. */
export function useOpenUrl(): (url: string) => Promise<void> {
  const { showToast } = useToasts()
  return async (url) => {
    if (!url) return
    try {
      await Browser.OpenURL(url)
    } catch (error) {
      log.warn('unable to open URL', error)
      showToast('Could not open the link', { severity: 'error' })
    }
  }
}
