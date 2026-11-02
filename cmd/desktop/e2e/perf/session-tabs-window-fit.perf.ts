import { expect, test, type Page } from '@playwright/test'
import { execFileSync } from 'node:child_process'

// In this lane because it is the one with real tmux, not because it measures
// anything. Run it alone, because it changes a global tmux option:
//   HIVE_E2E_GO_TAGS=server,serverterminal ./cmd/desktop/e2e/scripts/run-docker.sh \
//     --config playwright.perf.config.ts perf/session-tabs-window-fit.perf.ts

// The container's own tmux, the one the server under test attaches to.
const tmux = (...args: string[]) => execFileSync('tmux', args, { encoding: 'utf8' }).trim()
const activeWindow = () =>
  tmux('list-windows', '-a', '-F', '#{window_active} #{window_id}')
    .split('\n')
    .find((line) => line.startsWith('1 '))

const panes = (page: Page) => page.locator('[data-testid="terminal-pane-host"]:visible')

// How far each visible grid is from filling its pane, in pixels. A grid tmux left
// at the old size overflows, so this goes negative. Within a cell or two is a fit.
function slack(page: Page) {
  return panes(page).evaluateAll((hosts) =>
    hosts.map((host) => {
      // The host takes the grid's size, so only the pane around it says what fits.
      const box = host.closest('[data-testid="terminal-pane"]')!.getBoundingClientRect()
      const grid = host.querySelector('.xterm-screen')!.getBoundingClientRect()
      return { width: Math.round(box.width - grid.width), height: Math.round(box.height - grid.height) }
    }),
  )
}

async function expectEveryGridFits(page: Page) {
  await expect
    .poll(
      async () => {
        const fits = (await slack(page)).every((s) => s.width >= 0 && s.width < 40 && s.height >= 0 && s.height < 40)
        return fits ? 'fits' : JSON.stringify({ slack: await slack(page), tmux: tmux('list-windows', '-a', '-F', '#{window_id} #{window_width}x#{window_height} active=#{window_active}') })
      },
      { timeout: 5_000 },
    )
    .toBe('fits')
}

test('a window in a split fits its pane without being activated', async ({ page }) => {
  await page.goto('/#/terminal')
  const start = page.getByTestId('terminal-start-scratch')
  await expect(start).toBeVisible({ timeout: 30_000 })
  await start.click()
  await expect(panes(page)).toBeVisible({ timeout: 30_000 })

  // A common tmux setting, under which tmux sizes only a session's current window.
  tmux('set-option', '-g', 'aggressive-resize', 'on')

  await page.getByTestId('panel-split-row-p0').click()
  await page.getByTestId('panel-picker-terminal').click()
  await expect(panes(page)).toHaveCount(2, { timeout: 15_000 })
  await expectEveryGridFits(page)
  const current = activeWindow()

  const divider = (await page.getByTestId('panel-divider-s1').boundingBox())!
  const y = divider.y + divider.height / 2
  const dragBoundary = async (from: number, to: number) => {
    await page.mouse.move(from, y)
    await page.mouse.down()
    await page.mouse.move(to, y, { steps: 10 })
    await page.mouse.up()
  }
  const middle = divider.x + divider.width / 2

  // The window that is not current narrows, so it needs a grid of its own.
  await dragBoundary(middle, middle - 220)
  await expectEveryGridFits(page)

  // Now it is the largest, so the session's size is the one it needs.
  await dragBoundary(middle - 220, middle + 220)
  await expectEveryGridFits(page)
  expect(activeWindow(), 'nothing here may activate a window').toBe(current)

  await page.reload()
  await expect(panes(page)).toHaveCount(2, { timeout: 30_000 })
  await expectEveryGridFits(page)
  expect(activeWindow(), 'nor may a reload').toBe(current)
})
