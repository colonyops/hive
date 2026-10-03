import { ref, type Ref } from 'vue'
import type { OrderDropTarget } from '../lib/listOrder'

export type DropEdge = 'before' | 'after'

/** Which half of the hovered element the pointer is in. */
export function dropEdge(event: DragEvent): DropEdge {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  return event.clientY < rect.top + rect.height / 2 ? 'before' : 'after'
}

/** The insertion-marker classes for the row `id` under a `{ id, edge }` target. */
export function dropClass(target: OrderDropTarget | null, id: string): string {
  if (target?.id !== id) return ''
  return target.edge === 'before' ? 'drop-before' : 'drop-after'
}

/**
 * Native HTML5 drag-to-reorder. The dragged item lives in a ref because
 * dataTransfer cannot be read during dragover; `mime` still goes on the drag so
 * it carries data (some engines refuse to start an empty one) and is
 * identifiable as this list's. The host decides what a hovered row means by
 * passing a target to `over` (null refuses the drop there), and `onDrop` gets
 * the dragged item and the last target once the drag has been cleared.
 */
export function useDragReorder<T, D = OrderDropTarget>(options: {
  mime: string
  /** The drag's data. Defaults to String(item). */
  payload?: (item: T) => string
  onDrop: (dragged: T, target: D) => void
}): {
  dragging: Ref<T | null>
  target: Ref<D | null>
  start: (event: DragEvent, item: T) => void
  over: (event: DragEvent, target: D | null) => void
  drop: () => void
  end: () => void
} {
  const dragging = ref<T | null>(null) as Ref<T | null>
  const target = ref<D | null>(null) as Ref<D | null>

  function end(): void {
    dragging.value = null
    target.value = null
  }

  return {
    dragging,
    target,
    start(event, item) {
      dragging.value = item
      if (!event.dataTransfer) return
      event.dataTransfer.effectAllowed = 'move'
      event.dataTransfer.setData(options.mime, options.payload?.(item) ?? String(item))
    },
    over(event, next) {
      if (dragging.value === null) return
      if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
      target.value = next
    },
    drop() {
      const dragged = dragging.value
      const at = target.value
      end()
      if (dragged !== null && at !== null) options.onDrop(dragged, at)
    },
    end,
  }
}
