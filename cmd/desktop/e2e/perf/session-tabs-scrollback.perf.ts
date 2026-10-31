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

  await page.getByTestId('session-split-row').click()
  await page.getByTestId('session-pane-picker-terminal').click()
  await expect(page.locator('[data-testid="terminal-pane-host"]:visible')).toHaveCount(2, { timeout: 15_000 })
  await expect(page.getByTestId('session-tab-row-1').locator('[data-session-tab]')).toHaveCount(1)
  await page.screenshot({ path: `${screenshots}/session-tabs-real-tmux-split.png` })

  const divider = (await page.getByTestId('session-split-divider').boundingBox())!
  const middle = { x: divider.x + divider.width / 2, y: divider.y + divider.height / 2 }
  await dragTo(page, middle, { x: middle.x + 150, y: middle.y })
  await page.getByTestId('session-split-column').click()
  await page.screenshot({ path: `${screenshots}/session-tabs-real-tmux-stacked.png` })

  await page.getByTestId('session-tab-terminal').click()
  await page.locator('[data-testid="terminal-pane-host"]:visible').first().click()
  await findsOnce(page, 'mark-707')
  await findsOnce(page, 'mark-1407')
})
