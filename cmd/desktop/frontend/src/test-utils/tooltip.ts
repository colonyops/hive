import type { VueWrapper } from '@vue/test-utils'
import AppTooltip from '../components/AppTooltip.vue'

/**
 * The tooltip text for the element carrying `testid`, read off the AppTooltip
 * wrapping it — the text is a component prop, not a `title` attribute an
 * assertion could read directly.
 *
 * Returns '' when nothing wraps that testid, so a missing tooltip fails the
 * assertion rather than throwing somewhere unhelpful.
 */
// Narrowed to the one method used: VueWrapper is generic in its instance type,
// so concrete wrappers are not assignable to each other.
export function tooltipFor(wrapper: Pick<VueWrapper, 'findAllComponents'>, testid: string): string {
  const tip = wrapper
    .findAllComponents(AppTooltip)
    .find((candidate) => candidate.find(`[data-testid="${testid}"]`).exists())
  return tip?.props('text') ?? ''
}
