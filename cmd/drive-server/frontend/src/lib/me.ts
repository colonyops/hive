import { inject, type InjectionKey, type Ref } from 'vue'
import type { Me } from '../types'

export interface MeContext {
  me: Ref<Me | null>
  refresh(): Promise<void>
}

export const meKey: InjectionKey<MeContext> = Symbol('me')

/** Pages that change storage call `refresh` so the layout's meter follows. Absent outside the layout. */
export function useMe(): MeContext | null {
  return inject(meKey, null)
}
