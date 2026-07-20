import { beforeEach, describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import KeybindingSettingsView from '../KeybindingSettingsView.vue'
import { useKeybindings } from '../../composables/useKeybindings'

const kb = useKeybindings()

beforeEach(() => {
  kb.clearAll()
  kb.recording.value = false
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

  it('filters the list by title, group, or key', async () => {
    const wrapper = mount(KeybindingSettingsView)
    await wrapper.get('[data-testid="keybinding-filter"]').setValue('refresh')

    const rows = wrapper.findAll('[data-testid="keybinding-row"]')
    expect(rows).toHaveLength(1)
    expect(rows[0].attributes('data-command-id')).toBe('feed.refresh')
  })

  it('records a new binding from the next keystroke', async () => {
    const wrapper = mount(KeybindingSettingsView)
    await row(wrapper, 'window.hide').get('[data-testid="keybinding-add"]').trigger('click')
    expect(kb.recording.value).toBe(true)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'h' }))
    await nextTick()

    expect(kb.combosFor('window.hide')).toEqual(['h'])
    expect(kb.recording.value).toBe(false)
    expect(row(wrapper, 'window.hide').text()).toContain('H')
  })

  it('cancels recording on Escape without binding anything', async () => {
    const wrapper = mount(KeybindingSettingsView)
    await row(wrapper, 'window.hide').get('[data-testid="keybinding-add"]').trigger('click')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()

    expect(kb.combosFor('window.hide')).toEqual([])
    expect(kb.recording.value).toBe(false)
    expect(row(wrapper, 'window.hide').find('[data-testid="keybinding-capture"]').exists()).toBe(false)
  })

  it('ignores a lone modifier and keeps waiting for the full combo', async () => {
    const wrapper = mount(KeybindingSettingsView)
    await row(wrapper, 'window.hide').get('[data-testid="keybinding-add"]').trigger('click')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Meta', metaKey: true }))
    await nextTick()
    expect(kb.recording.value).toBe(true) // still capturing

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'p', metaKey: true }))
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
})
