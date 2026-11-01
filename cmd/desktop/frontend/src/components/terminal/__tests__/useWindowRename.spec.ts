import { describe, expect, it, vi } from 'vitest'
import { shallowRef } from 'vue'
import type { UseTerminalWindows } from '../../../composables/useTerminalWindows'
import type { TreeWindowRow } from '../useTerminalTree'
import { useWindowRename } from '../useWindowRename'

const live: TreeWindowRow = { windowId: '@1', name: 'agent', active: true, live: true, indicator: null }

function setup() {
  const rename = vi.fn()
  const session = shallowRef<UseTerminalWindows | null>({ rename } as unknown as UseTerminalWindows)
  return { rename, ...useWindowRename(session) }
}

describe('useWindowRename', () => {
  it('renames a live window to the trimmed draft', () => {
    const { rename, start, commit, draft, windowId } = setup()
    start(live)
    expect(draft.value).toBe('agent')
    draft.value = '  shell '
    commit()
    expect(rename).toHaveBeenCalledWith('@1', 'shell')
    expect(windowId.value).toBe('')
  })

  it('ignores a listed window, which has no client to rename through', () => {
    const { start, windowId } = setup()
    start({ ...live, live: false })
    expect(windowId.value).toBe('')
  })

  it('keeps the draft when a double-click lands on a rename already under way', () => {
    const { start, draft } = setup()
    start(live)
    draft.value = 'typed'
    start(live)
    expect(draft.value).toBe('typed')
  })

  it('drops an empty name', () => {
    const { rename, start, commit, draft } = setup()
    start(live)
    draft.value = '  '
    commit()
    expect(rename).not.toHaveBeenCalled()
  })
})
