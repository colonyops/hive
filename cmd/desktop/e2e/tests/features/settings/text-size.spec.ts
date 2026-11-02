import type { Page } from '@playwright/test'
import { screenshotPath } from '../../../fixtures/paths.js'
import { expect, tag, test } from '../../../fixtures/test.js'

const rootFontPx = (page: Page): Promise<number> =>
  page.evaluate(() => parseFloat(getComputedStyle(document.documentElement).fontSize))

const stepper = (page: Page, surface: string, part: 'value' | 'increase' | 'decrease') =>
  page.getByTestId(`settings-appearance-scale-${surface}-stepper-${part}`)

async function stepInterfaceTo(page: Page, percent: string, part: 'increase' | 'decrease'): Promise<void> {
  const value = stepper(page, 'interface', 'value')
  while ((await value.textContent()) !== percent) await stepper(page, 'interface', part).click()
}

// The feed server is shared and these persist, so each test leaves 100% behind.
test.describe('text size', { tag: [tag.settings] }, () => {
  test.afterEach(async ({ page }) => {
    await page.goto('/#/settings/appearance')
    const value = stepper(page, 'interface', 'value')
    await expect(value).toBeVisible()
    const current = parseInt((await value.textContent()) ?? '100', 10)
    if (current !== 100) await stepInterfaceTo(page, '100%', current > 100 ? 'decrease' : 'increase')
  })

  test('the interface scale reflows the chrome without touching other surfaces', async ({ page, browserName }) => {
    await page.goto('/#/settings/appearance')
    await expect(stepper(page, 'interface', 'value')).toHaveText('100%')
    expect(await rootFontPx(page)).toBe(16)

    const row = page.getByTestId('settings-appearance-scale-interface')
    const before = await row.boundingBox()

    await stepInterfaceTo(page, '150%', 'increase')
    await expect.poll(() => rootFontPx(page)).toBe(24)

    const after = await row.boundingBox()
    expect(after!.height).toBeGreaterThan(before!.height)
    await expect(stepper(page, 'terminal', 'value')).toHaveText('100%')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    await stepper(page, 'interface', 'value').scrollIntoViewIfNeeded()
    await page.screenshot({ path: screenshotPath(`text-size-settings-150-${browserName}.png`) })

    await page.goto('/#/')
    await expect(page.locator('body')).toBeVisible()
    await page.screenshot({ path: screenshotPath(`text-size-feed-150-${browserName}.png`) })

    await page.goto('/#/settings/appearance')
    await stepInterfaceTo(page, '75%', 'decrease')
    await expect.poll(() => rootFontPx(page)).toBe(12)
    await page.screenshot({ path: screenshotPath(`text-size-settings-75-${browserName}.png`) })
    await page.goto('/#/')
    await page.screenshot({ path: screenshotPath(`text-size-feed-75-${browserName}.png`) })
  })

  test('the chords step the interface away from a terminal', async ({ page }) => {
    await page.goto('/#/settings/appearance')
    await expect(stepper(page, 'interface', 'value')).toHaveText('100%')
    await page.getByTestId('settings-appearance').click({ position: { x: 4, y: 4 } })

    await page.keyboard.press('Control+=')
    await expect(stepper(page, 'interface', 'value')).toHaveText('110%')
    await expect.poll(() => rootFontPx(page)).toBeCloseTo(17.6)

    await page.keyboard.press('Control+0')
    await expect(stepper(page, 'interface', 'value')).toHaveText('100%')
  })

  test('the scale survives a reload', async ({ page }) => {
    await page.goto('/#/settings/appearance')
    await stepper(page, 'interface', 'increase').click()
    await expect.poll(() => rootFontPx(page)).toBeCloseTo(17.6)

    await page.reload()
    await expect.poll(() => rootFontPx(page)).toBeCloseTo(17.6)
    await expect(stepper(page, 'interface', 'value')).toHaveText('110%')
  })
})
