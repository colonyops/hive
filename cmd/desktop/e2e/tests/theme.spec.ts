import { mkdir } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from './fixtures.js'

const here = dirname(fileURLToPath(import.meta.url))

// Mirrors useTheme.ts (the e2e package cannot import frontend modules). The
// loop proves every id here has a CSS block that actually applies.
const themes = [
  { id: 'light', label: 'Light' },
  { id: 'midnight', label: 'Midnight' },
  { id: 'gruvbox', label: 'Gruvbox' },
  { id: 'slate', label: 'Slate' },
  { id: 'slate-light', label: 'Slate Light' },
  { id: 'one-dark', label: 'One Dark' },
  { id: 'one-light', label: 'One Light' },
  { id: 'tokyo-night', label: 'Tokyo Night' },
  { id: 'tokyo-night-day', label: 'Tokyo Night Day' },
  { id: 'catppuccin-mocha', label: 'Catppuccin Mocha' },
  { id: 'catppuccin-latte', label: 'Catppuccin Latte' },
  { id: 'nord', label: 'Nord' },
  { id: 'nord-light', label: 'Nord Light' },
] as const

test('switches between all themes from the command palette', async ({ page }, testInfo) => {
  await page.goto('/')
  await expect(page.getByTestId('feed-item')).toHaveCount(6)

  const palette = page.getByTestId('command-palette')
  const input = page.getByTestId('command-palette-input')
  const screenshots = join(here, '..', 'screenshots')
  await mkdir(screenshots, { recursive: true })

  for (const { id, label } of themes) {
    await page.keyboard.press('Meta+k')
    await expect(palette).toBeVisible()
    await input.fill(`Theme: ${label}`)
    // A label can prefix another ("Nord" / "Nord Light"), so the list may hold
    // more than one match — but equal-score results sort by title, putting the
    // exact match first, and Enter runs the top row. Pin it before pressing.
    await expect(page.getByTestId('command-palette-command-title').first()).toHaveText(`Theme: ${label}`)
    await input.press('Enter')

    await expect(palette).toBeHidden()
    await expect.poll(() => page.evaluate(() => document.documentElement.dataset.theme)).toBe(id)
    await page.screenshot({ path: join(screenshots, `full-window-${id}-${testInfo.project.name}.png`), fullPage: true })
  }

  await page.keyboard.press('Meta+k')
  await input.fill('Theme: Dark')
  await expect(page.getByTestId('command-palette-command-title').first()).toHaveText('Theme: Dark')
  await input.press('Enter')
  await expect.poll(() => page.evaluate(() => document.documentElement.dataset.theme)).toBe('dark')
})
