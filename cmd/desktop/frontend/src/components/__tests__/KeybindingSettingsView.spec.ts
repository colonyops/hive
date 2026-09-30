import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import KeybindingSettingsView, { RECORDER_COMMIT_MS } from '../KeybindingSettingsView.vue'
import { useKeybindings } from '../../composables/useKeybindings'
import { requestedEditorFilter } from '../../keybindings/keymapRows'

const kb = useKeybindings()

beforeEach(() => {
  kb.clearAll()
  kb.recording.value = false
  requestedEditorFilter.value = null
})

afterEach(() => {
  vi.useRealTimers()
})

function row(wrapper: ReturnType<typeof mount>, id: string) {
  return wrapper.get(`[data-command-id="${id}"]`)
}

describe('KeybindingSettingsView', () => {
  it('lists commands grouped, showing default combos as formatted chips', () => {
    const wrapper = mount(KeybindingSettingsView)
    const next = row(wrapper, 'feed.next')
    const chips = next.findAll('[data-testid="keybinding-combo"]').map((c) => c.text())
    expect(chips.some((t) => t.includes('J'))).toBe(true)
    expect(chips.some((t) => t.includes('↓'))).toBe(true)
  })

  it('applies a requested filter from the ? scope handshake on mount, then clears it', async () => {
    requestedEditorFilter.value = 'Next item'

    const wrapper = mount(KeybindingSettingsView)
    await nextTick()

    expect((wrapper.get('[data-testid="keybinding-filter"]').element as HTMLInputElement).value).toBe('Next item')
    expect(requestedEditorFilter.value).toBeNull()
    expect(wrapper.findAll('[data-testid="keybinding-row"]').map((r) => r.attributes('data-command-id')))
      .toEqual(['feed.next'])
  })

  // A Keys-row run while already on Settings › Keyboard pushes the same route
  // without remounting the view (no v-else-if :key), so the handshake must
  // also apply on a later change to the request, not just on mount.
  it('applies a requested filter set after mount, then clears it', async () => {
    const wrapper = mount(KeybindingSettingsView)
    await nextTick()

    requestedEditorFilter.value = 'Next item'
    await nextTick()

    expect((wrapper.get('[data-testid="keybinding-filter"]').element as HTMLInputElement).value).toBe('Next item')
    expect(requestedEditorFilter.value).toBeNull()
    expect(wrapper.findAll('[data-testid="keybinding-row"]').map((r) => r.attributes('data-command-id')))
      .toEqual(['feed.next'])
  })

  it('filters the list by title, group, or key', async () => {
    const wrapper = mount(KeybindingSettingsView)
    await wrapper.get('[data-testid="keybinding-filter"]').setValue('refresh')

    const rows = wrapper.findAll('[data-testid="keybinding-row"]')
    expect(rows).toHaveLength(1)
    expect(rows[0].attributes('data-command-id')).toBe('feed.refresh')
  })

  it('records a single chord as before, once the pause elapses (regression)', async () => {
    vi.useFakeTimers()
    const wrapper = mount(KeybindingSettingsView)
    await row(wrapper, 'window.hide').get('[data-testid="keybinding-add"]').trigger('click')
    expect(kb.recording.value).toBe(true)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'h' }))
    await nextTick()
    expect(kb.combosFor('window.hide')).toEqual([]) // pending, not yet committed
    expect(kb.recording.value).toBe(true)

    vi.advanceTimersByTime(RECORDER_COMMIT_MS)
    await nextTick()

    expect(kb.combosFor('window.hide')).toEqual(['h'])
    expect(kb.recording.value).toBe(false)
    expect(row(wrapper, 'window.hide').text()).toContain('H')
  })

  it('captures a two-step sequence and commits it as one binding on the pause', async () => {
    vi.useFakeTimers()
    const wrapper = mount(KeybindingSettingsView)
    await row(wrapper, 'window.hide').get('[data-testid="keybinding-add"]').trigger('click')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
    await nextTick()
    expect(kb.combosFor('window.hide')).toEqual([]) // first step pending
    expect(row(wrapper, 'window.hide').get('[data-testid="keybinding-capture"]').text()).toContain('G')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'x' }))
    await nextTick()
    expect(kb.combosFor('window.hide')).toEqual([]) // second step still pending

    vi.advanceTimersByTime(RECORDER_COMMIT_MS)
    await nextTick()

    expect(kb.combosFor('window.hide')).toEqual(['g x'])
    expect(kb.recording.value).toBe(false)
  })

  it('clicking the capture chip commits immediately, without waiting for the pause', async () => {
    vi.useFakeTimers()
    const wrapper = mount(KeybindingSettingsView)
    await row(wrapper, 'window.hide').get('[data-testid="keybinding-add"]').trigger('click')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
    await nextTick()

    await row(wrapper, 'window.hide').get('[data-testid="keybinding-capture"]').trigger('click')

    expect(kb.combosFor('window.hide')).toEqual(['g'])
    expect(kb.recording.value).toBe(false)
  })

  it('cancels recording on Escape mid-sequence without binding anything', async () => {
    vi.useFakeTimers()
    const wrapper = mount(KeybindingSettingsView)
    await row(wrapper, 'window.hide').get('[data-testid="keybinding-add"]').trigger('click')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
    await nextTick()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()

    expect(kb.combosFor('window.hide')).toEqual([])
    expect(kb.recording.value).toBe(false)
    expect(row(wrapper, 'window.hide').find('[data-testid="keybinding-capture"]').exists()).toBe(false)
  })

  it('ignores a lone modifier and keeps waiting for the full combo', async () => {
    vi.useFakeTimers()
    const wrapper = mount(KeybindingSettingsView)
    await row(wrapper, 'window.hide').get('[data-testid="keybinding-add"]').trigger('click')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Meta', metaKey: true }))
    await nextTick()
    expect(kb.recording.value).toBe(true) // still capturing

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'p', metaKey: true }))
    await nextTick()
    vi.advanceTimersByTime(RECORDER_COMMIT_MS)
    await nextTick()

    expect(kb.combosFor('window.hide')).toEqual(['mod+p'])
    expect(kb.recording.value).toBe(false)
  })

  it('removes an individual binding', async () => {
    const wrapper = mount(KeybindingSettingsView)
    await row(wrapper, 'feed.next').get('[data-testid="keybinding-remove"]').trigger('click')
    expect(kb.combosFor('feed.next')).toEqual(['arrowdown']) // removed the first chip (j)
  })

  it('shows and applies reset-to-default once overridden', async () => {
    const wrapper = mount(KeybindingSettingsView)
    kb.removeBinding('feed.next', 'j')
    await nextTick()

    const reset = row(wrapper, 'feed.next').get('[data-testid="keybinding-reset"]')
    await reset.trigger('click')

    expect(kb.combosFor('feed.next')).toEqual(['j', 'arrowdown'])
    expect(row(wrapper, 'feed.next').find('[data-testid="keybinding-reset"]').exists()).toBe(false)
  })

  it('flags a combo bound to more than one command', async () => {
    const wrapper = mount(KeybindingSettingsView)
    kb.addBinding('feed.refresh', 'j') // now j is on both feed.next and feed.refresh
    await nextTick()

    const conflictChips = wrapper.findAll('[data-testid="keybinding-combo"].combo-conflict')
    expect(conflictChips.length).toBeGreaterThanOrEqual(2)
  })

  // The Zed rule (ADR keybindings-are-chord-sequences-not-a-leader-key): a
  // binding that only prefixes another stays fully functional, so recording
  // one is not a conflict — only an exact duplicate binding is.
  it('does not flag a recorded sequence whose first step equals an existing single-key binding', async () => {
    vi.useFakeTimers()
    const wrapper = mount(KeybindingSettingsView)
    // feed.next binds 'j' by default; window.hide records 'j x', prefixed by it.
    await row(wrapper, 'window.hide').get('[data-testid="keybinding-add"]').trigger('click')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j' }))
    await nextTick()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'x' }))
    await nextTick()
    vi.advanceTimersByTime(RECORDER_COMMIT_MS)
    await nextTick()

    expect(kb.combosFor('window.hide')).toEqual(['j x'])
    expect(wrapper.findAll('[data-testid="keybinding-combo"].combo-conflict')).toHaveLength(0)
  })

  it('flags a recorded sequence that exactly duplicates an existing binding', async () => {
    vi.useFakeTimers()
    const wrapper = mount(KeybindingSettingsView)
    // view.go-inbox binds 'g i' by default; record the identical sequence onto
    // a second command.
    await row(wrapper, 'window.hide').get('[data-testid="keybinding-add"]').trigger('click')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'g' }))
    await nextTick()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'i' }))
    await nextTick()
    vi.advanceTimersByTime(RECORDER_COMMIT_MS)
    await nextTick()

    expect(kb.combosFor('window.hide')).toEqual(['g i'])
    const conflictChips = wrapper.findAll('[data-testid="keybinding-combo"].combo-conflict')
    expect(conflictChips.length).toBeGreaterThanOrEqual(2)
  })
})
