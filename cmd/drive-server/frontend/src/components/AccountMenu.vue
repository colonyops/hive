<script setup lang="ts">
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import type { MenuEntry } from '@hive/ui/types/menu'
import { computed, shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import IconKeyRound from '~icons/lucide/key-round'
import IconLogOut from '~icons/lucide/log-out'
import IconUserRound from '~icons/lucide/user-round'
import { api } from '../api'
import { useMe } from '../lib/me'
import { forgetSignIn } from '../router'

const router = useRouter()
const me = useMe()
const anchor = shallowRef<HTMLElement | null>(null)

const entries = computed<MenuEntry[]>(() => [
  { kind: 'label', text: me?.me.value?.email ?? 'Signed in' },
  { kind: 'action', id: 'tokens', label: 'Tokens', icon: IconKeyRound, testid: 'account-tokens' },
  { kind: 'separator' },
  { kind: 'action', id: 'sign-out', label: 'Sign out', icon: IconLogOut, testid: 'sign-out' },
])

async function onSelect(id: string): Promise<void> {
  anchor.value = null
  if (id === 'tokens') await router.push({ name: 'tokens' })
  else if (id === 'sign-out') {
    await api.signOut()
    forgetSignIn()
    await router.push({ name: 'sign-in' })
  }
}
</script>

<template>
  <div>
    <IconButton
      label="Account"
      :icon="IconUserRound"
      size="xl"
      variant="outline"
      :active="!!anchor"
      data-testid="account-menu"
      @click="anchor = anchor ? null : ($event.currentTarget as HTMLElement)"
    />
    <AppMenu
      v-if="anchor"
      :entries="entries"
      :anchor="anchor"
      :ignore="[anchor]"
      testid="account-menu-panel"
      @select="onSelect"
      @close="anchor = null"
    />
  </div>
</template>
