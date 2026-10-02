import { Events } from '@wailsio/runtime'
import { onScopeDispose } from 'vue'

/**
 * Subscribes to a Wails event for the lifetime of the calling effect scope.
 */
export function useWailsEvent(name: string, handler: Events.WailsEventCallback): void {
  const unsubscribe: unknown = Events.On(name, handler)
  if (typeof unsubscribe !== 'function') {
    throw new TypeError(`Events.On("${name}") returned no unsubscribe function; a test mock must return one`)
  }
  onScopeDispose(unsubscribe as () => void)
}
