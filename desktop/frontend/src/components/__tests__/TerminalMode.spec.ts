import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalMode from '../TerminalMode.vue'

const terminalMocks = vi.hoisted(() => ({
  clear: vi.fn(),
  dispose: vi.fn(),
  fit: vi.fn(),
  focus: vi.fn(),
  loadAddon: vi.fn(),
  open: vi.fn(),
  write: vi.fn(),
}))

vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    clear = terminalMocks.clear
    dispose = terminalMocks.dispose
    focus = terminalMocks.focus
    loadAddon = terminalMocks.loadAddon
    open = terminalMocks.open
    write = terminalMocks.write
  },
}))

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    fit = terminalMocks.fit
  },
}))

describe('TerminalMode', () => {
  beforeEach(() => vi.clearAllMocks())

  it('renders the mock session tree and writes the selected session to xterm', async () => {
    const wrapper = mount(TerminalMode)
    await flushPromises()

    expect(wrapper.find('[data-testid="terminal-mode"]').exists()).toBe(true)
    expect(wrapper.findAll('[role="treeitem"]')).toHaveLength(3)
    expect(wrapper.get('[data-testid="terminal-session-terminal-mode"]').attributes('aria-selected')).toBe('true')
    expect(terminalMocks.open).toHaveBeenCalledOnce()
    expect(terminalMocks.fit).toHaveBeenCalled()
    expect(terminalMocks.focus).toHaveBeenCalledOnce()
    expect(terminalMocks.write.mock.calls.flat().join('')).toContain('terminal-mode-poc')

    await wrapper.get('[data-testid="terminal-session-review"]').trigger('click')

    expect(wrapper.get('[data-testid="terminal-session-review"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.text()).toContain('review-cli-output')
    expect(terminalMocks.clear).toHaveBeenCalled()
    expect(terminalMocks.write.mock.calls.flat().join('')).toContain('review-cli-output')
    expect(terminalMocks.focus).toHaveBeenCalledTimes(2)

    wrapper.unmount()
    expect(terminalMocks.dispose).toHaveBeenCalledOnce()
  })
})
