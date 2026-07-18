import { defineConfig } from '@playwright/test'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  testDir: './tests',
  timeout: 30_000,
  expect: { timeout: 10_000 },
  outputDir: 'test-results',
  use: {
    baseURL: 'http://127.0.0.1:8931',
    viewport: { width: 1360, height: 864 },
  },
  webServer: {
    command: './scripts/serve.sh',
    cwd: here,
    url: 'http://127.0.0.1:8931',
    timeout: 180_000,
    reuseExistingServer: !process.env.CI,
  },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
    { name: 'webkit', use: { browserName: 'webkit' } },
  ],
})
