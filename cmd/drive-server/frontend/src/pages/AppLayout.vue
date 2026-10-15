<script setup lang="ts">
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import { computed, onMounted, provide, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api'
import AccountMenu from '../components/AccountMenu.vue'
import IconMenu from '~icons/lucide/menu'
import HiveMark from '../components/HiveMark.vue'
import NavSidebar from '../components/NavSidebar.vue'
import ToastHost from '../components/ToastHost.vue'
import ToolbarSearch from '../components/ToolbarSearch.vue'
import { createDriveData, driveDataKey } from '../lib/driveData'
import { meKey } from '../lib/me'
import { searchQueryKey } from '../lib/search'
import { fromRouteParams } from '../lib/space'
import { createToast, toastKey } from '../lib/toast'
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
provide(toastKey, createToast())
onMounted(refresh)

const inSpace = computed(() => ['repo', 'workspace', 'shared'].includes(String(route.name)))
const target = computed(() => (inSpace.value ? fromRouteParams(route.params) : null))
const space = computed(() => target.value?.space ?? null)

// Below the md breakpoint the nav is a panel the menu button opens.
const navOpen = ref(false)
watch(
  () => route.fullPath,
  () => {
    navOpen.value = false
  },
)
</script>

<template>
  <div class="relative flex h-screen flex-col bg-app">
    <header class="flex h-16 shrink-0 items-center gap-2 border-b md:gap-3 border-row bg-sidebar px-4">
      <IconButton
        label="Navigation"
        :icon="IconMenu"
        size="xl"
        class="md:hidden"
        :active="navOpen"
        data-testid="nav-toggle"
        @click="navOpen = !navOpen"
      />
      <RouterLink
        :to="{ name: 'recent' }"
        class="flex shrink-0 items-center gap-2.5 text-heading font-semibold text-text md:w-64"
        data-testid="brand"
      >
        <HiveMark :size="28" />
        <span class="hidden sm:inline">Hive Drive</span>
      </RouterLink>
      <div class="flex min-w-0 flex-1 justify-center">
        <ToolbarSearch v-model="query" :space="space" />
      </div>
      <AccountMenu />
    </header>
    <div class="flex min-h-0 flex-1 flex-col md:flex-row">
      <NavSidebar :space="space" :class="navOpen ? 'absolute inset-x-0 bottom-0 top-16 z-20' : 'hidden md:flex'" />
      <main class="min-h-0 min-w-0 flex-1 overflow-y-auto">
        <RouterView />
      </main>
    </div>
    <ToastHost />
  </div>
</template>
