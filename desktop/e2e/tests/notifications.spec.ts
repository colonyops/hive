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
