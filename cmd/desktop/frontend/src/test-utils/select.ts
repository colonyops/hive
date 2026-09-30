// Test helpers for driving AppSelect (and anything wrapping it, e.g.
// SelectField). Its popover teleports to <body> to escape scrolling and modal
// ancestors, so it is not reachable through the mounted wrapper — the trigger
// is found on the wrapper, the open list on the document. Keeping that in one
// place means specs read as "open this, choose that" instead of restating the
// teleport every time.
import type { VueWrapper } from '@vue/test-utils'

/** Click the trigger open and return the teleported popover. */
export async function openSelect(wrapper: VueWrapper<any>, testid: string): Promise<HTMLElement> {
  await wrapper.get(`[data-testid="${testid}"]`).trigger('click')
  const popover = document.querySelector<HTMLElement>(`[data-testid="${testid}-popover"]`)
  if (!popover) throw new Error(`select "${testid}" did not open`)
  return popover
}

/** Open the select and click one of its options. */
export async function chooseOption(wrapper: VueWrapper<any>, testid: string, value: string): Promise<void> {
  const popover = await openSelect(wrapper, testid)
  const option = popover.querySelector<HTMLElement>(`[data-testid="${testid}-option-${value}"]`)
  if (!option) throw new Error(`select "${testid}" has no option "${value}"`)
  option.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  await wrapper.vm.$nextTick()
}

/** The label the trigger currently shows. */
export function selectedLabel(wrapper: VueWrapper<any>, testid: string): string {
  return wrapper.get(`[data-testid="${testid}"]`).text()
}
