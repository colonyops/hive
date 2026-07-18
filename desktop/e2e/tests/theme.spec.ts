import { expect, test } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const themes = ['light', 'midnight', 'gruvbox'] as const

test('switches between all themes from the command palette', async ({ page }, testInfo) => {
  await page.goto('/')
  await expect(page.getByTestId('feed-item')).toHaveCount(6)

  const palette = page.getByTestId('command-palette')
  const input = page.getByTestId('command-palette-input')
  const screenshots = join(here, '..', 'screenshots')
  await mkdir(screenshots, { recursive: true })

  for (const theme of themes) {
    await page.keyboard.press('Meta+k')
    await expect(palette).toBeVisible()
    await input.fill(`theme: ${theme}`)
    await expect(page.getByTestId('command-palette-command')).toHaveCount(1)
    await input.press('Enter')

    await expect(palette).toBeHidden()
    await expect.poll(() => page.evaluate(() => document.documentElement.dataset.theme)).toBe(theme)
    await page.screenshot({ path: join(screenshots, `full-window-${theme}-${testInfo.project.name}.png`), fullPage: true })
  }

  await page.keyboard.press('Meta+k')
  await input.fill('theme: dark')
  await expect(page.getByTestId('command-palette-command')).toHaveCount(1)
  await input.press('Enter')
  await expect.poll(() => page.evaluate(() => document.documentElement.dataset.theme)).toBe('dark')
})
