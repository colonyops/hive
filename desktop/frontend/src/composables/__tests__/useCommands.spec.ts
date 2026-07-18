import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import {
  filterAndScore,
  scoreCommand,
  sortCommands,
  useCommandPalette,
  useCommands,
  type Command,
} from '../useCommands'

function command(overrides: Partial<Command> & Pick<Command, 'id' | 'title'>): Command {
  return {
    group: 'Profiles',
    run: vi.fn(),
    ...overrides,
  }
}

describe('useCommands', () => {
  afterEach(() => {
    const palette = useCommandPalette()
    palette.open.value = false
    palette.query.value = ''
  })

  it('scores title, keyword, and group matches', () => {
    const cmd = command({
      id: 'open-repo',
      title: 'Open repository',
      group: 'Navigation',
      keywords: ['browser'],
    })

    expect(scoreCommand('', cmd)).toBe(0)
    expect(scoreCommand('open', cmd)).toBe(3)
    expect(scoreCommand('repo', cmd)).toBe(2)
    expect(scoreCommand('browser', cmd)).toBe(1)
    expect(scoreCommand('navigation', cmd)).toBe(1)
    expect(scoreCommand('missing', cmd)).toBe(-1)
  })

  it('sorts commands by group and title', () => {
    const sorted = sortCommands([
      command({ id: 'profile-work', title: 'Work profile', group: 'Profiles' }),
      command({ id: 'feed-desktop', title: 'Desktop feed', group: 'Feeds' }),
      command({ id: 'profile-personal', title: 'Personal profile', group: 'Profiles' }),
    ])

    expect(sorted.map((cmd) => cmd.id)).toEqual(['feed-desktop', 'profile-personal', 'profile-work'])
  })

  it('filters and ranks within grouped results', () => {
    const results = filterAndScore('profile', [
      command({ id: 'profile-prefix', title: 'Profile settings', group: 'Profiles' }),
      command({ id: 'profile-keyword', title: 'Desktop feed', group: 'Profiles', keywords: ['profile'] }),
      command({ id: 'feed-keyword', title: 'Review inbox', group: 'Feeds', keywords: ['profile'] }),
    ])

    expect(results.map((cmd) => cmd.id)).toEqual(['feed-keyword', 'profile-prefix', 'profile-keyword'])
  })

  it('registers commands and removes them when the effect scope is disposed', () => {
    const scope = effectScope()
    const palette = useCommandPalette()

    scope.run(() => useCommands([command({ id: 'alpha', title: 'Alpha command' })]))

    expect(palette.results.value.map((cmd) => cmd.id)).toContain('alpha')

    scope.stop()

    expect(palette.results.value.map((cmd) => cmd.id)).not.toContain('alpha')
  })

  it('updates results from a reactive command source', () => {
    const scope = effectScope()
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

    scope.stop()
  })

  it('runs a palette command, closes the palette, and clears the query', async () => {
    const scope = effectScope()
    const handler = vi.fn()
    const palette = useCommandPalette()

    scope.run(() => useCommands([command({ id: 'do-thing', title: 'Do thing', run: handler })]))
    palette.open.value = true
    palette.query.value = 'do'

    await palette.run(palette.results.value[0])

    expect(handler).toHaveBeenCalledTimes(1)
    expect(palette.open.value).toBe(false)
    expect(palette.query.value).toBe('')

    scope.stop()
  })
})
