<script setup lang="ts">
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import RenameDialog from '@hive/ui/components/ui/RenameDialog.vue'
import type { MenuEntry } from '@hive/ui/types/menu'
import { computed, ref, shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import IconBadge from '~icons/material-symbols/badge-outline'
import IconKeyRound from '~icons/material-symbols/key'
import IconLogOut from '~icons/material-symbols/logout'
import IconUserRound from '~icons/material-symbols/account-circle'
import { api, describeApiError } from '../api'
import { useMe } from '../lib/me'
import { forgetSignIn } from '../router'

const router = useRouter()
const me = useMe()
const root = shallowRef<HTMLElement | null>(null)
const open = ref(false)
const email = computed(() => me?.me.value?.email ?? 'Signed in')
const label = computed(() => {
  const name = me?.me.value?.display_name
  return name ? `${name} · ${email.value}` : email.value
})

const renaming = ref(false)
const saving = ref(false)
const renameError = ref<string | null>(null)

async function saveName(name: string): Promise<void> {
  saving.value = true
  renameError.value = null
  try {
    await api.updateMe(name)
    await me?.refresh()
    renaming.value = false
  } catch (e) {
    renameError.value = describeApiError(e, 'Could not save the name.')
  } finally {
    saving.value = false
  }
}

const entries = computed<MenuEntry[]>(() => [
  { kind: 'label', text: label.value },
  { kind: 'action', id: 'name', label: 'Display name', icon: IconBadge, testid: 'account-name' },
  { kind: 'action', id: 'tokens', label: 'API tokens', icon: IconKeyRound, testid: 'account-tokens' },
  { kind: 'separator' },
  { kind: 'action', id: 'sign-out', label: 'Sign out', icon: IconLogOut, testid: 'sign-out' },
])

async function onSelect(id: string): Promise<void> {
  open.value = false
  if (id === 'name') {
    renameError.value = null
    renaming.value = true
  } else if (id === 'tokens') await router.push({ name: 'tokens' })
  else if (id === 'sign-out') {
    await api.signOut()
    forgetSignIn()
    await router.push({ name: 'sign-in' })
  }
}
</script>

<template>
  <div ref="root" class="account-menu relative" :title="open ? email : undefined">
    <IconButton
      label="Account"
      :icon="IconUserRound"
      size="xl"
      variant="outline"
      :active="open"
      data-testid="account-menu"
      @click="open = !open"
    />
    <!-- Not anchored: an anchored menu takes the width of its anchor, which is the 34px avatar. -->
    <AppMenu
      v-if="open"
      :entries="entries"
      width="18rem"
      :ignore="[root]"
      testid="account-menu-panel"
      @select="onSelect"
      @close="open = false"
    />
    <RenameDialog
      v-if="renaming"
      title="Display name"
      label="The name people see on your comments"
      :name="me?.me.value?.display_name ?? ''"
      confirm-label="Save"
      :busy="saving"
      :error="renameError"
      testid="display-name"
      @save="saveName"
      @close="renaming = false"
    />
  </div>
</template>

<style scoped>
/* The group label is built for short mono captions; an email needs its own case and to truncate. */
.account-menu :deep(.app-menu-label) {
  overflow: hidden;
  font-family: var(--font-sans);
  font-size: var(--text-small);
  font-weight: 500;
  letter-spacing: 0;
  text-overflow: ellipsis;
  text-transform: none;
  white-space: nowrap;
}
</style>
