import { flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ALL_SESSIONS_VIEW_ID } from '../../lib/sessionViews'
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
})
