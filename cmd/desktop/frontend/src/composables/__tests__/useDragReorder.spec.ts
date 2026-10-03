import { describe, expect, it, vi } from 'vitest'
import { dropClass, dropEdge, useDragReorder } from '../useDragReorder'

function dragEvent(clientY = 0): DragEvent {
  const event = new Event('dragover') as DragEvent
  const store: Record<string, string> = {}
  Object.defineProperty(event, 'clientY', { value: clientY })
  Object.defineProperty(event, 'dataTransfer', {
    value: { effectAllowed: '', dropEffect: '', setData: (type: string, data: string) => (store[type] = data), store },
  })
  Object.defineProperty(event, 'currentTarget', {
    value: { getBoundingClientRect: () => ({ top: 0, height: 20 }) },
  })
  return event
}

describe('useDragReorder', () => {
  it('carries the payload under its mime type and hands the drop to the host', () => {
    const onDrop = vi.fn()
    const drag = useDragReorder<string>({ mime: 'application/x-test', onDrop })
    const start = dragEvent()
    drag.start(start, 'a')
    expect((start.dataTransfer as unknown as { store: Record<string, string> }).store).toEqual({
      'application/x-test': 'a',
    })

    drag.over(dragEvent(15), { id: 'b', edge: dropEdge(dragEvent(15)) })
    expect(drag.target.value).toEqual({ id: 'b', edge: 'after' })
    drag.drop()
    expect(onDrop).toHaveBeenCalledWith('a', { id: 'b', edge: 'after' })
    expect(drag.dragging.value).toBeNull()
    expect(drag.target.value).toBeNull()
  })

  it('ignores a hover with no drag of its own, and a null target refuses the drop', () => {
    const onDrop = vi.fn()
    const drag = useDragReorder<string>({ mime: 'application/x-test', onDrop })
    drag.over(dragEvent(), { id: 'b', edge: 'before' })
    expect(drag.target.value).toBeNull()

    drag.start(dragEvent(), 'a')
    drag.over(dragEvent(), null)
    drag.drop()
    expect(onDrop).not.toHaveBeenCalled()
  })

  it('maps a target to the marker class of its row only', () => {
    expect(dropEdge(dragEvent(5))).toBe('before')
    expect(dropClass({ id: 'a', edge: 'before' }, 'a')).toBe('drop-before')
    expect(dropClass({ id: 'a', edge: 'after' }, 'a')).toBe('drop-after')
    expect(dropClass({ id: 'a', edge: 'after' }, 'b')).toBe('')
    expect(dropClass(null, 'a')).toBe('')
  })
})
