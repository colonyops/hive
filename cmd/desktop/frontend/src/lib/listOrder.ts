// Pure move-one-item-in-a-flat-list logic, kept free of Vue and Wails so the
// drag-and-drop that uses it can be unit-tested in isolation.

// OrderDropTarget is the item a drag is hovering and which of its edges the
// pointer is nearer, which is where the dragged item lands.
export type OrderDropTarget = { id: string; edge: 'before' | 'after' }

// moveId returns ids with dragId moved to target, or null when the move is
// unresolvable or would leave the order unchanged — so a drop that lands where
// the item already sat persists nothing.
export function moveId(ids: string[], dragId: string, target: OrderDropTarget): string[] | null {
  const from = ids.indexOf(dragId)
  if (from === -1) return null
  const without = ids.filter((id) => id !== dragId)
  const anchor = without.indexOf(target.id)
  if (anchor === -1) return null
  const at = target.edge === 'before' ? anchor : anchor + 1
  if (at === from) return null
  return [...without.slice(0, at), dragId, ...without.slice(at)]
}
