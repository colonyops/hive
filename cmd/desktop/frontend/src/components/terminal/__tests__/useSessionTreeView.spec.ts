import { afterEach, describe, expect, it } from 'vitest'
import { computed, effectScope, nextTick, ref } from 'vue'
import type { TerminalSessionGroup, TerminalSessionRow } from '../../../stores/useTerminalSessions'
import { useSessionTreeView } from '../useSessionTreeView'

function row(id: string, name: string, repo = 'github.com/acme/site'): TerminalSessionRow {
  return { id, name, slug: `${name}-${id}`, repo, state: 'active' }
}

const parser = row('1', 'parser')
const docs = row('2', 'docs')
const api = row('3', 'api', 'github.com/acme/api')
const scratch = row('s', 'scratch', '')

function setup(running: string[] = [], settled = true) {
  const groups = ref<TerminalSessionGroup[]>([
    { key: 'scratch', name: 'Scratch', kind: 'scratch', sessions: [scratch] },
    { key: 'github.com/acme/api', name: 'acme/api', kind: 'repo', sessions: [api] },
    { key: 'github.com/acme/site', name: 'acme/site', kind: 'repo', sessions: [parser, docs] },
  ])
  const statusesLoaded = ref(true)
  const sessionsLoaded = ref(true)
  const listingsSettled = ref(settled)
  const tree = {
    sessionGroups: computed(() => groups.value),
    activeSessions: computed(() => [parser, docs, api]),
    attachable: computed(() => [scratch, parser, docs, api]),
    alwaysSwept: computed(() => [scratch]),
    rowRunning: (candidate: TerminalSessionRow) => running.includes(candidate.id),
    groupAttached: () => false,
    groupRunning: (group: TerminalSessionGroup) => group.sessions.some((r) => running.includes(r.id)),
    statusesLoaded,
    sessionsLoaded,
    sessionsError: ref(''),
    showAllWindows: ref(false),
    showAllWindowsReady: ref(true),
    listingsSettled,
  }
  const view = effectScope().run(() => useSessionTreeView(tree))!
  return { view, statusesLoaded, listingsSettled }
}

const names = (groups: TerminalSessionGroup[]) => groups.map((group) => group.sessions.map((r) => r.name))

afterEach(() => localStorage.clear())

describe('useSessionTreeView', () => {
  it('narrows sessions by name or slug, and a repo-name match carries its whole group', () => {
    const { view } = setup()
    view.query.value = 'pars'
    expect(names(view.groups.value)).toEqual([['parser']])
    view.query.value = 'docs-2'
    expect(names(view.groups.value)).toEqual([['docs']])
    view.query.value = 'acme/site'
    expect(names(view.groups.value)).toEqual([['parser', 'docs']])
  })

  it('keeps the pinned sections when narrowing to running sessions, and says how many it hid', () => {
    const { view } = setup(['1'])
    view.runningOnly.value = true
    expect(names(view.groups.value)).toEqual([['scratch'], ['parser']])
    expect(view.runningNote.value).toBe('Running sessions only · 2 hidden')
  })

  it('does not narrow to running sessions before the first status poll lands', () => {
    const { view, statusesLoaded } = setup([])
    statusesLoaded.value = false
    view.runningOnly.value = true
    expect(names(view.groups.value)).toHaveLength(3)
  })

  it('reports no matches when only the pinned sections are left', async () => {
    const { view } = setup([])
    await nextTick()
    view.runningOnly.value = true
    expect(view.note.value).toBe('no-matches')
    expect(view.noMatchesNote.value).toBe('No sessions are running.')
    view.runningOnly.value = false
    view.query.value = 'zzz'
    expect(view.note.value).toBe('no-matches')
    expect(view.noMatchesNote.value).toBe('No sessions match “zzz”.')
  })

  it('holds the first paint until the window sweep it waits on has settled, then never again', async () => {
    const { view, listingsSettled } = setup([], false)
    expect(view.ready.value).toBe(false)
    listingsSettled.value = true
    await nextTick()
    expect(view.ready.value).toBe(true)
    listingsSettled.value = false
    await nextTick()
    expect(view.ready.value).toBe(true)
  })

  it('opens every group while a query is typed', () => {
    const { view } = setup([])
    const site = view.groups.value[2]
    expect(view.expansion.expanded(site)).toBe(false)
    view.query.value = 'docs'
    expect(view.expansion.expanded(site)).toBe(true)
  })
})
