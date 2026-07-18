import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CommandPalette from '../CommandPalette.vue'
import { useCommandPalette, useCommands, type Command } from '../../composables/useCommands'

const runBackend = vi.fn()
const runDesktop = vi.fn()
const runPersonal = vi.fn()

// Empty query sorts by group asc then title asc:
// Feeds → backend, desktop; Profiles → personal.
const commands: Command[] = [
  { id: 'profile-personal', title: 'Switch to personal', group: 'Profiles', run: runPersonal },
  { id: 'feed-desktop', title: 'Open desktop feed', group: 'Feeds', run: runDesktop },
  { id: 'feed-backend', title: 'Open backend feed', group: 'Feeds', run: runBackend },
]

function mountPalette() {
  return mount(
    {
      components: { CommandPalette },
      template: '<CommandPalette />',
      setup() {
        useCommands(commands)
        return {}
      },
    },
    {
      attachTo: document.body,
      global: { stubs: { teleport: true } },
    },
  )
}

async function openPalette() {
  useCommandPalette().toggle()
  await flushPromises()
}

describe('CommandPalette', () => {
  let wrapper: ReturnType<typeof mountPalette> | undefined

  beforeEach(() => {
    vi.clearAllMocks()
    wrapper = mountPalette()
  })

  afterEach(() => {
    // Unconditional cleanup: unmounting disposes the useCommands registration
    // even when an assertion failed mid-test, and the module-scoped palette
    // state is reset by hand.
    wrapper?.unmount()
    wrapper = undefined
    const palette = useCommandPalette()
    palette.open.value = false
    palette.query.value = ''
  })

  function panel() {
    return wrapper!.find('[data-testid="command-palette"]')
  }

  function selectedRows() {
    return wrapper!.findAll('.palette-row').filter((row) => row.classes().includes('palette-row-selected'))
  }

  it('focuses the input on open and lists commands grouped under headers', async () => {
    await openPalette()

    expect(document.activeElement).toBe(wrapper!.find('[data-testid="command-palette-input"]').element)

    const entries = wrapper!.findAll('.palette-results > *').map((node) => ({
      header: node.classes().includes('palette-group-header'),
      text: node.text(),
    }))
    expect(entries).toEqual([
      { header: true, text: 'Feeds' },
      { header: false, text: 'Open backend feed' },
      { header: false, text: 'Open desktop feed' },
      { header: true, text: 'Profiles' },
      { header: false, text: 'Switch to personal' },
    ])
  })

  it('moves the selection with ArrowDown/ArrowUp and wraps at both ends', async () => {
    await openPalette()
    expect(selectedRows().map((row) => row.text())).toEqual(['Open backend feed'])

    await panel().trigger('keydown', { key: 'ArrowDown' })
    expect(selectedRows().map((row) => row.text())).toEqual(['Open desktop feed'])

    await panel().trigger('keydown', { key: 'ArrowDown' })
    expect(selectedRows().map((row) => row.text())).toEqual(['Switch to personal'])

    await panel().trigger('keydown', { key: 'ArrowDown' })
    expect(selectedRows().map((row) => row.text())).toEqual(['Open backend feed'])

    await panel().trigger('keydown', { key: 'ArrowUp' })
    expect(selectedRows().map((row) => row.text())).toEqual(['Switch to personal'])
  })

  it('runs the selected command on Enter, closes the palette, and clears the query', async () => {
    const palette = useCommandPalette()
    await openPalette()
    palette.query.value = 'open'
    await flushPromises()

    await panel().trigger('keydown', { key: 'ArrowDown' })
    await panel().trigger('keydown', { key: 'Enter' })

    expect(runDesktop).toHaveBeenCalledTimes(1)
    expect(runBackend).not.toHaveBeenCalled()
    expect(palette.open.value).toBe(false)
    expect(palette.query.value).toBe('')
  })

  it('shows the empty state for a query with no results and ignores Enter', async () => {
    const palette = useCommandPalette()
    await openPalette()

    await wrapper!.find('[data-testid="command-palette-input"]').setValue('zzz')

    expect(wrapper!.findAll('.palette-row')).toHaveLength(0)
    expect(wrapper!.find('.palette-empty').text()).toContain('No results for "zzz"')

    await panel().trigger('keydown', { key: 'Enter' })

    expect(palette.open.value).toBe(true)
    expect(runBackend).not.toHaveBeenCalled()
    expect(runDesktop).not.toHaveBeenCalled()
    expect(runPersonal).not.toHaveBeenCalled()
  })

  it('closes on backdrop click but not on clicks inside the panel', async () => {
    const palette = useCommandPalette()
    await openPalette()

    await panel().trigger('click')
    expect(palette.open.value).toBe(true)

    await wrapper!.find('.palette-backdrop').trigger('click')
    expect(palette.open.value).toBe(false)
  })

  it('resets the selection to the first row when the results change', async () => {
    await openPalette()

    await panel().trigger('keydown', { key: 'ArrowDown' })
    await panel().trigger('keydown', { key: 'ArrowDown' })
    expect(selectedRows().map((row) => row.text())).toEqual(['Switch to personal'])

    await wrapper!.find('[data-testid="command-palette-input"]').setValue('open')

    expect(wrapper!.findAll('.palette-row').map((row) => row.text())).toEqual([
      'Open backend feed',
      'Open desktop feed',
    ])
    expect(selectedRows().map((row) => row.text())).toEqual(['Open backend feed'])
  })
})
