import { config, enableAutoUnmount } from '@vue/test-utils'
import { afterEach, beforeEach, vi } from 'vitest'
import { installHiveUi } from './platform'
import { fakePlatform } from './test-utils/platform'
import { installMemoryStorage } from './test-utils/memoryStorage'

config.global.plugins = [[installHiveUi, fakePlatform]]

// Leaked wrappers stay attached to the document, and each carries its own Vue
// app whose `useId()` counter restarts at the same value, so a stale wrapper's
// `id` shadows the live one.
enableAutoUnmount(afterEach)

installMemoryStorage()

beforeEach(() => {
  localStorage.clear()
  vi.mocked(fakePlatform.clipboard.writeText).mockReset().mockResolvedValue(undefined)
  vi.mocked(fakePlatform.toast).mockReset()
  fakePlatform.theme.value = 'dark'
})
