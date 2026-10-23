import { screenshotPath } from '../../fixtures/paths.js'
import { expect, tag, test } from '../../fixtures/test.js'

test.describe('diagnostics', { tag: [tag.diagnostics, tag.critical] }, () => {
  test('Diagnostics opens independently and retains evidence when investigation is unavailable', async ({ page, browserName }) => {
    await page.goto('/?diagnostics=1')
    await expect(page.getByTestId('diagnostics-window')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Diagnostics', exact: true })).toBeVisible()
    await expect(page.getByTestId('diagnostics-source-log')).toContainText('hive.log')
    await expect(page.getByTestId('diagnostics-terminal-panel')).toHaveCount(0)

    await page.getByTestId('diagnostics-live-follow').click()
    await page.getByTestId('diagnostics-time-range-toggle').click()
    await page.getByRole('button', { name: 'Retained history' }).click()
    await page.getByTestId('diagnostics-incident-description').fill('A new window never appeared.')
    await page.getByTestId('diagnostics-investigate').click()
    await expect(page.getByTestId('diagnostics-agent-error')).toBeVisible()
    await expect(page.getByTestId('diagnostics-entries')).toBeVisible()

    await page.getByTestId('diagnostics-actions').click()
    await page.getByRole('menuitem', { name: 'Export snapshot' }).click()
    await expect(page.getByRole('status')).toContainText('Saved')
    await page.screenshot({ path: screenshotPath(`diagnostics-${browserName}.png`), fullPage: true })
  })
})
