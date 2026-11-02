import type { Page } from '@playwright/test'
import { expect } from './test.js'

/** Turns one of an applet's places on or off from Settings. */
export async function togglePlace(page: Page, applet: string, surface: string): Promise<void> {
  await page.goto('/#/settings/applets')
  await page.getByTestId(`settings-applet-${applet}-places`).click()
  await page.getByTestId(`settings-applet-${applet}-places-${surface}`).click()
  await page.keyboard.press('Escape')
  await expect(page.getByTestId(`settings-applet-${applet}-places-menu`)).toHaveCount(0)
}
