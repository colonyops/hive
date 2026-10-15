<script setup lang="ts">
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import { computed, onMounted, provide, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { formatBytes } from '../lib/format'
import { meKey } from '../lib/me'
import { forgetSignIn } from '../router'
import type { Me } from '../types'

const router = useRouter()
const me = ref<Me | null>(null)

async function refresh(): Promise<void> {
  try {
    me.value = await api.me()
  } catch {
    // The meter is secondary; a page's own request reports a real failure.
  }
}
provide(meKey, { me, refresh })
onMounted(refresh)

const quotaPercent = computed(() => {
  const quota = me.value?.quota
  if (!quota?.limit) return 0
  return Math.min(100, Math.round((quota.used / quota.limit) * 100))
})

async function signOut(): Promise<void> {
  await api.signOut()
  forgetSignIn()
  await router.push({ name: 'sign-in' })
}
</script>

<template>
  <div class="min-h-screen">
    <header class="flex min-h-12 flex-wrap items-center gap-x-4 gap-y-1 border-b border-row bg-sidebar px-4 py-2">
      <RouterLink :to="{ name: 'spaces' }" class="text-title font-semibold text-text">Hive Drive</RouterLink>
      <nav class="flex gap-3 text-body text-text-3">
        <RouterLink :to="{ name: 'spaces' }" class="hover:text-text">Spaces</RouterLink>
        <RouterLink :to="{ name: 'tokens' }" class="hover:text-text">Tokens</RouterLink>
      </nav>
      <div class="flex-1" />
      <div v-if="me" class="flex items-center gap-3 text-small text-text-3" data-testid="storage-meter">
        <span>{{ me.email }}</span>
        <span v-if="me.quota.limit" class="flex items-center gap-2">
          <span
            class="h-1.5 w-24 overflow-hidden rounded-full bg-chip"
            role="progressbar"
            :aria-valuenow="quotaPercent"
            aria-valuemin="0"
            aria-valuemax="100"
          >
            <span class="block h-full bg-accent" :style="{ width: `${quotaPercent}%` }" />
          </span>
          {{ formatBytes(me.quota.used) }} of {{ formatBytes(me.quota.limit) }}
        </span>
        <span v-else>{{ formatBytes(me.quota.used) }} used</span>
      </div>
      <BaseButton variant="ghost" size="xs" data-testid="sign-out" @click="signOut">Sign out</BaseButton>
    </header>
    <RouterView />
  </div>
</template>
