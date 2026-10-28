import { flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ALL_SESSIONS_VIEW_ID, RUNNING_VIEW_ID } from '../../lib/sessionViews'
import { useCodeViews } from '../useCodeViews'

const mocks = vi.hoisted(() => ({
  CodeViews: vi.fn(),
  SetCodeViews: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  CodeViews: mocks.CodeViews,
  SetCodeViews: mocks.SetCodeViews,
}))

beforeEach(() => {
  mocks.CodeViews.mockResolvedValue([])
  mocks.SetCodeViews.mockResolvedValue(undefined)
})

afterEach(() => {
  localStorage.clear()
  vi.clearAllMocks()
})

describe('useCodeViews', () => {
  it('hydrates views from settings and keeps the restored view once they land', async () => {
    localStorage.setItem('hive.code.active-view', 'bugs')
    mocks.CodeViews.mockResolvedValue([
      {
        id: 'bugs',
        name: 'Bugs',
        filter: { match: 'all', conditions: [{ field: 'tag', op: 'any', values: ['bug'] }] },
      },
    ])
    const views = useCodeViews()
    await flushPromises()
    expect(views.activeViewId.value).toBe('bugs')
    expect(views.draft.value.conditions).toEqual([{ field: 'tag', op: 'any', values: ['bug'] }])
    expect(views.dirty.value).toBe(false)
  })

  // Go omits an empty values list and a zero day count, and settings.yaml can be hand-edited.
  it('fills in condition keys the wire left out', async () => {
    localStorage.setItem('hive.code.active-view', 'v')
    mocks.CodeViews.mockResolvedValue([
      {
        id: 'v',
        name: 'V',
        filter: {
          match: 'all',
          conditions: [
            { field: 'tag', op: 'any' },
            { field: 'repo', op: 'none', values: null },
            { field: 'age', op: 'older' },
          ],
        },
      },
    ])
    const views = useCodeViews()
    await flushPromises()
    expect(views.draft.value.conditions).toEqual([
      { field: 'tag', op: 'any', values: [] },
      { field: 'repo', op: 'none', values: [] },
      { field: 'age', op: 'older', days: 0 },
    ])
  })

  it('writes saved views through to settings', async () => {
    const views = useCodeViews()
    await flushPromises()
    views.addCondition({ field: 'tag', op: 'any', values: ['bug'] })
    views.saveAs('Bugs')
    await flushPromises()
    expect(mocks.SetCodeViews).toHaveBeenLastCalledWith([
      expect.objectContaining({
        name: 'Bugs',
        filter: { match: 'all', conditions: [{ field: 'tag', op: 'any', values: ['bug'] }] },
      }),
    ])
  })

  it('starts on All sessions with an empty, clean filter', async () => {
    const views = useCodeViews()
    await flushPromises()
    expect(views.activeViewId.value).toBe(ALL_SESSIONS_VIEW_ID)
    expect(views.draft.value.conditions).toEqual([])
    expect(views.dirty.value).toBe(false)
  })

  it('saves the draft as a new view and selects it', async () => {
    const views = useCodeViews()
    await flushPromises()
    views.addCondition({ field: 'tag', op: 'any', values: ['bug'] })
    expect(views.dirty.value).toBe(true)

    views.saveAs('Bugs')
    expect(views.activeView.value?.name).toBe('Bugs')
    expect(views.dirty.value).toBe(false)
  })

  it('resets edits back to the saved view and saves them in place', async () => {
    const views = useCodeViews()
    await flushPromises()
    views.addCondition({ field: 'tag', op: 'any', values: ['bug'] })
    views.saveAs('Bugs')

    views.addCondition({ field: 'status', op: 'is', value: 'running' })
    views.reset()
    expect(views.draft.value.conditions).toHaveLength(1)

    views.addCondition({ field: 'status', op: 'is', value: 'running' })
    views.save()
    expect(views.dirty.value).toBe(false)
    expect(views.activeView.value?.filter.conditions).toHaveLength(2)
  })

  it('switching views replaces the draft, and deleting the active view falls back to All', async () => {
    const views = useCodeViews()
    await flushPromises()
    views.addCondition({ field: 'tag', op: 'any', values: ['bug'] })
    views.saveAs('Bugs')
    const id = views.activeViewId.value

    views.select(ALL_SESSIONS_VIEW_ID)
    expect(views.draft.value.conditions).toEqual([])

    views.select(id)
    views.remove(id)
    await nextTick()
    expect(views.activeViewId.value).toBe(ALL_SESSIONS_VIEW_ID)
  })

  it('offers Running as a built-in view that a save cannot change', async () => {
    const views = useCodeViews()
    await flushPromises()
    views.select(RUNNING_VIEW_ID)
    expect(views.activeIsBuiltIn.value).toBe(true)
    expect(views.draft.value.conditions).toEqual([{ field: 'status', op: 'is', value: 'running' }])

    views.addCondition({ field: 'tag', op: 'any', values: ['bug'] })
    views.save()
    await flushPromises()
    expect(mocks.SetCodeViews).not.toHaveBeenCalled()
    expect(views.dirty.value).toBe(true)
  })

  it('moves the old running-only toggle onto the Running view, once', async () => {
    localStorage.setItem('hive.terminal.sidebar.running-only', 'true')
    const views = useCodeViews()
    await flushPromises()
    expect(views.activeViewId.value).toBe(RUNNING_VIEW_ID)
    expect(localStorage.getItem('hive.terminal.sidebar.running-only')).toBeNull()
  })

  it('does not override a view already chosen when adopting the old toggle', async () => {
    localStorage.setItem('hive.terminal.sidebar.running-only', 'true')
    localStorage.setItem('hive.code.active-view', ALL_SESSIONS_VIEW_ID)
    const views = useCodeViews()
    await flushPromises()
    expect(views.activeViewId.value).toBe(ALL_SESSIONS_VIEW_ID)
  })

  it('reorders saved views and ignores an order that drops one', async () => {
    mocks.CodeViews.mockResolvedValue([
      { id: 'a', name: 'A', filter: { match: 'all', conditions: [] } },
      { id: 'b', name: 'B', filter: { match: 'all', conditions: [] } },
    ])
    const views = useCodeViews()
    await flushPromises()
    views.reorder(['b', 'a'])
    expect(views.views.value.map((v) => v.id)).toEqual(['b', 'a'])
    views.reorder(['a'])
    expect(views.views.value.map((v) => v.id)).toEqual(['b', 'a'])
  })
})
