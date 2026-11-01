import { expect, test, type Page } from '@playwright/test'

// In this lane because it is the one with real tmux, not because it measures
// anything. Run it alone:
//   HIVE_E2E_GO_TAGS=server,serverterminal ./cmd/desktop/e2e/scripts/run-docker.sh \
//     --config playwright.perf.config.ts perf/session-tabs-scrollback.perf.ts
const screenshots = process.env.HIVE_SESSION_TABS_SHOTS ?? 'screenshots'

async function dragTo(page: Page, from: { x: number; y: number }, to: { x: number; y: number }) {
  await page.mouse.move(from.x, from.y)
  await page.mouse.down()
  await page.mouse.move(to.x, to.y, { steps: 10 })
  await page.mouse.up()
}

async function findsOnce(page: Page, query: string) {
  await page.keyboard.press('Meta+f')
  const input = page.getByTestId('terminal-search-input')
  await input.fill(query)
  await expect(page.getByTestId('terminal-search-count')).toHaveText('1/1')
  await page.keyboard.press('Escape')
}

test('a terminal keeps its scrollback across a drag and a direction change', async ({ page }) => {
  await page.goto('/#/terminal')
  const start = page.getByTestId('terminal-start-scratch')
  await expect(start).toBeVisible({ timeout: 30_000 })
  await start.click()
  const pane = page.locator('[data-testid="terminal-pane-host"]:visible')
  await expect(pane).toBeVisible({ timeout: 30_000 })

  await pane.click()
  await page.keyboard.type('for i in $(seq 1 400); do echo mark-$((i*7)); done\n')
  await findsOnce(page, 'mark-707')

  await page.getByTestId('panel-split-row-p0').click()
  await page.getByTestId('panel-picker-terminal').click()
  await expect(page.locator('[data-testid="terminal-pane-host"]:visible')).toHaveCount(2, { timeout: 15_000 })
  await expect(page.getByTestId('panel-tab-row-p1').locator('[data-panel-tab]')).toHaveCount(1)
  await page.screenshot({ path: `${screenshots}/session-tabs-real-tmux-split.png` })

  const divider = (await page.getByTestId('panel-divider-s1').boundingBox())!
  const middle = { x: divider.x + divider.width / 2, y: divider.y + divider.height / 2 }
  await dragTo(page, middle, { x: middle.x + 150, y: middle.y })

  // Panes of different sizes: each window gets its own tmux grid.
  await page.waitForTimeout(800)
  const [left, right] = await page.locator('[data-testid="terminal-pane-host"]:visible').all()
  await right.click()
  await page.keyboard.type('stty size > /tmp/right-size\n')
  await left.click()
  await page.keyboard.type('[ "$(stty size)" != "$(cat /tmp/right-size)" ] && echo GRIDS-DIFFER-$((1+1))\n')
  await findsOnce(page, 'GRIDS-DIFFER-2')

  await page.getByTestId('panel-divider-s1').hover()
  await page.getByTestId('panel-turn-s1').click()
  await page.screenshot({ path: `${screenshots}/session-tabs-real-tmux-stacked.png` })

  await page.locator('[data-testid="terminal-pane-host"]:visible').first().click()
  await findsOnce(page, 'mark-707')
  await findsOnce(page, 'mark-1407')
})
