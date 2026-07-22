import { expect, test } from '@playwright/test'

test('persists notification preferences from application settings', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('feed-item')).toHaveCount(6)

  await page.getByTestId('application-settings').click()
  await page.getByTestId('settings-category-notifications').click()
  await expect(page.getByTestId('notification-settings')).toBeVisible()

  const master = page.getByTestId('notification-enable')
  await expect(master).toHaveAttribute('aria-checked', 'true')
  await expect(page.getByTestId('notification-system')).toBeVisible()
  await expect(page.getByTestId('notification-sound')).toBeVisible()

  await master.click()
  await expect(master).toHaveAttribute('aria-checked', 'false')
  await expect(page.getByTestId('notification-system')).toBeDisabled()

  await page.reload()
  await expect(page.getByTestId('notification-settings')).toBeVisible()
  await expect(page.getByTestId('notification-enable')).toHaveAttribute('aria-checked', 'false')

  // E2E specs share the server configuration, so restore the default before
  // later notification-action tests run.
  await page.getByTestId('notification-enable').click()
  await expect(page.getByTestId('notification-enable')).toHaveAttribute('aria-checked', 'true')
  await page.reload()
  await expect(page.getByTestId('notification-enable')).toHaveAttribute('aria-checked', 'true')
})

test('records focused profile rename feedback in both toast and Activity', async ({ page }) => {
  await page.goto('/')
  const originalName = await page.getByTestId('sidebar-profile-name').textContent()
  expect(originalName).toBeTruthy()

  await page.getByTestId('titlebar-activity').click()
  await expect(page.getByTestId('activity-view')).toBeVisible()
  const beforeRows = await page.getByTestId('activity-row').count()
  await page.getByTestId('activity-close').click()

  const renamedName = `${originalName} notifications`
  await page.getByTestId('sidebar-open-settings').click()
  await page.getByTestId('profile-settings-name').fill(renamedName)
  await page.getByTestId('profile-settings-save-name').click()
  await expect(page.getByTestId('toast').last()).toContainText('Profile renamed')

  await page.getByTestId('titlebar-activity').click()
  await expect(page.getByTestId('activity-row')).toHaveCount(beforeRows + 1)
  await expect(page.getByTestId('activity-row').first()).toContainText('Profile renamed')
  await page.getByTestId('activity-close').click()

  // This spec shares a fixture server with the rest of the notification suite.
  await page.getByTestId('sidebar-open-settings').click()
  await page.getByTestId('profile-settings-name').fill(originalName!)
  await page.getByTestId('profile-settings-save-name').click()
  await expect(page.getByTestId('toast').last()).toContainText('Profile renamed')
})
