<script setup lang="ts">
import SettingsError from './SettingsError.vue'
import { computed, onMounted, ref } from 'vue'
import { Browser } from '@wailsio/runtime'
import IconGithub from '~icons/lucide/github'
import BaseButton from '../BaseButton.vue'
import DrawerSheet from '../DrawerSheet.vue'
import SettingsField from './SettingsField.vue'
import { useGitHubConnection } from '../../composables/useGitHubConnection'
import * as SettingsService from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'

const emit = defineEmits<{ close: [] }>()

// Acquisition is provider-specific, so it lives with the provider's own
// surface rather than on the generic Integrations card. The card shows what
// the credential store holds and opens this.
const {
  status: connectionStatus, connected, deviceFlow, card, error: connectError, busy: connectBusy,
  startDeviceFlow, cancelDeviceFlow, useTokenInstead, backToStart, submitToken, disconnect,
} = useGitHubConnection()

const tokenInput = ref('')
// Disconnecting drops every stored GitHub account at once, so it asks first —
// inline rather than as a modal, which a drawer cannot stack.
const confirmingDisconnect = ref(false)

const pollIntervalSeconds = ref('60')
const minPollIntervalSeconds = ref(60)
const loading = ref(true)
const saving = ref(false)
const error = ref('')

const parsedInterval = computed(() => Number(pollIntervalSeconds.value))
const valid = computed(() => Number.isInteger(parsedInterval.value) && parsedInterval.value >= minPollIntervalSeconds.value)
const minimumHint = computed(() => `Minimum ${minPollIntervalSeconds.value}s — GitHub's polling contract`)

function messageFor(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

async function openVerification() {
  const uri = deviceFlow.value?.verificationUri
  if (!uri) return
  try {
    await Browser.OpenURL(uri)
  } catch (err) {
    console.warn('Unable to open the verification page', err)
  }
}

async function confirmDisconnect() {
  confirmingDisconnect.value = false
  await disconnect()
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const settings = await SettingsService.GithubSettings()
    pollIntervalSeconds.value = String(settings.pollIntervalSeconds)
    minPollIntervalSeconds.value = settings.minPollIntervalSeconds
  } catch (err) {
    error.value = messageFor(err)
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!valid.value || saving.value) return
  saving.value = true
  error.value = ''
  try {
    await SettingsService.SetGithubSettings({
      pollIntervalSeconds: parsedInterval.value,
      minPollIntervalSeconds: minPollIntervalSeconds.value,
    })
    emit('close')
  } catch (err) {
    error.value = messageFor(err)
  } finally {
    saving.value = false
  }
}

onMounted(() => void load())
</script>

<template>
  <DrawerSheet ariaLabel="GitHub settings" testid="github-integration-drawer" backdrop-testid="github-integration-backdrop" :default-size="380" :min="320" :max="560" @close="emit('close')">
    <template #header>
      <div class="flex items-center gap-2.5">
        <span class="flex size-[26px] items-center justify-center rounded-[7px] bg-chip text-text-2"><IconGithub class="size-3.5" /></span>
        <div>
          <div class="text-[14px] font-semibold tracking-[-.01em]">GitHub settings</div>
          <div class="font-mono text-[11px] text-text-3">Connection and polling</div>
        </div>
      </div>
    </template>

    <SettingsField label="Connection" testid="github-connection">
      <div v-if="connected" class="flex items-center justify-between gap-3 rounded-lg border border-border bg-raised px-3 py-2.5">
        <div class="min-w-0">
          <div class="truncate text-[13px] text-text" data-testid="github-connection-account">
            {{ connectionStatus?.login ? `Connected as ${connectionStatus.login}` : 'Connected' }}
          </div>
          <div v-if="connectionStatus?.name" class="truncate text-xs text-text-3">{{ connectionStatus.name }}</div>
        </div>
        <BaseButton
          v-if="!confirmingDisconnect"
          variant="secondary"
          size="sm"
          data-testid="github-connection-disconnect"
          @click="confirmingDisconnect = true"
        >Disconnect</BaseButton>
        <div v-else class="flex shrink-0 items-center gap-2">
          <BaseButton variant="secondary" size="sm" data-testid="github-connection-disconnect-cancel" @click="confirmingDisconnect = false">Cancel</BaseButton>
          <BaseButton variant="danger" size="sm" data-testid="github-connection-disconnect-confirm" @click="confirmDisconnect">Disconnect</BaseButton>
        </div>
      </div>

      <div v-else-if="card === 'device'" class="rounded-lg border border-border bg-raised px-3 py-2.5" data-testid="github-connection-device">
        <div class="text-xs text-text-3">Enter this code on GitHub:</div>
        <div class="mt-1 font-mono text-[17px] font-semibold tracking-[.14em] text-text" data-testid="github-connection-code">{{ deviceFlow?.userCode }}</div>
        <div class="mt-2.5 flex items-center gap-2">
          <BaseButton size="sm" data-testid="github-connection-open" @click="openVerification">Open GitHub</BaseButton>
          <BaseButton variant="secondary" size="sm" data-testid="github-connection-cancel" @click="cancelDeviceFlow">Cancel</BaseButton>
        </div>
        <div class="mt-2 text-xs text-text-4">Waiting for authorization…</div>
      </div>

      <div v-else-if="card === 'token'" class="rounded-lg border border-border bg-raised px-3 py-2.5" data-testid="github-connection-token">
        <input
          v-model="tokenInput"
          type="password"
          placeholder="ghp_…"
          class="w-full rounded-lg border border-strong bg-app px-[11px] py-[9px] font-mono text-[13px] text-text outline-none focus:border-accent"
          data-testid="github-connection-token-input"
        >
        <div class="mt-2.5 flex items-center gap-2">
          <BaseButton size="sm" :busy="connectBusy" :disabled="!tokenInput.trim()" data-testid="github-connection-token-submit" @click="submitToken(tokenInput)">Connect</BaseButton>
          <BaseButton variant="secondary" size="sm" data-testid="github-connection-token-back" @click="backToStart">Back</BaseButton>
        </div>
      </div>

      <div v-else class="flex items-center justify-between gap-3 rounded-lg border border-border bg-raised px-3 py-2.5">
        <div class="text-[13px] text-text-3">No account connected</div>
        <div class="flex shrink-0 items-center gap-2">
          <BaseButton variant="secondary" size="sm" data-testid="github-connection-use-token" @click="useTokenInstead">Use a token</BaseButton>
          <BaseButton size="sm" :busy="connectBusy" data-testid="github-connection-connect" @click="startDeviceFlow">Connect</BaseButton>
        </div>
      </div>
    </SettingsField>
    <p v-if="connectError" class="mt-2 text-xs text-severity-error" data-testid="github-connection-error">{{ connectError }}</p>

    <div class="mt-5">
      <SettingsField label="Poll interval" :hint="minimumHint" testid="github-poll-interval">
        <div class="relative">
          <input
            v-model="pollIntervalSeconds"
            type="number"
            inputmode="numeric"
            :min="minPollIntervalSeconds"
            step="1"
            class="w-full rounded-lg border border-strong bg-app px-[11px] py-[9px] pr-16 text-[13px] text-text outline-none focus:border-accent"
            :class="!valid ? 'border-severity-error' : ''"
            data-testid="github-poll-interval-input"
            :disabled="loading"
          >
          <span class="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-text-3">seconds</span>
        </div>
      </SettingsField>
    </div>
    <p v-if="!valid" class="mt-2 text-xs text-severity-error" data-testid="github-poll-interval-error">Enter a whole number of at least {{ minPollIntervalSeconds }} seconds.</p>
    <SettingsError v-if="error" class="mt-4" :message="error" testid="github-settings-error" />

    <template #footer>
      <div class="flex items-center justify-end gap-2.5">
        <BaseButton
          variant="secondary"
          size="sm"
          data-testid="github-settings-cancel"
          @click="emit('close')"
        >Cancel</BaseButton>
        <BaseButton
          size="sm"
          :busy="loading || saving"
          :disabled="!valid"
          data-testid="github-settings-save"
          @click="save"
        >Save</BaseButton>
      </div>
    </template>
  </DrawerSheet>
</template>
