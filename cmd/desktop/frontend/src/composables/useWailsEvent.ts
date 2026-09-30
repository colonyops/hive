import { Events } from '@wailsio/runtime'
import { onScopeDispose } from 'vue'

/**
 * Subscribes to a Wails event for the lifetime of the calling effect scope.
 */
export function useWailsEvent(name: string, handler: Events.WailsEventCallback): void {
  const unsubscribe = Events.On(name, handler)
  onScopeDispose(unsubscribe)
}
