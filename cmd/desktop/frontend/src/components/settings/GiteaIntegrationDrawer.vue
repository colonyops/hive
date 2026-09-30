<script setup lang="ts">
import { computed, ref } from 'vue'
import { Browser } from '@wailsio/runtime'
import BaseButton from '../BaseButton.vue'
import DrawerSheet from '../DrawerSheet.vue'
import GiteaMark from '../marks/GiteaMark.vue'
import SettingsField from './SettingsField.vue'
import { useGiteaConnection } from '../../composables/useGiteaConnection'
import { useIntegrations } from '../../composables/useIntegrations'

const emit = defineEmits<{ close: [] }>()

const TOKEN_DOCS = 'https://docs.gitea.com/development/api-usage'

const { busy, error, connect, disconnect } = useGiteaConnection()
const { accountsFor } = useIntegrations()
const connectedAccounts = computed(() => accountsFor('gitea'))

const urlInput = ref('')
const tokenInput = ref('')
const disconnecting = ref<string | null>(null)

const canConnect = computed(() => urlInput.value.trim() !== '' && tokenInput.value.trim() !== '')

// The entered instance's own token page, so minting one is a click away. Both
// forges serve it at the same path.
const instanceTokensUrl = computed(() => {
  const base = urlInput.value.trim().replace(/\/+$/, '')
  if (!/^https?:\/\//i.test(base)) return ''
  return `${base}/user/settings/applications`
})

function openDocs() {
  void Browser.OpenURL(TOKEN_DOCS)
}

function openInstanceTokens() {
  if (instanceTokensUrl.value) void Browser.OpenURL(instanceTokensUrl.value)
}

async function onConnect() {
  if (!canConnect.value) return
  const ok = await connect(urlInput.value.trim(), tokenInput.value.trim())
  if (ok) {
    urlInput.value = ''
    tokenInput.value = ''
  }
}

async function onDisconnect(account: string) {
  disconnecting.value = account
  try {
    await disconnect(account)
  } finally {
    disconnecting.value = null
  }
}
</script>

<template>
  <DrawerSheet ariaLabel="Gitea settings" testid="gitea-integration-drawer" backdrop-testid="gitea-integration-backdrop" :default-size="380" :min="320" :max="560" @close="emit('close')">
    <template #header>
      <div class="flex items-center gap-2.5">
        <span class="flex size-[26px] items-center justify-center rounded-[7px] bg-chip text-text-2"><GiteaMark class="size-3.5" /></span>
        <div>
          <div class="text-[14px] font-semibold tracking-[-.01em]">Gitea settings</div>
          <div class="font-mono text-[11px] text-text-3">Connected accounts</div>
        </div>
      </div>
    </template>

    <SettingsField label="Connected accounts" testid="gitea-connected">
      <div v-if="connectedAccounts.length > 0" class="flex flex-col gap-2">
        <div
          v-for="account in connectedAccounts"
          :key="account"
          class="flex items-center justify-between gap-3 rounded-lg border border-border bg-raised px-3 py-2.5"
          :data-testid="`gitea-connected-${account}`"
        >
          <div class="min-w-0 truncate font-mono text-[13px] text-text">{{ account }}</div>
          <BaseButton
            variant="secondary"
            size="sm"
            :busy="disconnecting === account"
            :data-testid="`gitea-disconnect-${account}`"
            @click="onDisconnect(account)"
          >Disconnect</BaseButton>
        </div>
      </div>
      <div v-else class="rounded-lg border border-border bg-raised px-3 py-2.5 text-[13px] text-text-3" data-testid="gitea-connected-empty">
        No account connected
      </div>
    </SettingsField>

    <div class="mt-5">
      <SettingsField label="Connect an instance" hint="The token is validated once and stored in your keychain; only the URL is written to disk." testid="gitea-connect">
        <div class="mb-2.5 rounded-lg border border-border bg-app px-3 py-2.5 text-xs leading-relaxed text-text-3" data-testid="gitea-connect-help">
          Create an <span class="text-text-2">access token</span> with the
          <span class="font-mono text-text-2">read:user</span>,
          <span class="font-mono text-text-2">read:issue</span> and
          <span class="font-mono text-text-2">read:notification</span>
          scopes, then paste it below. Forgejo instances work the same way.
          <div class="mt-1.5 flex flex-wrap gap-x-3 gap-y-1">
            <button type="button" class="cursor-pointer text-accent hover:underline" data-testid="gitea-connect-docs" @click="openDocs">Gitea docs ↗</button>
            <button v-if="instanceTokensUrl" type="button" class="cursor-pointer text-accent hover:underline" data-testid="gitea-connect-instance-link" @click="openInstanceTokens">Tokens on your instance ↗</button>
          </div>
        </div>
        <div class="flex flex-col gap-2">
          <input
            v-model="urlInput"
            type="url"
            placeholder="https://git.example.com"
            class="w-full rounded-lg border border-strong bg-app px-[11px] py-[9px] font-mono text-[13px] text-text outline-none focus:border-accent"
            data-testid="gitea-connect-url"
          >
          <input
            v-model="tokenInput"
            type="password"
            placeholder="Access token"
            class="w-full rounded-lg border border-strong bg-app px-[11px] py-[9px] font-mono text-[13px] text-text outline-none focus:border-accent"
            data-testid="gitea-connect-token"
          >
          <div>
            <BaseButton size="sm" :busy="busy" :disabled="!canConnect" data-testid="gitea-connect-submit" @click="onConnect">Connect</BaseButton>
          </div>
        </div>
      </SettingsField>
    </div>
    <p v-if="error" class="mt-2 text-xs text-severity-error" data-testid="gitea-connect-error">{{ error }}</p>

    <template #footer>
      <div class="flex items-center justify-end gap-2.5">
        <BaseButton variant="secondary" size="sm" data-testid="gitea-settings-close" @click="emit('close')">Close</BaseButton>
      </div>
    </template>
  </DrawerSheet>
</template>
