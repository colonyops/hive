import { expect, test } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('feed-item')).toHaveCount(6)
})

test('opens the feeds-as-code sheet from the sidebar and closes it', async ({ page }) => {
  await page.getByTestId('sidebar-edit-feeds').click()

  const sheet = page.getByTestId('config-sheet')
  await expect(sheet).toBeVisible()
  await expect(page.getByTestId('config-sheet-path')).toHaveText('~/.config/hive/desktop/profiles.yaml')
  await expect(page.getByTestId('config-sheet-yaml')).toContainText('id: my-open-prs')
  await expect(page.getByTestId('config-sheet-valid')).toContainText('changes apply live')
  await expect(page.getByTestId('config-sheet-copy-prompt')).toBeVisible()

  await page.keyboard.press('Escape')
  await expect(sheet).toBeHidden()
})

test('opens the feeds-as-code sheet from the command palette', async ({ page }) => {
  await page.keyboard.press('Meta+k')
  const input = page.getByTestId('command-palette-input')
  await input.fill('as code')
  await expect(page.getByTestId('command-palette-command')).toHaveText('Edit feeds as code…')
  await input.press('Enter')

  await expect(page.getByTestId('config-sheet')).toBeVisible()
})

// Profile creation is a one-way state change on the shared feed-mode
// server, so it lives in onboarding.spec.ts, which owns per-browser
// mutable servers. This suite only opens read-only surfaces.
test('opens and cancels the new-profile modal without changing the rail', async ({ page }) => {
  await page.getByTestId('profile-add').click()

  const modal = page.getByTestId('new-profile-modal')
  await expect(modal).toBeVisible()
  await expect(page.getByTestId('new-profile-submit')).toBeDisabled()

  await page.keyboard.press('Escape')
  await expect(modal).toBeHidden()
  await expect(page.getByTestId('profile-tile')).toHaveCount(1)
})
