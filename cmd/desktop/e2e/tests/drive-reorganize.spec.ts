import { expect, test, type Page } from './fixtures.js'

// Reorganizes the e2e/repo space serve.sh seeds. /_e2e/reset puts the space
// back before every test, so drive.spec.ts still finds the plan where it was.

const plan = 'plans/2026-10-11-plan.md'

async function openRepoSpace(page: Page): Promise<void> {
  await page.goto('/')
  await expect(page.getByTestId('feed-item').first()).toBeVisible()
  await page.getByTestId('titlebar-menu').click()
  await page.getByTestId('titlebar-menu-drive').click()
  await page.getByTestId('drive-page-space').click()
  await page.getByTestId('drive-page-space-option-e2e/repo').click()
  await expect(page.getByTestId('drive-page-row-plans')).toBeVisible()
}

// The reader reveals the document it shows by opening its folders, and which
// document it shows by default can change as the tree does, so a folder is
// clicked open only while the row under it is still hidden.
async function showRow(page: Page, folder: string, path: string): Promise<void> {
  await expect(async () => {
    if (!(await page.getByTestId(`drive-page-row-${path}`).isVisible())) {
      await page.getByTestId(`drive-page-row-${folder}`).click()
    }
    await expect(page.getByTestId(`drive-page-row-${path}`)).toBeVisible({ timeout: 1_000 })
  }).toPass()
}

async function rowMenu(page: Page, path: string, action: string): Promise<void> {
  await page.getByTestId(`drive-page-row-${path}`).hover()
  await page.getByTestId(`drive-page-row-${path}-menu`).click()
  await page.getByTestId(`drive-page-menu-${action}`).click()
}

test('the Drive tree creates, renames, moves, and deletes without an agent', async ({ page }) => {
  await openRepoSpace(page)

  await page.getByTestId('drive-page-new-folder-open').click()
  await page.getByTestId('drive-page-new-folder-input').fill('inbox')
  await page.getByTestId('drive-page-new-folder-save').click()
  await expect(page.getByTestId('drive-page-row-inbox')).toHaveAttribute('data-kind', 'dir')

  await rowMenu(page, 'inbox', 'rename')
  await page.getByTestId('drive-page-rename-input').fill('archive')
  await page.getByTestId('drive-page-rename-input').press('Enter')
  await expect(page.getByTestId('drive-page-row-archive')).toBeVisible()
  await expect(page.getByTestId('drive-page-row-inbox')).toHaveCount(0)

  await showRow(page, 'plans', plan)
  await rowMenu(page, plan, 'move')
  await expect(page.getByTestId('drive-page-move-folder-plans')).toHaveAttribute('aria-selected', 'true')
  await page.getByTestId('drive-page-move-folder-archive').click()
  await page.getByTestId('drive-page-move-confirm').click()
  await expect(page.getByTestId('drive-page-move')).toHaveCount(0)
  await expect(page.getByTestId(`drive-page-row-${plan}`)).toHaveCount(0)
  await showRow(page, 'archive', 'archive/2026-10-11-plan.md')

  await page.getByTestId('drive-page-row-archive').hover()
  await page.getByTestId('drive-page-row-archive-menu').click()
  await expect(page.getByTestId('drive-page-menu-delete')).toBeDisabled()
  await page.keyboard.press('Escape')

  await rowMenu(page, 'archive/2026-10-11-plan.md', 'delete')
  await page.getByTestId('drive-page-delete-confirmation-confirm').click()
  await expect(page.getByTestId('drive-page-row-archive/2026-10-11-plan.md')).toHaveCount(0)

  await rowMenu(page, 'archive', 'delete')
  await page.getByTestId('drive-page-delete-confirmation-confirm').click()
  await expect(page.getByTestId('drive-page-row-archive')).toHaveCount(0)
  await expect(page.getByTestId('drive-page-row-plans')).toBeVisible()
  await expect(page.getByTestId('drive-page-row-canvases')).toBeVisible()
})
