import { vi } from 'vitest'
import type { HiveUiPlatform } from '../platform'

export function createFakePlatform() {
  const logger = { debug: vi.fn(), warn: vi.fn(), error: vi.fn() }
  return {
    clipboard: { writeText: vi.fn<(text: string) => Promise<void>>(() => Promise.resolve()) },
    log: vi.fn(() => logger),
    logger,
  } satisfies HiveUiPlatform & { logger: typeof logger }
}

export const fakePlatform = createFakePlatform()
