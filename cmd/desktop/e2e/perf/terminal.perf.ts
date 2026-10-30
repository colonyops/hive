import { mkdirSync, writeFileSync } from 'node:fs'
import { expect, test, type Page } from '@playwright/test'

interface BenchResult {
  scenario: string
  ok: boolean
  error?: string
  metrics: Record<string, number | boolean | string>
}

async function openScratchTerminal(page: Page): Promise<void> {
  await page.goto('/#/terminal')
  const start = page.getByTestId('terminal-start-scratch')
  await expect(start).toBeVisible({ timeout: 30_000 })
  await start.click()
  await expect(page.locator('[data-testid="terminal-pane-host"]:visible')).toBeVisible({ timeout: 30_000 })
  await page.waitForFunction(() => '__hiveTerminalBench' in window)
}

test('terminal benchmarks against real tmux', async ({ page }) => {
  await openScratchTerminal(page)
  const scale = Number(process.env.HIVE_PERF_SCALE ?? 1)
  const results = await page.evaluate(async (scale) => {
    const bench = (window as unknown as { __hiveTerminalBench: BenchHandle }).__hiveTerminalBench
    bench.start(
      bench.scenarios.filter((name) => name !== 'switchSession'),
      { scale },
    )
    for (;;) {
      const finished = bench.poll()
      if (finished) return finished
      await new Promise((resolve) => setTimeout(resolve, 500))
    }
  }, scale)

  mkdirSync('test-results/perf', { recursive: true })
  writeFileSync('test-results/perf/terminal.json', JSON.stringify(results, null, 2))
  for (const result of results) console.log(result.scenario, result.ok ? JSON.stringify(result.metrics) : result.error)

  for (const result of results) expect(result.ok, `${result.scenario}: ${result.error}`).toBe(true)
  // These two are counts of wrong frames rather than timings, so software GL
  // cannot excuse them.
  const switched = results.find((result) => result.scenario === 'switchWindow')
  expect(switched?.metrics.blankReveals).toBe(0)
  expect(switched?.metrics.freshFrameP50).toBe(0)
})

interface BenchHandle {
  scenarios: string[]
  start(names: string[], options: { scale: number }): void
  poll(): BenchResult[] | null
}
