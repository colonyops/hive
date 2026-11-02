import { config, enableAutoUnmount } from '@vue/test-utils'
import { afterEach, beforeEach } from 'vitest'
import { installHiveUi } from '@hive/ui/platform'
import { fakePlatform } from '@hive/ui/test-utils/platform'
import { installMemoryStorage } from '@hive/ui/test-utils/memoryStorage'

config.global.plugins = [[installHiveUi, fakePlatform]]
enableAutoUnmount(afterEach)

installMemoryStorage()

beforeEach(() => {
  localStorage.clear()
})
