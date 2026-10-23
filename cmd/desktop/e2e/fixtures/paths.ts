import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const e2eRoot = join(dirname(fileURLToPath(import.meta.url)), '..')

export function screenshotPath(name: string): string {
  const dir = join(e2eRoot, 'screenshots')
  mkdirSync(dir, { recursive: true })
  return join(dir, name)
}

export function testdataPath(...parts: string[]): string {
  return join(e2eRoot, 'testdata', ...parts)
}
