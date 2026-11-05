import type { Page } from '@playwright/test'
import { expect, tag, test } from '../../../fixtures/test.js'

// Reorganizes the e2e/repo space serve.sh seeds. /_e2e/reset puts the space
// back before every test, so drive.spec.ts still finds the plan where it was.

const plan = 'plans/2026-10-11-plan.md'

// The tree renders in a shadow root; CSS locators reach into it. A folder's
// row path ends in a slash.
const row = (page: Page, path: string) =>
  page.getByTestId('drive-page-tree').locator(`[role="treeitem"][data-item-path="${path}"]`)

async function openRepoSpace(page: Page): Promise<void> {
  await page.goto('/')
  await expect(page.getByTestId('feed-item').first()).toBeVisible()
  await page.getByTestId('titlebar-menu').click()
  await page.getByTestId('titlebar-menu-drive').click()
  await page.getByTestId('drive-page-space').click()
  await page.getByTestId('drive-page-space-option-e2e/repo').click()
  await expect(row(page, 'plans/')).toBeVisible()
}

// The reader reveals the document it shows by opening its folders, and which
// document it shows by default can change as the tree does, so a folder is
// clicked open only while the row under it is still hidden.
async function showRow(page: Page, folder: string, path: string): Promise<void> {
  await expect(async () => {
    if (!(await row(page, path).isVisible())) {
      await row(page, `${folder}/`).click()
    }
    await expect(row(page, path)).toBeVisible({ timeout: 1_000 })
  }).toPass()
}

async function rowMenu(page: Page, path: string, action: string): Promise<void> {
  await row(page, path).click({ button: 'right' })
  await page.getByTestId(`drive-page-menu-${action}`).click()
}

test.describe('drive reorganize', { tag: [tag.drive] }, () => {
  test('the Drive tree creates, renames, moves, and deletes without an agent', { tag: [tag.chromiumOnly] }, async ({ page }) => {
    await openRepoSpace(page)

    await page.getByTestId('drive-page-new-folder-open').click()
    await page.getByTestId('drive-page-new-folder-input').fill('inbox')
    await page.getByTestId('drive-page-new-folder-save').click()
    await expect(row(page, 'inbox/')).toHaveAttribute('data-item-type', 'folder')

    await rowMenu(page, 'inbox/', 'rename')
    const renameInput = page.getByTestId('drive-page-tree').locator('[data-item-rename-input]')
    await renameInput.fill('archive')
    await renameInput.press('Enter')
    await expect(row(page, 'archive/')).toBeVisible()
    await expect(row(page, 'inbox/')).toHaveCount(0)

    await showRow(page, 'plans', plan)
    await rowMenu(page, plan, 'move')
    const moveFolder = (path: string) =>
      page.getByTestId('drive-page-move-folders').locator(`[role="treeitem"][data-item-path="${path}"]`)
    await expect(moveFolder('plans/')).toHaveAttribute('aria-selected', 'true')
    await moveFolder('archive/').click()
    await page.getByTestId('drive-page-move-confirm').click()
    await expect(page.getByTestId('drive-page-move')).toHaveCount(0)
    await expect(row(page, plan)).toHaveCount(0)
    await showRow(page, 'archive', 'archive/2026-10-11-plan.md')

    await row(page, 'archive/').click({ button: 'right' })
    await expect(page.getByTestId('drive-page-menu-delete')).toBeDisabled()
    await page.keyboard.press('Escape')

    await rowMenu(page, 'archive/2026-10-11-plan.md', 'delete')
    await page.getByTestId('drive-page-delete-confirmation-confirm').click()
    await expect(row(page, 'archive/2026-10-11-plan.md')).toHaveCount(0)

    await rowMenu(page, 'archive/', 'delete')
    await page.getByTestId('drive-page-delete-confirmation-confirm').click()
    await expect(row(page, 'archive/')).toHaveCount(0)
    await expect(row(page, 'plans/')).toBeVisible()
    await expect(row(page, 'canvases/')).toBeVisible()
  })
})
