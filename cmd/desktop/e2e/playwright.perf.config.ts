import { defineConfig } from '@playwright/test'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const PERF_PORT = 8941

// Apart from playwright.config.ts on purpose: this needs a server built with
// tmux enabled, and a run takes minutes rather than proving a behavior.
export default defineConfig({
  testDir: './perf',
  testMatch: '*.perf.ts',
  timeout: 600_000,
  workers: 1,
  retries: 0,
  outputDir: 'test-results',
  reporter: 'line',
  use: {
    baseURL: `http://127.0.0.1:${PERF_PORT}`,
    viewport: { width: 1360, height: 864 },
    browserName: 'chromium',
    launchOptions: {
      // A container has no GPU. Software GL costs far more than a real one, so
      // these frame numbers compare builds with each other and say nothing about
      // a real display; perf-native.sh measures that.
      args: ['--use-angle=swiftshader', '--enable-unsafe-swiftshader', '--ignore-gpu-blocklist'],
    },
  },
  webServer: {
    command: './scripts/serve-perf.sh',
    cwd: here,
    url: `http://127.0.0.1:${PERF_PORT}`,
    timeout: 120_000,
    reuseExistingServer: false,
  },
})
