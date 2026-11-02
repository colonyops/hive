import type { Page } from '@playwright/test'
import { expect, tag, test } from '../../../fixtures/test.js'
import { togglePlace } from '../../../fixtures/applets.js'
import { dismissSizeBanner, mockTerminal, windowState } from '../../../fixtures/terminal.js'

async function openSession(page: Page) {
  await mockTerminal(page, {
    sessions: [{ id: 'review', name: 'Review' }],
    windows: [windowState('@1', '%1', 'agent', true)],
  })
  await page.goto('/#/terminal/review')
  await expect(page.getByTestId('panel-tab-window:@1')).toBeVisible()
  await dismissSizeBanner(page)
}

test.describe('applet tabs', { tag: [tag.applets] }, () => {
  test('the + menu has only what the user added, and opens it as a tab or a pop-up', async ({ page }) => {
    await page.goto('/')
    await openSession(page)
    await page.getByTestId('panel-tab-add-p0').click()
    await expect(page.getByTestId('session-add-applet-tab-e2e-page')).toHaveCount(0)
    await page.keyboard.press('Escape')

    await togglePlace(page, 'e2e-page', 'tab')
    await togglePlace(page, 'e2e-page', 'page')
    await openSession(page)

    await page.getByTestId('panel-tab-add-p0').click()
    await page.getByTestId('session-add-applet-page-e2e-page').click()
    await expect(page.getByTestId('applet-frame')).toBeVisible()
    await page.getByTestId('applet-page-close').click()

    await page.getByTestId('panel-tab-add-p0').click()
    await page.getByTestId('session-add-applet-tab-e2e-page').click()
    const frame = page.getByTestId('applet-tab-frame')
    await expect(frame).toBeVisible()
    await expect(page.getByTestId('panel-tab-applet:e2e-page')).toHaveText(/E2E page/)

    // Moving an iframe in the DOM would reload the page and lose this.
    const content = page.frameLocator('[data-testid="applet-tab-frame"]')
    await content.locator('body').evaluate(() => {
      ;(window as unknown as { marker: number }).marker = 1
    })
    await page.getByTestId('panel-split-row-p0').click()
    const panels = page.locator('[data-testid^="panel-tab-row-"]')
    await expect(panels).toHaveCount(2)
    const target = (await panels.nth(1).boundingBox())!
    const tab = (await page.getByTestId('panel-tab-applet:e2e-page').boundingBox())!
    await page.mouse.move(tab.x + tab.width / 2, tab.y + tab.height / 2)
    await page.mouse.down()
    await page.mouse.move(target.x + 40, target.y + target.height / 2, { steps: 10 })
    await page.mouse.up()
    await expect(panels.nth(1).getByTestId('panel-tab-applet:e2e-page')).toBeVisible()
    await expect(panels.nth(0).getByTestId('panel-tab-applet:e2e-page')).toHaveCount(0)
    const box = (await frame.boundingBox())!
    expect(box.x).toBeGreaterThanOrEqual(target.x - 1)
    expect(
      await content.locator('body').evaluate(() => (window as unknown as { marker?: number }).marker),
    ).toBe(1)

    await frame.click()
    await page.keyboard.press('Escape')
    await expect(frame).toBeVisible()

    // A press in a frame never reaches the app's document, so a menu has to notice it another way.
    await panels.nth(1).locator('[data-testid^="panel-tab-add-"]').click()
    await expect(page.getByTestId('new-window-menu')).toBeVisible()
    await frame.click()
    await expect(page.getByTestId('new-window-menu')).toHaveCount(0)

    // A tab that comes forward from behind another takes the keyboard.
    const activeTestId = () => page.evaluate(() => (document.activeElement as HTMLElement | null)?.dataset.testid)
    await panels.nth(1).locator('[data-testid^="panel-tab-add-"]').click()
    await page.getByTestId('new-window-view-tasks').click()
    await expect(frame).toBeHidden()
    await page.getByTestId('panel-tab-applet:e2e-page').click()
    await expect(frame).toBeVisible()
    await expect.poll(activeTestId).toBe('applet-tab-frame')

    await page.getByTestId('panel-tab-close-applet:e2e-page').click({ force: true })
    await expect(frame).toHaveCount(0)

    await togglePlace(page, 'e2e-page', 'tab')
    await togglePlace(page, 'e2e-page', 'page')
  })
})
