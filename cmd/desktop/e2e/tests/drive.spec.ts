import { expect, test } from './fixtures.js'

// serve.sh seeds the e2e/repo space with one plan and one canvas
// (fixtures/drive). The drive is the engine's, not the Chats area's, so it
// works in this server build even though Chats is unavailable here.

const planPath = 'plans/2026-10-11-plan.md'
const planTitle = 'E2E Drive Plan'

test('the Drive overlay opens a document from a space and finds it from another', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('feed-item').first()).toBeVisible()

  await page.getByTestId('titlebar-menu').click()
  await page.getByTestId('titlebar-menu-drive').click()
  await expect(page.getByTestId('drive-overlay')).toBeVisible()

  await page.getByTestId('drive-page-space').click()
  await expect(page.getByTestId('drive-page-space-group-Repositories')).toBeVisible()
  await page.getByTestId('drive-page-space-option-e2e/repo').click()

  await page.getByTestId('drive-page-row-plans').click()
  await page.getByTestId(`drive-page-row-${planPath}`).click()
  await expect(page.getByTestId('drive-page-title')).toHaveText(planTitle)
  await expect(page.getByTestId('drive-page-markdown').getByRole('heading', { name: planTitle })).toBeVisible()
  await expect(page.getByTestId('drive-page-frontmatter-type')).toContainText('plan')

  await page.getByTestId('drive-page-space').click()
  await page.getByTestId('drive-page-space-option-shared').click()
  await expect(page.getByTestId('drive-page-title')).not.toHaveText(planTitle)

  await page.getByTestId('drive-page-search').fill('E2E Drive')
  await page.getByTestId(`drive-page-hit-e2e/repo:${planPath}`).click()
  await expect(page.getByTestId('drive-page-title')).toHaveText(planTitle)

  await page.keyboard.press('Escape')
  await expect(page.getByTestId('drive-overlay')).toHaveCount(0)
})
