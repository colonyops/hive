import { expect, tag, test } from '../../fixtures/test.js'

test.describe('pop-out windows', { tag: [tag.drive] }, () => {
  test('A Drive pop-out boots on its own root without the shell', async ({ page }) => {
    await page.goto('/?popout=drive')
    await expect(page.getByTestId('popout-drive')).toBeVisible()
    await expect(page.getByTestId('drive-page')).toBeVisible()
    await expect(page.getByTestId('drive-page-close')).toHaveCount(0)
    await expect(page.getByTestId('drive-page-pop-out')).toHaveCount(0)

    await page.getByTestId('popout-pin').click()
    await expect(page.getByTestId('popout-pin')).toHaveAttribute('aria-pressed', 'true')
  })

  test('An unknown kind says so rather than falling back to the shell', async ({ page }) => {
    await page.goto('/?popout=nope')
    await expect(page.getByTestId('popout-unknown')).toContainText('nope')
  })
})
