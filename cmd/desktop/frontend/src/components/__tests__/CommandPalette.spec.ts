import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import CommandPalette from '../CommandPalette.vue'
import { useCommandPalette, useCommands, useShellEscape, type Command } from '../../composables/useCommands'
import { resetPaletteRecentsForTests, usePaletteRecents } from '../../composables/usePaletteRecents'

const runBackend = vi.fn()
const runDesktop = vi.fn()
const runPersonal = vi.fn()
const runShell = vi.fn()

// Empty query sorts by group placement, registration order within a group:
// Feeds → desktop, backend; Profiles → personal.
const commands: Command[] = [
  { id: 'profile-personal', title: 'Switch to personal', group: 'Profiles', run: runPersonal },
  { id: 'feed-desktop', title: 'Open desktop feed', group: 'Feeds', kind: 'feed', run: runDesktop },
  { id: 'feed-backend', title: 'Open backend feed', group: 'Feeds', run: runBackend },
]

function mountPalette() {
  return mount(
    {
      components: { CommandPalette },
      template: '<CommandPalette />',
      setup() {
        useCommands(commands)
        useShellEscape((line) => [{ id: 'shell:run', title: `Run: ${line}`, run: () => runShell(line) }])
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
    resetPaletteRecentsForTests()
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
    palette.scope.value = 'all'
    resetPaletteRecentsForTests()
  })

  function panel() {
    return wrapper!.find('[data-testid="command-palette"]')
  }

  // Row text is read from .palette-title: rows also contain an icon chip, an
  // optional hint, and the ↵ badge on the selected row.
  function rowTitle(row: ReturnType<typeof panel>) {
    return row.find('.palette-title').text()
  }

  // The container prefix rendered ahead of the title — present on a typed-
  // query row and, now, a Recent row; absent (empty) on an ordinary grouped
  // row, which gets its container from the section header above it instead.
  function rowScope(row: ReturnType<typeof panel>) {
    const scope = row.find('[data-testid="command-palette-command-scope"]')
    return scope.exists() ? scope.text() : ''
  }

  function selectedRows() {
    return wrapper!.findAll('.palette-row').filter((row) => row.classes().includes('palette-row-selected'))
  }

  it('focuses the input on open and lists commands grouped under headers', async () => {
    await openPalette()

    expect(document.activeElement).toBe(wrapper!.find('[data-testid="command-palette-input"]').element)

    const entries = wrapper!.findAll('.palette-results > *').map((node) => ({
      header: node.classes().includes('palette-group-header'),
      text: node.classes().includes('palette-group-header') ? node.text() : rowTitle(node),
    }))
    expect(entries).toEqual([
      { header: true, text: 'Feeds' },
      { header: false, text: 'Open desktop feed' },
      { header: false, text: 'Open backend feed' },
      { header: true, text: 'Profiles' },
      { header: false, text: 'Switch to personal' },
    ])
  })

  it('renders the kind word only on rows that declare one', async () => {
    await openPalette()

    const kinds = wrapper!.findAll('.palette-row').map((row) => {
      const kind = row.find('[data-testid="command-palette-command-kind"]')
      return kind.exists() ? kind.text() : ''
    })
    expect(kinds).toEqual(['feed', '', ''])
  })

  it('moves the selection with ArrowDown/ArrowUp and wraps at both ends', async () => {
    await openPalette()
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Open desktop feed'])

    await panel().trigger('keydown', { key: 'ArrowDown' })
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Open backend feed'])

    await panel().trigger('keydown', { key: 'ArrowDown' })
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Switch to personal'])

    await panel().trigger('keydown', { key: 'ArrowDown' })
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Open desktop feed'])

    await panel().trigger('keydown', { key: 'ArrowUp' })
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Switch to personal'])
  })

  it('ignores a mousemove that did not actually move the pointer', async () => {
    await openPalette()
    const rows = wrapper!.findAll('.palette-row')

    // The first event after open only anchors where the pointer rests.
    await rows[2]!.trigger('mousemove', { clientX: 40, clientY: 80 })
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Open desktop feed'])

    await panel().trigger('keydown', { key: 'ArrowDown' })
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Open backend feed'])

    // Same coordinates — WebKit's fake mousemove after a keyboard scroll:
    // the row under the cursor changed because the list moved, not the
    // mouse, so the arrow-key selection stays put.
    await rows[2]!.trigger('mousemove', { clientX: 40, clientY: 80 })
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Open backend feed'])

    // A pointer that really moved takes the selection.
    await rows[2]!.trigger('mousemove', { clientX: 41, clientY: 82 })
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Switch to personal'])
  })

  it('runs the selected command on Enter, closes the palette, and clears the query', async () => {
    const palette = useCommandPalette()
    await openPalette()
    palette.query.value = 'open'
    await flushPromises()

    await panel().trigger('keydown', { key: 'ArrowDown' })
    await panel().trigger('keydown', { key: 'Enter' })

    expect(runBackend).toHaveBeenCalledTimes(1)
    expect(runDesktop).not.toHaveBeenCalled()
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

  // The Code view's rows are rebuilt whenever the state they read changes, and
  // session statuses poll — so a selection that reset on every rebuild would
  // walk back to the top on its own, under the user's arrow keys.
  it('keeps the selected row when the results are rebuilt around it', async () => {
    wrapper!.unmount()
    const churn = ref(0)
    wrapper = mount(
      {
        components: { CommandPalette },
        template: '<CommandPalette />',
        setup() {
          useCommands(() => [
            { id: 'a', title: 'Alpha', group: 'Group', run: vi.fn() },
            { id: 'b', title: `Bravo ${churn.value}`, group: 'Group', run: vi.fn() },
            { id: 'c', title: 'Charlie', group: 'Group', run: vi.fn() },
          ])
          return {}
        },
      },
      { attachTo: document.body, global: { stubs: { teleport: true } } },
    )
    await openPalette()

    await panel().trigger('keydown', { key: 'ArrowDown' })
    await panel().trigger('keydown', { key: 'ArrowDown' })
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Charlie'])

    churn.value++
    await flushPromises()
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Charlie'])
  })

  // A row that goes away takes the selection with it rather than stranding it.
  it('falls back to the first row when the selected command disappears', async () => {
    wrapper!.unmount()
    const present = ref(true)
    wrapper = mount(
      {
        components: { CommandPalette },
        template: '<CommandPalette />',
        setup() {
          useCommands(() => present.value
            ? [{ id: 'a', title: 'Alpha', run: vi.fn() }, { id: 'b', title: 'Bravo', run: vi.fn() }]
            : [{ id: 'a', title: 'Alpha', run: vi.fn() }])
          return {}
        },
      },
      { attachTo: document.body, global: { stubs: { teleport: true } } },
    )
    await openPalette()

    await panel().trigger('keydown', { key: 'ArrowDown' })
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Bravo'])

    present.value = false
    await flushPromises()
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Alpha'])
  })

  it('resets the selection to the first row when the query changes', async () => {
    await openPalette()

    await panel().trigger('keydown', { key: 'ArrowDown' })
    await panel().trigger('keydown', { key: 'ArrowDown' })
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Switch to personal'])

    await wrapper!.find('[data-testid="command-palette-input"]').setValue('open')

    // Two title-prefix hits rank above 'Switch to personal', which the fuzzy
    // matcher still finds — 'o', 'p', 'e', 'n' appear in that order — as a
    // weak scattered subsequence hit.
    expect(wrapper!.findAll('.palette-row').map((row) => rowTitle(row))).toEqual([
      'Open desktop feed',
      'Open backend feed',
      'Switch to personal',
    ])
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Open desktop feed'])
  })

  it('highlights the matched substring in result titles, case-insensitively', async () => {
    await openPalette()

    await wrapper!.find('[data-testid="command-palette-input"]').setValue('open')

    // The two prefix hits highlight as one run each ('Open'); the scattered
    // hit on 'Switch to personal' highlights its four matched characters
    // individually ('o', 'pe', 'n' — 'p' and 'e' land adjacent).
    expect(wrapper!.findAll('.palette-title-match').map((node) => node.text())).toEqual(['Open', 'Open', 'o', 'pe', 'n'])
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Open desktop feed'])
  })

  // Fuzzy matching lets a query scatter across a title, so highlighting must
  // mark each matched run individually rather than one contiguous span.
  it('highlights a scattered fuzzy match per matched run, merging adjacent hits', async () => {
    wrapper!.unmount()
    wrapper = mount(
      {
        components: { CommandPalette },
        template: '<CommandPalette />',
        setup() {
          useCommands([{ id: 'mark-read', title: 'Mark all as read', run: vi.fn() }])
          return {}
        },
      },
      { attachTo: document.body, global: { stubs: { teleport: true } } },
    )
    await openPalette()

    await wrapper!.find('[data-testid="command-palette-input"]').setValue('mkalrd')

    const row = wrapper!.find('.palette-row')
    expect(rowTitle(row)).toBe('Mark all as read')
    expect(row.findAll('.palette-title-match').map((node) => node.text())).toEqual(['M', 'k', 'al', 'r', 'd'])
  })

  it('replaces headers with a per-row scope prefix while filtering', async () => {
    await openPalette()

    await wrapper!.find('[data-testid="command-palette-input"]').setValue('open')

    expect(wrapper!.findAll('.palette-group-header')).toHaveLength(0)
    expect(wrapper!.findAll('[data-testid="command-palette-command-scope"]').map((node) => node.text()))
      .toEqual(['Feeds ›', 'Feeds ›', 'Profiles ›'])
  })

  function tabs() {
    return wrapper!.findAll('[data-testid="command-palette-tab"]')
  }

  it('renders a tab per visible scope with All active by default', async () => {
    await openPalette()

    expect(tabs().map((tab) => tab.attributes('data-scope'))).toEqual(['all', 'goto', 'actions', 'shell'])
    expect(tabs().filter((tab) => tab.classes().includes('palette-tab-active')).map((tab) => tab.attributes('data-scope')))
      .toEqual(['all'])
  })

  it('clicking a tab switches scope and refocuses the input', async () => {
    const palette = useCommandPalette()
    await openPalette()

    const gotoTab = tabs().find((tab) => tab.attributes('data-scope') === 'goto')!
    await gotoTab.trigger('click')

    expect(palette.scope.value).toBe('goto')
    expect(gotoTab.classes()).toContain('palette-tab-active')
    expect(document.activeElement).toBe(wrapper!.find('[data-testid="command-palette-input"]').element)
  })

  // setQuery's sigil interception is a no-op when the sigil enters the scope
  // that's already active (e.g. typing "@" again while on Go to): no reactive
  // change, so nothing would re-render the input back to the empty query
  // without the component forcing it.
  it('resyncs the input to the query when typing the active scope\'s own sigil on an empty query', async () => {
    const palette = useCommandPalette()
    await openPalette()
    await tabs().find((tab) => tab.attributes('data-scope') === 'goto')!.trigger('click')
    expect(palette.query.value).toBe('')

    const input = wrapper!.find('[data-testid="command-palette-input"]')
    await input.setValue('@')

    expect(palette.scope.value).toBe('goto')
    expect(palette.query.value).toBe('')
    expect((input.element as HTMLInputElement).value).toBe('')
  })

  it('cycles scope forward and backward with Tab and Shift+Tab', async () => {
    const palette = useCommandPalette()
    await openPalette()
    expect(palette.scope.value).toBe('all')

    await panel().trigger('keydown', { key: 'Tab' })
    expect(palette.scope.value).toBe('goto')

    await panel().trigger('keydown', { key: 'Tab' })
    expect(palette.scope.value).toBe('actions')

    await panel().trigger('keydown', { key: 'Tab', shiftKey: true })
    expect(palette.scope.value).toBe('goto')
  })

  it('pops to All on Backspace with an empty query', async () => {
    const palette = useCommandPalette()
    await openPalette()
    palette.scope.value = 'goto'
    await flushPromises()

    await panel().trigger('keydown', { key: 'Backspace' })

    expect(palette.scope.value).toBe('all')
  })

  it('deletes a character on Backspace with a non-empty query, without popping scope', async () => {
    const palette = useCommandPalette()
    await openPalette()
    palette.scope.value = 'goto'
    await wrapper!.find('[data-testid="command-palette-input"]').setValue('abc')

    const event = new KeyboardEvent('keydown', { key: 'Backspace', bubbles: true, cancelable: true })
    panel().element.dispatchEvent(event)
    await flushPromises()

    expect(palette.scope.value).toBe('goto')
    expect(event.defaultPrevented).toBe(false)
  })

  // A scope switch is a new question, like a query change — even when the
  // selected row is still present in the narrower scope's results, the
  // highlight resets to the top rather than following it.
  it('resets the selection to the first visible row on a scope switch', async () => {
    wrapper!.unmount()
    wrapper = mount(
      {
        components: { CommandPalette },
        template: '<CommandPalette />',
        setup() {
          useCommands([
            { id: 'goto-a', title: 'Goto A', scope: 'goto', run: vi.fn() },
            { id: 'act-a', title: 'Act A', run: vi.fn() },
            { id: 'act-b', title: 'Act B', run: vi.fn() },
          ])
          return {}
        },
      },
      { attachTo: document.body, global: { stubs: { teleport: true } } },
    )
    await openPalette()

    // All: registration order with no groups, so no headers. Select the
    // lower of the two rows that will still be present after the switch.
    await panel().trigger('keydown', { key: 'ArrowDown' })
    await panel().trigger('keydown', { key: 'ArrowDown' })
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Act B'])

    const actionsTab = tabs().find((tab) => tab.attributes('data-scope') === 'actions')!
    await actionsTab.trigger('click')

    expect(wrapper!.findAll('.palette-row').map((row) => rowTitle(row))).toEqual(['Act A', 'Act B'])
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Act A'])
  })

  it('shows only the shell escape for a !-query and runs it on Enter', async () => {
    const palette = useCommandPalette()
    await openPalette()

    await wrapper!.find('[data-testid="command-palette-input"]').setValue('!make build')

    const rows = wrapper!.findAll('.palette-row')
    expect(rows).toHaveLength(1)
    expect(rowTitle(rows[0])).toBe('Run: make build')
    expect(wrapper!.findAll('.palette-group-header')).toHaveLength(0)

    await panel().trigger('keydown', { key: 'Enter' })

    expect(runShell).toHaveBeenCalledWith('make build')
    expect(runBackend).not.toHaveBeenCalled()
    expect(palette.open.value).toBe(false)
  })

  // ── Recent section ────────────────────────────────────────────────────────

  function displayEntries() {
    return wrapper!.findAll('.palette-results > *').map((node) => ({
      header: node.classes().includes('palette-group-header'),
      text: node.classes().includes('palette-group-header') ? node.text() : rowTitle(node),
    }))
  }

  it('shows Recent first on an empty All query, most recent first, omitted from their group below', async () => {
    usePaletteRecents().recordRun('feed-desktop')
    usePaletteRecents().recordRun('profile-personal')
    await openPalette()

    expect(displayEntries()).toEqual([
      { header: true, text: 'Recent' },
      { header: false, text: 'Switch to personal' },
      { header: false, text: 'Open desktop feed' },
      { header: true, text: 'Feeds' },
      { header: false, text: 'Open backend feed' },
    ])

    // A Recent row carries its own container prefix, same as a typed-query
    // row does — it is the only way to tell where it lives once it is pulled
    // out from under its own group's header. An ordinary grouped row below
    // still gets its container from the section header instead.
    const rows = wrapper!.findAll('.palette-row')
    expect(rows.map((row) => rowScope(row))).toEqual(['Profiles ›', 'Feeds ›', ''])
  })

  it('drops a stale recent id that no longer resolves to a command', async () => {
    usePaletteRecents().recordRun('feed-does-not-exist')
    usePaletteRecents().recordRun('feed-desktop')
    await openPalette()

    expect(displayEntries()).toEqual([
      { header: true, text: 'Recent' },
      { header: false, text: 'Open desktop feed' },
      { header: true, text: 'Feeds' },
      { header: false, text: 'Open backend feed' },
      { header: true, text: 'Profiles' },
      { header: false, text: 'Switch to personal' },
    ])
  })

  it('hides the Recent section once the query is non-empty', async () => {
    usePaletteRecents().recordRun('feed-desktop')
    await openPalette()

    await wrapper!.find('[data-testid="command-palette-input"]').setValue('open')

    expect(displayEntries().map((entry) => entry.text)).not.toContain('Recent')
  })

  it('hides the Recent section outside the All scope', async () => {
    const palette = useCommandPalette()
    usePaletteRecents().recordRun('feed-desktop')
    await openPalette()

    const actionsTab = tabs().find((tab) => tab.attributes('data-scope') === 'actions')!
    await actionsTab.trigger('click')

    expect(palette.scope.value).toBe('actions')
    expect(displayEntries().map((entry) => entry.text)).not.toContain('Recent')
  })

  // Navigation walks DISPLAY order (Recent hoisted to the top), not `results`
  // order — otherwise opening highlights a mid-list row, arrows zig-zag, and
  // Enter-on-open runs a row the user never saw highlighted.
  it('highlights the top visible row on open, even though it sorts mid-list in results', async () => {
    usePaletteRecents().recordRun('feed-desktop')
    usePaletteRecents().recordRun('profile-personal')
    await openPalette()

    expect(displayEntries()[0]).toEqual({ header: true, text: 'Recent' })
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Switch to personal'])
  })

  it('walks ArrowDown downward in display order, across the Recent/group boundary', async () => {
    usePaletteRecents().recordRun('feed-desktop')
    usePaletteRecents().recordRun('profile-personal')
    await openPalette()

    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Switch to personal'])

    await panel().trigger('keydown', { key: 'ArrowDown' })
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Open desktop feed'])

    // Crosses from Recent into the Feeds group below it.
    await panel().trigger('keydown', { key: 'ArrowDown' })
    expect(selectedRows().map((row) => rowTitle(row))).toEqual(['Open backend feed'])
  })

  it('runs the top Recent row on Enter-on-open', async () => {
    usePaletteRecents().recordRun('feed-desktop')
    usePaletteRecents().recordRun('profile-personal')
    await openPalette()

    await panel().trigger('keydown', { key: 'Enter' })

    expect(runPersonal).toHaveBeenCalledTimes(1)
    expect(runDesktop).not.toHaveBeenCalled()
    expect(runBackend).not.toHaveBeenCalled()
  })
})
