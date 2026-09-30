import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { markRaw } from 'vue'
import AppSelect from '../AppSelect.vue'
import { chooseOption, openSelect } from '../../test-utils/select'

const options = [
  { value: 'launch-session', label: 'Launch session' },
  { value: 'shell', label: 'Shell' },
]

const Dot = markRaw({ template: '<i class="dot" />' })
const iconOptions = [
  { value: 'git-branch', label: 'Branch', icon: Dot },
  { value: 'sparkles', label: 'AI / generated', icon: Dot },
  { value: 'bug', label: 'Bugs', icon: Dot },
]

function mountSelect(props: Record<string, unknown> = {}) {
  return mount(AppSelect, { props: { modelValue: 'launch-session', options, testid: 'action-type', ...props } })
}

function vueFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name)
    if (entry.isDirectory()) return vueFiles(path)
    return entry.name.endsWith('.vue') ? [path] : []
  })
}

describe('AppSelect', () => {
  // AppSelect exists because a native picker cannot be themed in the WebKit
  // webview Wails uses — one reintroduced element is a light-mode flash in a
  // dark app, and neither type-checking nor a unit test would notice.
  it('is the only single-select control in the app — no native ones remain', () => {
    const src = join(process.cwd(), 'src')
    const offenders = vueFiles(src).filter((path) => /<select[\s>]/.test(readFileSync(path, 'utf8')))
    expect(offenders.map((path) => path.slice(src.length + 1))).toEqual([])
  })

  it('shows the selected label, opens on click, and emits the chosen value', async () => {
    const wrapper = mountSelect()
    expect(wrapper.get('[data-testid="action-type"]').text()).toContain('Launch session')
    expect(document.querySelector('[data-testid="action-type-popover"]')).toBeNull()

    await chooseOption(wrapper, 'action-type', 'shell')

    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['shell'])
    expect(document.querySelector('[data-testid="action-type-popover"]')).toBeNull()
    wrapper.unmount()
  })

  // The trigger is often narrower than its longest option (DevView's severity
  // picker is 92px wide, "success" is not), and a list that truncates to
  // "succ…" is unreadable.
  it('lets the list outgrow the trigger rather than truncating a long label', async () => {
    const wrapper = mountSelect()
    const popover = await openSelect(wrapper, 'action-type')

    expect(popover.style.minWidth).toBe('0px') // the trigger's width, zero in a layout-less DOM
    expect(popover.style.width).toBe('')
    expect(Number.parseFloat(popover.style.maxWidth)).toBeGreaterThan(0)
    wrapper.unmount()
  })

  // A check reserved on every row indents every label for the sake of one.
  it('marks the selected row with a trailing check, leaving the others ungutted', async () => {
    const wrapper = mountSelect()
    const popover = await openSelect(wrapper, 'action-type')
    const rows = Array.from(popover.querySelectorAll('[role="option"] button'))

    expect(rows.map((row) => row.querySelectorAll('svg').length)).toEqual([1, 0])
    expect(rows[0].lastElementChild?.tagName.toLowerCase()).toBe('svg') // trailing, not leading
    wrapper.unmount()
  })

  it('teleports the popover to the document body so scrolling and modal ancestors cannot clip it', async () => {
    const wrapper = mountSelect()
    const popover = await openSelect(wrapper, 'action-type')

    expect(popover.parentElement).toBe(document.body)
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    expect(popover.querySelector('[role="listbox"]')).not.toBeNull()

    wrapper.unmount()
    expect(document.querySelector('[data-testid="action-type-popover"]')).toBeNull()
  })

  it('does not re-emit when the current value is re-selected', async () => {
    const wrapper = mountSelect({ modelValue: 'shell' })
    await chooseOption(wrapper, 'action-type', 'shell')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    wrapper.unmount()
  })

  it('selects the active option with the keyboard', async () => {
    const wrapper = mountSelect()
    const root = wrapper.get('[data-testid="action-type"]')
    await root.trigger('keydown', { key: 'ArrowDown' }) // open
    await root.trigger('keydown', { key: 'ArrowDown' }) // move to shell
    await root.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['shell'])
    wrapper.unmount()
  })

  it('walks to the list edges with Home and End', async () => {
    const wrapper = mountSelect({ modelValue: 'launch-session', options: iconOptions })
    const root = wrapper.get('[data-testid="action-type"]')
    await root.trigger('keydown', { key: 'ArrowDown' }) // open
    await root.trigger('keydown', { key: 'End' })
    await root.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['bug'])
    wrapper.unmount()
  })

  it('closes on Escape without emitting', async () => {
    const wrapper = mountSelect()
    const root = wrapper.get('[data-testid="action-type"]')
    await root.trigger('keydown', { key: 'ArrowDown' })
    expect(document.querySelector('[data-testid="action-type-popover"]')).not.toBeNull()

    await root.trigger('keydown', { key: 'Escape' })
    expect(document.querySelector('[data-testid="action-type-popover"]')).toBeNull()
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    wrapper.unmount()
  })

  describe('placeholder', () => {
    it('shows the placeholder when the value matches no option', () => {
      const wrapper = mountSelect({ modelValue: '', placeholder: 'Choose one' })
      const trigger = wrapper.get('[data-testid="action-type"]')
      expect(trigger.text()).toContain('Choose one')
      expect(trigger.get('span').classes()).toContain('text-text-4')
      wrapper.unmount()
    })

    it('prefers a real option over the placeholder, including one with an empty value', () => {
      const wrapper = mountSelect({
        modelValue: '',
        placeholder: 'Choose one',
        options: [{ value: '', label: 'Use action default' }, ...options],
      })
      expect(wrapper.get('[data-testid="action-type"]').text()).toContain('Use action default')
      wrapper.unmount()
    })

    it('renders an empty trigger when no placeholder is given', () => {
      const wrapper = mountSelect({ modelValue: 'gone' })
      expect(wrapper.get('[data-testid="action-type"]').text()).toBe('')
      wrapper.unmount()
    })
  })

  describe('disabled', () => {
    it('does not open a disabled control', async () => {
      const wrapper = mountSelect({ disabled: true })
      const trigger = wrapper.get<HTMLButtonElement>('[data-testid="action-type"]')
      expect(trigger.element.disabled).toBe(true)

      await trigger.trigger('click')
      expect(document.querySelector('[data-testid="action-type-popover"]')).toBeNull()
      wrapper.unmount()
    })

    it('ignores clicks on a disabled option', async () => {
      const wrapper = mountSelect({ options: [options[0], { ...options[1], disabled: true }] })
      await chooseOption(wrapper, 'action-type', 'shell')
      expect(wrapper.emitted('update:modelValue')).toBeUndefined()
      wrapper.unmount()
    })

    it('skips disabled options when arrowing', async () => {
      const wrapper = mountSelect({
        modelValue: 'launch-session',
        options: [options[0], { value: 'skip-me', label: 'Skip me', disabled: true }, options[1]],
      })
      const root = wrapper.get('[data-testid="action-type"]')
      await root.trigger('keydown', { key: 'ArrowDown' }) // open
      await root.trigger('keydown', { key: 'ArrowDown' }) // past the disabled row, onto shell
      await root.trigger('keydown', { key: 'Enter' })
      expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['shell'])
      wrapper.unmount()
    })
  })

  describe('size', () => {
    const classesFor = (size?: string) => {
      const wrapper = mountSelect(size ? { size } : {})
      const classes = wrapper.get('[data-testid="action-type"]').classes()
      wrapper.unmount()
      return classes
    }

    it('defaults to the form-field metrics TextField uses', () => {
      expect(classesFor()).toEqual(expect.arrayContaining(['rounded-lg', 'px-3', 'py-2.5', 'text-[13.5px]']))
      expect(classesFor()).toEqual(classesFor('md'))
    })

    it('shrinks to toolbar metrics at sm', () => {
      expect(classesFor('sm')).toEqual(expect.arrayContaining(['rounded-md', 'px-2', 'py-1.5', 'text-[11px]']))
      expect(classesFor('sm')).not.toContain('text-[13.5px]')
    })
  })

  describe('searchable', () => {
    const mountSearchable = (props: Record<string, unknown> = {}) =>
      mount(AppSelect, { props: { modelValue: 'git-branch', options: iconOptions, searchable: true, testid: 'icon', ...props } })

    it('filters options by the search query', async () => {
      const wrapper = mountSearchable()
      const popover = await openSelect(wrapper, 'icon')
      const search = popover.querySelector<HTMLInputElement>('[data-testid="icon-search"]')!
      search.value = 'bug'
      search.dispatchEvent(new Event('input', { bubbles: true }))
      await wrapper.vm.$nextTick()

      expect(popover.querySelector('[data-testid="icon-option-bug"]')).not.toBeNull()
      expect(popover.querySelector('[data-testid="icon-option-sparkles"]')).toBeNull()
      wrapper.unmount()
    })

    it('shows an empty state when nothing matches', async () => {
      const wrapper = mountSearchable()
      const popover = await openSelect(wrapper, 'icon')
      const search = popover.querySelector<HTMLInputElement>('[data-testid="icon-search"]')!
      search.value = 'zzz'
      search.dispatchEvent(new Event('input', { bubbles: true }))
      await wrapper.vm.$nextTick()

      expect(popover.querySelector('[role="listbox"]')).toBeNull()
      expect(popover.querySelector('[data-testid="icon-empty"]')?.textContent).toContain('No matches')
      wrapper.unmount()
    })

    it('selects the keyboard-active option from the filtered list', async () => {
      const wrapper = mountSearchable()
      const popover = await openSelect(wrapper, 'icon')
      const search = popover.querySelector<HTMLInputElement>('[data-testid="icon-search"]')!
      search.value = 'b' // Branch, Bugs
      search.dispatchEvent(new Event('input', { bubbles: true }))
      await wrapper.vm.$nextTick()
      search.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true })) // active -> Bugs
      search.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
      await wrapper.vm.$nextTick()

      expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['bug'])
      wrapper.unmount()
    })

    it('renders no search box when searchable is off', async () => {
      const wrapper = mountSearchable({ searchable: false })
      const popover = await openSelect(wrapper, 'icon')
      expect(popover.querySelector('[data-testid="icon-search"]')).toBeNull()
      wrapper.unmount()
    })
  })

  describe('editable (combobox)', () => {
    function mountEditable(props: Record<string, unknown> = {}) {
      return mount(AppSelect, { props: { modelValue: '', options, testid: 'repo', editable: true, ...props } })
    }

    it('renders a text input showing the model value', () => {
      const wrapper = mountEditable({ modelValue: 'https://example.com/x.git' })
      const input = wrapper.get('[data-testid="repo"]')
      expect(input.element.tagName).toBe('INPUT')
      expect((input.element as HTMLInputElement).value).toBe('https://example.com/x.git')
      wrapper.unmount()
    })

    it('shows the whole list on focus even when a value is prefilled', async () => {
      const wrapper = mountEditable({ modelValue: 'launch-session' })
      await wrapper.get('[data-testid="repo"]').trigger('focus')
      const popover = document.querySelector('[data-testid="repo-popover"]')!
      expect(popover.querySelectorAll('[role="option"]').length).toBe(options.length)
      wrapper.unmount()
    })

    it('filters by label or value as you type and emits the raw text', async () => {
      const wrapper = mountEditable()
      const input = wrapper.get('[data-testid="repo"]')
      await input.trigger('focus')
      await input.setValue('she')
      expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['she'])
      const popover = document.querySelector('[data-testid="repo-popover"]')!
      expect(Array.from(popover.querySelectorAll('[role="option"]')).map((o) => o.textContent?.trim())).toEqual(['Shell'])
      wrapper.unmount()
    })

    it('selects an option without needing to clear the input', async () => {
      const wrapper = mountEditable({ modelValue: 'launch-session' })
      await wrapper.get('[data-testid="repo"]').trigger('focus')
      const option = document.querySelector<HTMLElement>('[data-testid="repo-option-shell"]')!
      option.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await wrapper.vm.$nextTick()
      expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['shell'])
      expect(document.querySelector('[data-testid="repo-popover"]')).toBeNull()
      wrapper.unmount()
    })

    it('accepts a free-form value that is not one of the options', async () => {
      const wrapper = mountEditable()
      const input = wrapper.get('[data-testid="repo"]')
      await input.trigger('focus')
      await input.setValue('git@github.com:acme/site.git')
      expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['git@github.com:acme/site.git'])
      const popover = document.querySelector('[data-testid="repo-popover"]')!
      expect(popover.querySelector('[data-testid="repo-empty"]')).not.toBeNull()
      wrapper.unmount()
    })
  })
})
