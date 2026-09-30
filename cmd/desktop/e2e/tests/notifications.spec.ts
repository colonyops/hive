import { expect, test } from './fixtures.js'

test('persists notification preferences from application settings', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('feed-item')).toHaveCount(6)

  await page.getByTestId('application-settings').click()
  await page.getByTestId('settings-category-notifications').click()
  await expect(page.getByTestId('notification-settings')).toBeVisible()

  const master = page.getByTestId('notification-enable')
  const delivery = page.getByTestId('notification-delivery')
  await expect(master).toHaveAttribute('aria-checked', 'true')
  await expect(delivery).toContainText('Automatic')
  await expect(page.getByTestId('notification-sound')).toBeVisible()

  await delivery.click()
  await page.getByTestId('notification-delivery-option-app').click()
  await expect(delivery).toContainText('Always in Hive')

  await master.click()
  await expect(master).toHaveAttribute('aria-checked', 'false')
  await expect(delivery).toBeDisabled()

  await page.reload()
  await expect(page.getByTestId('notification-settings')).toBeVisible()
  await expect(page.getByTestId('notification-enable')).toHaveAttribute('aria-checked', 'false')
  await expect(page.getByTestId('notification-delivery')).toContainText('Always in Hive')
})

test('records focused profile rename feedback in both toast and Activity', async ({ page }) => {
  await page.goto('/')
  const originalName = await page.getByTestId('sidebar-profile-name').textContent()
  expect(originalName).toBeTruthy()

  await page.getByTestId('titlebar-activity').click()
  await expect(page.getByTestId('activity-view')).toBeVisible()
  const beforeRows = await page.getByTestId('activity-row').count()
  await page.keyboard.press('Escape')

  const renamedName = `${originalName} notifications`
  await page.getByTestId('sidebar-open-settings').click()
  await page.getByTestId('profile-settings-name').fill(renamedName)
  await page.getByTestId('profile-settings-save-name').click()
  await expect(page.getByTestId('toast').last()).toContainText('Profile renamed')

  await page.getByTestId('titlebar-activity').click()
  await expect(page.getByTestId('activity-row')).toHaveCount(beforeRows + 1)
  await expect(page.getByTestId('activity-row').first()).toContainText('Profile renamed')
  await page.keyboard.press('Escape')
})
