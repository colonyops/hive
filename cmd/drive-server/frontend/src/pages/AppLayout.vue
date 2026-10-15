<script setup lang="ts">
import { computed, onMounted, provide, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api'
import AccountMenu from '../components/AccountMenu.vue'
import HiveMark from '../components/HiveMark.vue'
import NavSidebar from '../components/NavSidebar.vue'
import ToolbarSearch from '../components/ToolbarSearch.vue'
import { createDriveData, driveDataKey } from '../lib/driveData'
import { meKey } from '../lib/me'
import { searchQueryKey } from '../lib/search'
import { fromRouteParams } from '../lib/space'
import { ancestorDirs } from '../lib/tree'
import type { Me } from '../types'

const route = useRoute()
const me = ref<Me | null>(null)
const query = ref('')
const data = createDriveData()

async function refresh(): Promise<void> {
  try {
    me.value = await api.me()
  } catch {
    // The meter is secondary; a page's own request reports a real failure.
  }
}
provide(meKey, { me, refresh })
provide(searchQueryKey, query)
provide(driveDataKey, data)
onMounted(refresh)

const inSpace = computed(() => ['repo', 'workspace', 'shared'].includes(String(route.name)))
const target = computed(() => (inSpace.value ? fromRouteParams(route.params) : null))
const space = computed(() => target.value?.space ?? null)

// A document marks its folder in the nav tree.
const folder = computed(() => {
  if (!target.value) return null
  const path = target.value.path.replace(/\/+$/, '')
  const isDocument = !!data.documents[target.value.space.key]?.some((d) => d.path === path)
  const dir = isDocument ? (ancestorDirs(path).pop() ?? '') : path
  return dir || null
})
</script>

<template>
  <div class="flex h-screen flex-col bg-app">
    <header class="flex h-12 shrink-0 items-center gap-3 border-b border-row bg-sidebar px-3">
      <RouterLink
        :to="{ name: 'spaces' }"
        class="flex shrink-0 items-center gap-2 pr-2 text-title font-semibold text-text md:w-60"
        data-testid="brand"
      >
        <HiveMark :size="22" />
        Hive Drive
      </RouterLink>
      <div class="flex min-w-0 flex-1 justify-center">
        <ToolbarSearch v-model="query" :space="space" />
      </div>
      <AccountMenu />
    </header>
    <div class="flex min-h-0 flex-1 flex-col md:flex-row">
      <NavSidebar :space="space" :folder="folder" />
      <main class="min-h-0 min-w-0 flex-1 overflow-y-auto">
        <RouterView />
      </main>
    </div>
  </div>
</template>
