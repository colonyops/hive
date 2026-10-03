import { onScopeDispose, ref, type Ref } from 'vue'

// Module-scope so every BaseModal and DrawerSheet shares one registry. App.vue
// gates global keybindings on it, and TasksView gates its own keys on it.
const openIds = new Set<symbol>()
const count = ref(0)

/**
 * Registers a modal surface as open for as long as its component stays
 * mounted. Only BaseModal and DrawerSheet register: HubOverlay gates its keys
 * on useOpenModalCount() === 0, so the count must stay zero while it is the
 * topmost surface.
 */
export function useRegisterOpenModal(): void {
  const id = Symbol()
  openIds.add(id)
  count.value = openIds.size
  onScopeDispose(() => {
    openIds.delete(id)
    count.value = openIds.size
  })
}

/** Reactive count of mounted BaseModal and DrawerSheet instances, shared app-wide. */
export function useOpenModalCount(): Readonly<Ref<number>> {
  return count
}
