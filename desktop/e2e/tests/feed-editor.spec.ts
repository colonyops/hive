import { expect, test } from '@playwright/test'

// Read-only feed-editor coverage against the shared feed-mode server (8931):
// the editor is opened, inspected, and dismissed — never saved. Mutation
// coverage (create source → create feed → edit feed) lives in
// onboarding.spec.ts, which owns the per-browser mutable servers.

test.beforeEach(async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('feed-item')).toHaveCount(6)
})

test('opens the editor prefilled from the sidebar pencil and closes without mutating', async ({ page }) => {
  const row = page.locator('[data-testid="sidebar-feed"][data-id="my-open-prs"]')
  await row.hover()
  await page.getByTestId('sidebar-feed-edit-my-open-prs').click()

  const editor = page.getByTestId('feed-editor')
  await expect(editor).toBeVisible()
  await expect(page.getByTestId('feed-editor-title')).toHaveText('Edit feed')
  await expect(page.getByTestId('feed-editor-name')).toHaveValue('My open PRs')

  // The canned sources are listed; only the feed's own source is checked.
  await expect(page.getByTestId('feed-editor-source-my-prs')).toBeChecked()
  await expect(page.getByTestId('feed-editor-source-assigned')).not.toBeChecked()
  await expect(page.getByTestId('feed-editor-source-inbox')).not.toBeChecked()

  await expect(page.getByTestId('feed-editor-yaml')).toContainText('id: my-open-prs')
  await expect(page.getByTestId('feed-editor-path')).toHaveText('~/.config/hive/desktop/profiles.yaml')
  await expect(page.getByTestId('feed-editor-copy-prompt')).toBeVisible()

  await page.keyboard.press('Escape')
  await expect(editor).toBeHidden()
  await expect(page.getByTestId('sidebar-feed')).toHaveCount(3)
})

test('opens a create-mode editor from the palette and previews the typed name', async ({ page }) => {
  await page.keyboard.press('Meta+k')
  await page.getByTestId('command-palette-input').fill('new feed')
  await page.getByTestId('command-palette-command').filter({ hasText: 'New feed…' }).click()

  const editor = page.getByTestId('feed-editor')
  await expect(editor).toBeVisible()
  await expect(page.getByTestId('feed-editor-title')).toHaveText('New feed')
  await expect(page.getByTestId('feed-editor-source-my-prs')).toBeVisible()
  await expect(page.getByTestId('feed-editor-source-inbox')).toBeVisible()

  await page.getByTestId('feed-editor-name').fill('Scratch feed')
  await expect(page.getByTestId('feed-editor-yaml')).toContainText('name: Scratch feed')
  // No source checked yet, so the save gate holds.
  await expect(page.getByTestId('feed-editor-save')).toBeDisabled()

  await page.keyboard.press('Escape')
  await expect(editor).toBeHidden()
  await expect(page.getByTestId('sidebar-feed')).toHaveCount(3)
})
