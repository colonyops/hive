import type { Page } from '@playwright/test'
import { expect } from './test.js'

/** Flips an applet's off switch from Settings. */
export async function toggleApplet(page: Page, applet: string, enabled: boolean): Promise<void> {
  await page.goto('/#/settings/applets')
  const toggle = page.getByTestId(`settings-applet-${applet}-enabled`)
  await toggle.click()
  await expect(page.getByTestId(`settings-applet-${applet}-state`)).toHaveCount(enabled ? 0 : 1)
}
