import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref, type EffectScope } from 'vue'
import {
  filterAndScore,
  fuzzyMatch,
  scoreCommand,
  sortCommands,
  useCommandPalette,
  useCommands,
  useKeysScope,
  useShellEscape,
  type Command,
} from '../useCommands'
import { resetPaletteRecentsForTests, usePaletteRecents } from '../usePaletteRecents'

function command(overrides: Partial<Command> & Pick<Command, 'id' | 'title'>): Command {
  return {
    group: 'Profiles',
    run: vi.fn(),
    ...overrides,
  }
}

describe('useCommands', () => {
  let scope: EffectScope

  beforeEach(() => {
    scope = effectScope()
  })

  afterEach(() => {
    scope.stop()
    const palette = useCommandPalette()
    palette.open.value = false
    palette.query.value = ''
    palette.scope.value = 'all'
    resetPaletteRecentsForTests()
  })

  it('scores title, keyword, and group matches, banded so title > keyword > group', () => {
    const cmd = command({
      id: 'open-repo',
      title: 'Open repository',
      group: 'Navigation',
      keywords: ['browser'],
    })

    expect(scoreCommand('', cmd)).toBe(0)
    expect(scoreCommand('missing', cmd)).toBe(-1)

    const titleScore = scoreCommand('open', cmd) // title prefix
    const titleSubstringScore = scoreCommand('repo', cmd) // title substring, not a prefix
    const keywordScore = scoreCommand('browser', cmd) // exact keyword, no title match
    const groupScore = scoreCommand('navigation', cmd) // exact group, no title/keyword match ('v' isn't in the title)

    expect(titleScore).toBeGreaterThan(keywordScore)
    expect(titleSubstringScore).toBeGreaterThan(keywordScore)
    expect(keywordScore).toBeGreaterThan(groupScore)
    expect(groupScore).toBeGreaterThan(-1)
  })

  it('fuzzyMatch does a case-insensitive ordered subsequence match, or fails when a character is out of order', () => {
    expect(fuzzyMatch('brd', 'bread')?.positions).toEqual([0, 1, 4])
    expect(fuzzyMatch('BRD', 'Bread')?.positions).toEqual([0, 1, 4])
    expect(fuzzyMatch('xyz', 'bread')).toBeNull()
    expect(fuzzyMatch('db', 'bread')).toBeNull() // 'd' then 'b' — 'b' only appears before 'd'
  })

  // İ.toLowerCase() is "i̇" — two code units — so lowercasing the whole string
  // would shift every position after it out of step with the original title's
  // indices (which is what titleSegments slices from). Comparing character by
  // character and leaving a length-changing character as-is keeps positions
  // aligned to the original string.
  it('fuzzyMatch keeps positions aligned to the original string across a length-changing lowercase mapping', () => {
    const title = 'İstanbul office'
    const match = fuzzyMatch('office', title)

    expect(match).not.toBeNull()
    expect(match!.positions).toEqual([9, 10, 11, 12, 13, 14])
    expect(title.slice(match!.positions[0], match!.positions[0] + 6)).toBe('office')
  })

  it('fuzzyMatch ranks a whole-prefix match above a scattered subsequence hit', () => {
    const prefix = fuzzyMatch('mar', 'Mark all as read')
    const scattered = fuzzyMatch('mar', 'Send a mail, archive it')

    expect(prefix).not.toBeNull()
    expect(scattered).not.toBeNull()
    expect(prefix!.score).toBeGreaterThan(scattered!.score)
  })

  it('fuzzyMatch rewards a word-start hit over the same letter matched mid-word', () => {
    const wordStart = fuzzyMatch('mar', 'Send mark') // 'm' starts the second word
    const midWord = fuzzyMatch('mar', 'Denmark') // 'm' sits mid-word

    expect(wordStart).not.toBeNull()
    expect(midWord).not.toBeNull()
    expect(wordStart!.score).toBeGreaterThan(midWord!.score)
  })

  it('finds a scattered query across word starts end to end via filterAndScore', () => {
    const results = filterAndScore('mkalrd', [
      command({ id: 'mark-read', title: 'Mark all as read', group: 'Inbox' }),
      command({ id: 'unrelated', title: 'Open settings', group: 'App' }),
    ])

    expect(results.map((cmd) => cmd.id)).toEqual(['mark-read'])
  })

  it('sorts commands by group placement, keeping registration order within a group', () => {
    const sorted = sortCommands([
      command({ id: 'profile-work', title: 'Work profile', group: 'Profiles' }),
      command({ id: 'feed-desktop', title: 'Desktop feed', group: 'Feeds' }),
      command({ id: 'profile-personal', title: 'Personal profile', group: 'Profiles' }),
      command({ id: 'session-attach', title: 'Attach session', group: 'Sessions', order: -1 }),
    ])

    expect(sorted.map((cmd) => cmd.id)).toEqual([
      'session-attach',
      'feed-desktop',
      'profile-work',
      'profile-personal',
    ])
  })

  it('keeps a group contiguous when its registrars disagree on order', () => {
    // The attached session's group is written from two files: TerminalMode's
    // ops rows carry an order, the App-level window rows do not. The group
    // sorts as early as its earliest row asks, header rendered once.
    const sorted = sortCommands([
      command({ id: 'feed-desktop', title: 'Desktop feed', group: 'Feeds' }),
      command({ id: 'session-kill', title: 'Kill session', group: 'hive-fix-parser', order: -3 }),
      command({ id: 'window-one', title: 'editor', group: 'hive-fix-parser' }),
    ])

    expect(sorted.map((cmd) => cmd.id)).toEqual(['session-kill', 'window-one', 'feed-desktop'])
  })

  it('ranks a stronger match above an earlier group', () => {
    const results = filterAndScore('profile', [
      command({ id: 'profile-prefix', title: 'Profile settings', group: 'Profiles' }),
      command({ id: 'profile-keyword', title: 'Desktop feed', group: 'Profiles', keywords: ['profile'] }),
      command({ id: 'feed-keyword', title: 'Review inbox', group: 'Feeds', keywords: ['profile'] }),
    ])

    expect(results.map((cmd) => cmd.id)).toEqual(['profile-prefix', 'feed-keyword', 'profile-keyword'])
  })

  it('breaks score ties by group placement', () => {
    const results = filterAndScore('profile', [
      command({ id: 'feed-keyword', title: 'Review inbox', group: 'Feeds', keywords: ['profile'] }),
      command({ id: 'session-keyword', title: 'Attach session', group: 'Sessions', order: -1, keywords: ['profile'] }),
    ])

    expect(results.map((cmd) => cmd.id)).toEqual(['session-keyword', 'feed-keyword'])
  })

  it('registers commands and removes them when the effect scope is disposed', () => {
    const palette = useCommandPalette()

    scope.run(() => useCommands([command({ id: 'alpha', title: 'Alpha command' })]))

    expect(palette.results.value.map((cmd) => cmd.id)).toContain('alpha')

    scope.stop()

    expect(palette.results.value.map((cmd) => cmd.id)).not.toContain('alpha')
  })

  it('updates results from a reactive command source', () => {
    const enabled = ref(false)
    const palette = useCommandPalette()

    scope.run(() => {
      useCommands(() => [
        command({ id: 'always', title: 'Always available' }),
        ...(enabled.value ? [command({ id: 'dynamic', title: 'Dynamic command' })] : []),
      ])
    })

    expect(palette.results.value.map((cmd) => cmd.id)).toEqual(['always'])

    enabled.value = true

    expect(palette.results.value.map((cmd) => cmd.id)).toEqual(['always', 'dynamic'])
  })

  it('runs a palette command, closes the palette, and clears the query', async () => {
    const handler = vi.fn()
    const palette = useCommandPalette()

    scope.run(() => useCommands([command({ id: 'do-thing', title: 'Do thing', run: handler })]))
    palette.open.value = true
    palette.query.value = 'do'

    await palette.run(palette.results.value[0])

    expect(handler).toHaveBeenCalledTimes(1)
    expect(palette.open.value).toBe(false)
    expect(palette.query.value).toBe('')
  })

  it('routes !-queries to the shell escape instead of the fuzzy list', () => {
    const palette = useCommandPalette()

    scope.run(() => {
      useCommands([command({ id: 'decoy', title: '!important looking row' })])
      useShellEscape((line) => [command({ id: 'shell:run', title: `Run: ${line}` })])
    })

    palette.setQuery('!git status')

    expect(palette.scope.value).toBe('shell')
    expect(palette.results.value.map((cmd) => cmd.id)).toEqual(['shell:run'])
    expect(palette.results.value[0].title).toBe('Run: git status')
  })

  it('offers nothing for a bare ! and when every escape declines', () => {
    const palette = useCommandPalette()
    const lines: string[] = []

    scope.run(() => useShellEscape((line) => {
      lines.push(line)
      return []
    }))

    palette.setQuery('!')
    expect(palette.scope.value).toBe('shell')
    expect(palette.results.value).toEqual([])

    palette.setQuery('   ')
    expect(palette.results.value).toEqual([])
    expect(lines).toEqual([])

    palette.setQuery('ls')
    expect(palette.results.value).toEqual([])
    expect(lines).toEqual(['ls'])
  })

  it('releases the shell escape when its scope is disposed', () => {
    const palette = useCommandPalette()

    scope.run(() => useShellEscape((line) => [command({ id: 'shell:run', title: line })]))
    palette.setQuery('!ls')
    expect(palette.results.value).toHaveLength(1)

    scope.stop()

    expect(palette.results.value).toEqual([])
  })

  // ── Scopes ─────────────────────────────────────────────────────────────────

  it('filters results to the active scope, defaulting an unmarked row to actions', () => {
    const palette = useCommandPalette()

    scope.run(() => useCommands([
      command({ id: 'goto-a', title: 'Goto A', scope: 'goto' }),
      command({ id: 'act-a', title: 'Actions A' }),
    ]))

    palette.scope.value = 'goto'
    expect(palette.results.value.map((cmd) => cmd.id)).toEqual(['goto-a'])

    palette.scope.value = 'actions'
    expect(palette.results.value.map((cmd) => cmd.id)).toEqual(['act-a'])
  })

  it('intercepts a leading sigil on an empty query, entering that scope with it stripped', () => {
    const palette = useCommandPalette()

    // A paste of "@foo" lands in the goto scope with query "foo".
    palette.setQuery('@foo')
    expect(palette.scope.value).toBe('goto')
    expect(palette.query.value).toBe('foo')

    palette.setQuery('')
    palette.setQuery('>run')
    expect(palette.scope.value).toBe('actions')
    expect(palette.query.value).toBe('run')
  })

  it('leaves a sigil as literal text once the query is already non-empty', () => {
    const palette = useCommandPalette()

    palette.setQuery('a')
    palette.setQuery('a@b')

    expect(palette.scope.value).toBe('all')
    expect(palette.query.value).toBe('a@b')
  })

  // The sigil only enters a scope that's actually reachable via the tab strip
  // — with no shell escape registered, Shell isn't offered, so "!" must not
  // silently switch scope out from under an unrelated "!ls" query.
  it('leaves a sigil as literal text when its scope is not visible', () => {
    const palette = useCommandPalette()

    palette.setQuery('!ls')

    expect(palette.scope.value).toBe('all')
    expect(palette.query.value).toBe('!ls')
  })

  it('hides the Shell tab until an escape claims it as available', () => {
    const palette = useCommandPalette()

    expect(palette.visibleScopes.value.map((s) => s.id)).toEqual(['all', 'goto', 'actions'])

    scope.run(() => useShellEscape(() => []))
    expect(palette.visibleScopes.value.map((s) => s.id)).toEqual(['all', 'goto', 'actions', 'shell'])

    scope.stop()
    expect(palette.visibleScopes.value.map((s) => s.id)).toEqual(['all', 'goto', 'actions'])
  })

  it('cycles forward and backward through only the visible scopes, wrapping at both ends', () => {
    const palette = useCommandPalette()

    // No shell escape registered, so Shell is not offered.
    expect(palette.scope.value).toBe('all')
    palette.cycleScope(1)
    expect(palette.scope.value).toBe('goto')
    palette.cycleScope(1)
    expect(palette.scope.value).toBe('actions')
    palette.cycleScope(1)
    expect(palette.scope.value).toBe('all')

    palette.cycleScope(-1)
    expect(palette.scope.value).toBe('actions')
  })

  it('pops to All only on an empty query, and never when already on All', () => {
    const palette = useCommandPalette()

    palette.scope.value = 'goto'
    palette.query.value = 'repo'
    expect(palette.popScope()).toBe(false)
    expect(palette.scope.value).toBe('goto')

    palette.query.value = ''
    expect(palette.popScope()).toBe(true)
    expect(palette.scope.value).toBe('all')

    expect(palette.popScope()).toBe(false)
  })

  it('hides the Keys tab until a provider registers, and routes ?-queries to it', () => {
    const palette = useCommandPalette()

    expect(palette.visibleScopes.value.map((s) => s.id)).not.toContain('keys')

    scope.run(() => useKeysScope(() => [command({ id: 'keys:feed.next', title: 'Next item' })]))
    expect(palette.visibleScopes.value.map((s) => s.id)).toContain('keys')

    palette.setQuery('?next')
    expect(palette.scope.value).toBe('keys')
    expect(palette.results.value.map((cmd) => cmd.id)).toEqual(['keys:feed.next'])

    scope.stop()
    expect(palette.visibleScopes.value.map((s) => s.id)).not.toContain('keys')
  })

  it('keeps the palette open and the query untouched when a command carries keepOpen', () => {
    const palette = useCommandPalette()
    const handler = vi.fn()

    scope.run(() => useCommands([command({ id: 'legend:goto', title: 'Jump to a place', keepOpen: true, run: handler })]))
    palette.open.value = true
    palette.query.value = 'jump'

    palette.run(palette.results.value[0])

    expect(handler).toHaveBeenCalledTimes(1)
    expect(palette.open.value).toBe(true)
    expect(palette.query.value).toBe('jump')
  })

  // A sigil-legend row's own run() sets the new scope; keepOpen must not let
  // run() reset that scope back to All right after.
  it('lets a keepOpen row switch scope without the palette closing or resetting it', () => {
    const palette = useCommandPalette()

    scope.run(() => useCommands([
      command({ id: 'legend:actions', title: 'Run a command', keepOpen: true, run: () => { palette.setScope('actions') } }),
    ]))
    palette.open.value = true
    palette.query.value = 'run'

    palette.run(palette.results.value[0])

    expect(palette.open.value).toBe(true)
    expect(palette.scope.value).toBe('actions')
  })

  it('snaps the active scope back to All when it disappears from visibleScopes, keeping the query', () => {
    const palette = useCommandPalette()
    const available = ref(true)

    scope.run(() => useShellEscape((line) => [command({ id: 'shell:run', title: line })], () => available.value))

    palette.open.value = true
    palette.setQuery('!ls')
    expect(palette.scope.value).toBe('shell')
    expect(palette.query.value).toBe('ls')

    available.value = false
    expect(palette.scope.value).toBe('all')
    expect(palette.query.value).toBe('ls')
  })

  // ── Recent recording ───────────────────────────────────────────────────────

  it('records a run made from the All scope', () => {
    const palette = useCommandPalette()
    scope.run(() => useCommands([command({ id: 'do-thing', title: 'Do thing' })]))
    palette.open.value = true

    palette.run(palette.results.value[0])

    expect(usePaletteRecents().recentIds.value).toEqual(['do-thing'])
  })

  it('records a run made from the Go to and Actions scopes, most recent first', () => {
    const palette = useCommandPalette()
    scope.run(() => useCommands([
      command({ id: 'goto-a', title: 'Goto A', scope: 'goto' }),
      command({ id: 'act-a', title: 'Actions A' }),
    ]))

    palette.scope.value = 'goto'
    palette.run(palette.results.value[0])
    palette.scope.value = 'actions'
    palette.run(palette.results.value[0])

    expect(usePaletteRecents().recentIds.value).toEqual(['act-a', 'goto-a'])
  })

  it('does not record a Shell escape run — it is a line, not a repeatable entry', () => {
    const palette = useCommandPalette()
    scope.run(() => useShellEscape((line) => [command({ id: 'shell:run', title: `Run: ${line}` })]))
    palette.setQuery('!ls')

    palette.run(palette.results.value[0])

    expect(usePaletteRecents().recentIds.value).toEqual([])
  })

  it('does not record a Keys row run — it is reference, not something to repeat', () => {
    const palette = useCommandPalette()
    scope.run(() => useKeysScope(() => [command({ id: 'keys:feed.next', title: 'Next item' })]))
    palette.setQuery('?next')

    palette.run(palette.results.value[0])

    expect(usePaletteRecents().recentIds.value).toEqual([])
  })

  it('does not record a keepOpen row run — sigil-legend rows are navigation chrome', () => {
    const palette = useCommandPalette()
    scope.run(() => useCommands([command({ id: 'legend:goto', title: 'Jump to a place', keepOpen: true })]))
    palette.open.value = true
    palette.query.value = 'jump'

    palette.run(palette.results.value[0])

    expect(usePaletteRecents().recentIds.value).toEqual([])
  })
})
