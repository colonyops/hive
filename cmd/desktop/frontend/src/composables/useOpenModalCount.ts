import { onScopeDispose, ref, type Ref } from 'vue'

// Module-scope so every BaseModal instance shares one registry: a
// ConfirmationDialog stacked on top of another overlay (TasksOverlay, a
// drawer) registers here too. That is what lets the overlay gate its own
// Escape handler on "is anything stacked on top of me" rather than racing a
// stacked dialog's own useEscapeToClose for the same keypress — see
// TasksView's use of useOpenModalCount().
const openIds = new Set<symbol>()
const count = ref(0)

/**
 * Registers a BaseModal as open for as long as its component stays mounted.
 * Only BaseModal-backed dialogs may register: an overlay that gates its own
 * dismissal on useOpenModalCount() === 0 (TasksOverlay) relies on the count
 * staying zero while it is the topmost surface.
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

/** Reactive count of currently-mounted BaseModal instances, shared app-wide. */
export function useOpenModalCount(): Readonly<Ref<number>> {
  return count
}
