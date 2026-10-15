import { vi } from 'vitest'
import { ref } from 'vue'
import type { Theme } from '../lib/themes'
import type { HiveUiPlatform } from '../platform'

export function createFakePlatform() {
  const theme = ref<Theme>('dark')
  const logger = { debug: vi.fn(), warn: vi.fn(), error: vi.fn() }
  return {
    clipboard: { writeText: vi.fn<(text: string) => Promise<void>>(() => Promise.resolve()) },
    log: vi.fn(() => logger),
    theme,
    toast: vi.fn<(msg: string) => void>(),
    logger,
  } satisfies HiveUiPlatform & { logger: typeof logger }
}

export const fakePlatform = createFakePlatform()
