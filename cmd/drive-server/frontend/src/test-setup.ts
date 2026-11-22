import { config, enableAutoUnmount } from '@vue/test-utils'
import { afterEach, beforeEach, vi } from 'vitest'
import { FakeEventSource } from './test-utils/eventSource'
import { installHiveUi } from '@hive/ui/platform'
import { fakePlatform } from '@hive/ui/test-utils/platform'
import { installMemoryStorage } from '@hive/ui/test-utils/memoryStorage'

config.global.plugins = [[installHiveUi, fakePlatform]]
enableAutoUnmount(afterEach)

installMemoryStorage()
vi.stubGlobal('EventSource', FakeEventSource)

beforeEach(() => {
  localStorage.clear()
  FakeEventSource.instances = []
})
