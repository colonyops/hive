import { beforeEach, describe, expect, it, vi } from 'vitest'

const service = vi.hoisted(() => ({ Launchers: vi.fn() }))

vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/popupterminalservice',
  () => ({
    Launchers: service.Launchers,
  }),
)

beforeEach(() => {
  service.Launchers.mockReset()
  vi.resetModules()
})

describe('useLaunchers', () => {
  it('turns each configured launcher into a bindable command', async () => {
    service.Launchers.mockResolvedValue([
      { id: 'lazygit', label: 'lazygit', icon: 'git-branch', requiresSession: true },
      { id: 'btop', label: '', icon: '', requiresSession: true },
    ])
    const { useLaunchers } = await import('../useLaunchers')
    const { commands } = await import('../../keybindings/catalog')

    await useLaunchers().refresh()

    const launchers = commands.value.filter((command) => command.group === 'Quick terminals')
    expect(launchers.map((command) => [command.id, command.title])).toEqual([
      ['launcher.lazygit', 'lazygit'],
      // A launcher with no label falls back to its id rather than an empty row.
      ['launcher.btop', 'btop'],
    ])
    // Unbound until the user says otherwise: config must not claim a chord.
    expect(launchers.every((command) => command.defaultCombos.length === 0)).toBe(true)
  })

  // Where a launcher may run is the core's answer, and it arrives as the
  // command's context so the palette and the keymap gate on the same thing the
  // core enforces.
  it('scopes a launcher to a session unless it carries its own directory', async () => {
    service.Launchers.mockResolvedValue([
      { id: 'lazygit', label: 'lazygit', icon: '', requiresSession: true },
      { id: 'dotfiles', label: 'Edit dotfiles', icon: '', requiresSession: false },
    ])
    const { useLaunchers } = await import('../useLaunchers')
    const { commands } = await import('../../keybindings/catalog')

    await useLaunchers().refresh()

    expect(
      commands.value
        .filter((command) => command.group === 'Quick terminals')
        .map((command) => [command.id, command.context]),
    ).toEqual([
      ['launcher.lazygit', 'terminal-session'],
      ['launcher.dotfiles', 'global'],
    ])
  })

  it('replaces the previous set rather than accumulating', async () => {
    service.Launchers.mockResolvedValue([{ id: 'lazygit', label: 'lazygit', icon: '' }])
    const { useLaunchers } = await import('../useLaunchers')
    const { commands } = await import('../../keybindings/catalog')
    await useLaunchers().refresh()

    service.Launchers.mockResolvedValue([{ id: 'btop', label: 'btop', icon: '' }])
    await useLaunchers().refresh()

    expect(commands.value.filter((command) => command.group === 'Quick terminals').map((c) => c.id)).toEqual([
      'launcher.btop',
    ])
  })

  // Losing every launcher shortcut over one failed read is worse than serving
  // the set from a moment ago.
  it('keeps the registered launchers when the catalog cannot be read', async () => {
    service.Launchers.mockResolvedValue([{ id: 'lazygit', label: 'lazygit', icon: '' }])
    const { useLaunchers } = await import('../useLaunchers')
    const { commands } = await import('../../keybindings/catalog')
    await useLaunchers().refresh()

    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    service.Launchers.mockRejectedValue(new Error('unavailable'))
    await useLaunchers().refresh()

    expect(warn).toHaveBeenCalled()
    expect(commands.value.filter((command) => command.group === 'Quick terminals').map((c) => c.id)).toEqual([
      'launcher.lazygit',
    ])
    warn.mockRestore()
  })

  // A null crossing the Wails bridge is an empty catalog, not a failure.
  it('reads a null answer as no launchers', async () => {
    service.Launchers.mockResolvedValue(null)
    const { useLaunchers } = await import('../useLaunchers')
    const { commands } = await import('../../keybindings/catalog')

    await useLaunchers().refresh()

    expect(commands.value.some((command) => command.group === 'Quick terminals')).toBe(false)
  })
})
