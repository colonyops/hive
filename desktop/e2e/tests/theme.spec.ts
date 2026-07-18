import { expect, test } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))

test('toggles between dark and light themes from the command palette', async ({ page }, testInfo) => {
  await page.goto('/')
  await expect(page.getByTestId('feed-item')).toHaveCount(6)

  await page.keyboard.press('Meta+k')
  const palette = page.getByTestId('command-palette')
  const input = page.getByTestId('command-palette-input')
  await input.fill('theme')
  await expect(page.getByTestId('command-palette-command')).toHaveText('Toggle light/dark theme')
  await input.press('Enter')

  await expect(palette).toBeHidden()
  await expect.poll(() => page.evaluate(() => document.documentElement.dataset.theme)).toBe('light')

  const screenshots = join(here, '..', 'screenshots')
  await mkdir(screenshots, { recursive: true })
  await page.screenshot({ path: join(screenshots, `full-window-light-${testInfo.project.name}.png`), fullPage: true })

  await page.keyboard.press('Meta+k')
  await input.fill('theme')
  await input.press('Enter')
  await expect.poll(() => page.evaluate(() => document.documentElement.dataset.theme)).toBe('dark')
})
