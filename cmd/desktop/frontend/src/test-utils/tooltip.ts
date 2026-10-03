import type { VueWrapper } from '@vue/test-utils'
import AppTooltip from '../components/ui/AppTooltip.vue'
import IconButton from '../components/ui/IconButton.vue'

/**
 * The tooltip text for the element carrying `testid`: an IconButton's
 * `tooltip` or `label`, or the text of the AppTooltip wrapping it. The text is
 * a component prop, not a `title` attribute an assertion could read directly.
 *
 * Returns '' when nothing gives that testid a tooltip, so a missing tooltip
 * fails the assertion rather than throwing somewhere unhelpful.
 */
// Narrowed to the one method used: VueWrapper is generic in its instance type,
// so concrete wrappers are not assignable to each other.
export function tooltipFor(wrapper: Pick<VueWrapper, 'findAllComponents'>, testid: string): string {
  const button = wrapper
    .findAllComponents(IconButton)
    .find((candidate) => candidate.attributes('data-testid') === testid)
  if (button) return String(button.props('tooltip') ?? button.props('label'))
  const tip = wrapper
    .findAllComponents(AppTooltip)
    .find((candidate) => candidate.find(`[data-testid="${testid}"]`).exists())
  return String(tip?.props('text') ?? '')
}
