<script setup lang="ts">
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { forgetSignIn } from '../router'

const router = useRouter()

async function signOut(): Promise<void> {
  await api.signOut()
  forgetSignIn()
  await router.push({ name: 'sign-in' })
}
</script>

<template>
  <div class="min-h-screen">
    <header class="flex h-12 items-center gap-4 border-b border-row bg-sidebar px-4">
      <RouterLink :to="{ name: 'spaces' }" class="text-title font-semibold text-text">Hive Drive</RouterLink>
      <div class="flex-1" />
      <BaseButton variant="ghost" size="xs" data-testid="sign-out" @click="signOut">Sign out</BaseButton>
    </header>
    <RouterView />
  </div>
</template>
