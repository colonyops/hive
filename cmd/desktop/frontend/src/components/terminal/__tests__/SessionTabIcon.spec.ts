import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useTerminalAgentIcons } from '../../../stores/useTerminalAgentIcons'
import SessionTabIcon from '../SessionTabIcon.vue'

vi.mock('../../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: vi.fn().mockResolvedValue({ terminalMonochromeAgentIcons: false }),
  SetTerminalMonochromeAgentIcons: vi.fn().mockResolvedValue(undefined),
}))

describe('SessionTabIcon', () => {
  beforeEach(async () => {
    await flushPromises()
    useTerminalAgentIcons().setMonochrome(false)
    await flushPromises()
  })

  it.each(['claude', 'codex', 'pi'] as const)('draws %s in full color by default', (agent) => {
    const wrapper = mount(SessionTabIcon, { props: { kind: 'window', agent } })
    expect(wrapper.find('img').attributes('data-agent-icon')).toBe(agent)
    expect(wrapper.find('svg').exists()).toBe(false)
  })

  // No image and no fill of its own: currentColor is what makes it match the row.
  it.each(['claude', 'codex', 'pi'] as const)('draws %s in the current color when monochrome', async (agent) => {
    useTerminalAgentIcons().setMonochrome(true)
    await flushPromises()
    const wrapper = mount(SessionTabIcon, { props: { kind: 'window', agent } })
    expect(wrapper.find('img').exists()).toBe(false)
    const svg = wrapper.get('svg')
    expect(svg.attributes('data-agent-icon')).toBe(agent)
    expect(svg.attributes('fill')).toBe('currentColor')
    expect(wrapper.html()).not.toMatch(/#[0-9a-f]{3,6}|url\(/i)
  })

  it('leaves a plain terminal alone', async () => {
    useTerminalAgentIcons().setMonochrome(true)
    await flushPromises()
    const wrapper = mount(SessionTabIcon, { props: { kind: 'window' } })
    expect(wrapper.find('[data-agent-icon]').exists()).toBe(false)
    expect(wrapper.find('svg').exists()).toBe(true)
  })
})
