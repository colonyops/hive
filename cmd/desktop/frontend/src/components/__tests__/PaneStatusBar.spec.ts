import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import PaneStatusBar from '../PaneStatusBar.vue'
import { tooltipFor } from '../../test-utils/tooltip'

function mountBar(props: Record<string, unknown> = {}, slots: Record<string, string> = {}) {
  return mount(PaneStatusBar, { props: { testid: 'pane-statusbar', label: 'Web App', ...props }, slots })
}

describe('PaneStatusBar', () => {
  it('names the directory and hangs the full path off the tooltip', () => {
    const wrapper = mountBar({ path: '/home/hayden/workspaces/web-app' })
    const name = wrapper.get('[data-testid="pane-statusbar-workspace"]')
    expect(name.text()).toBe('Web App')
    expect(name.attributes('title')).toBe('/home/hayden/workspaces/web-app')
  })

  // The Code view drops it: its sidebar names the session already, so the row
  // would spend its left edge repeating what is beside it.
  it('drops the name and its folder entirely when no label is given', () => {
    const wrapper = mountBar({ label: '' })
    expect(wrapper.find('[data-testid="pane-statusbar-workspace"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="pane-statusbar-reveal"]').exists()).toBe(true)
  })

  // The two areas prefix their own ids, which is what lets each keep the test
  // surface it had before the bar was shared.
  it('prefixes every test id with the one it was given', () => {
    const wrapper = mountBar({ testid: 'terminal-pane-statusbar', editorTitle: 'Zed', error: 'nope' })
    for (const suffix of ['', '-workspace', '-error', '-open-editor', '-reveal']) {
      expect(wrapper.find(`[data-testid="terminal-pane-statusbar${suffix}"]`).exists()).toBe(true)
    }
  })

  it('hides the editor button when none is configured, and keeps reveal', () => {
    const wrapper = mountBar()
    expect(wrapper.find('[data-testid="pane-statusbar-open-editor"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="pane-statusbar-reveal"]').exists()).toBe(true)
  })

  it('labels the editor button with the configured editor', () => {
    const wrapper = mountBar({ editorTitle: 'VS Code' })
    expect(tooltipFor(wrapper, 'pane-statusbar-open-editor')).toBe('Open in VS Code')
    expect(wrapper.get('[data-testid="pane-statusbar-open-editor"]').attributes('aria-label')).toBe('Open in VS Code')
  })

  it('emits rather than acting, since each area reaches its backend differently', async () => {
    const wrapper = mountBar({ editorTitle: 'Zed' })
    await wrapper.get('[data-testid="pane-statusbar-open-editor"]').trigger('click')
    await wrapper.get('[data-testid="pane-statusbar-reveal"]').trigger('click')
    expect(wrapper.emitted('open-editor')).toHaveLength(1)
    expect(wrapper.emitted('reveal')).toHaveLength(1)
  })

  it('renders area-specific status in the slot', () => {
    const wrapper = mountBar({}, { default: '<span data-testid="chips">feat/parser</span>' })
    expect(wrapper.get('[data-testid="chips"]').text()).toBe('feat/parser')
  })

  // Optional: an area with nothing to add there (Agents) renders the row
  // exactly as before, so this slot cannot be a breaking change for it.
  it('renders nothing extra in the right-hand cluster when no actions slot is given', () => {
    const wrapper = mountBar({ editorTitle: 'Zed' })
    const bar = wrapper.get('[data-testid="pane-statusbar"]')
    expect(bar.findAll('button')).toHaveLength(2)
  })

  it('renders an area-specific action after open-editor and reveal', () => {
    const wrapper = mountBar(
      { editorTitle: 'Zed' },
      { actions: '<button type="button" data-testid="extra-action">Extra</button>' },
    )
    const buttons = wrapper.get('[data-testid="pane-statusbar"]').findAll('button')
    expect(buttons.at(-1)?.attributes('data-testid')).toBe('extra-action')
  })
})
