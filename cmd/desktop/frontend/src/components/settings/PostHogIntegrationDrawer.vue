<script setup lang="ts">
import { computed, ref } from 'vue'
import { Browser } from '@wailsio/runtime'
import IconBug from '~icons/lucide/bug'
import AppSelect, { type AppSelectOption } from '../AppSelect.vue'
import BaseButton from '../BaseButton.vue'
import DrawerSheet from '../DrawerSheet.vue'
import SettingsField from './SettingsField.vue'
import { usePostHogConnection } from '../../composables/usePostHogConnection'
import { useIntegrations } from '../../composables/useIntegrations'

const emit = defineEmits<{ close: [] }>()

const API_KEY_DOCS = 'https://posthog.com/docs/api/personal-api-keys'

const { busy, error, projects, loadProjects, connect, reset, disconnect } = usePostHogConnection()
const { accountsFor } = useIntegrations()
const connectedAccounts = computed(() => accountsFor('posthog'))

const urlInput = ref('https://us.posthog.com')
const tokenInput = ref('')
const selectedProject = ref<number | null>(null)
const disconnecting = ref<string | null>(null)

const canLoad = computed(() => urlInput.value.trim() !== '' && tokenInput.value.trim() !== '')
const picking = computed(() => projects.value.length > 0)

// AppSelect is string-valued and a project id is numeric, so the conversion
// happens here rather than leaking a stringly-typed id into the connect call.
const projectOptions = computed<AppSelectOption[]>(() =>
  projects.value.map((project) => ({ value: String(project.id), label: `${project.name} (#${project.id})` })),
)
const selectedProjectValue = computed(() => (selectedProject.value === null ? '' : String(selectedProject.value)))

function chooseProject(value: string) {
  const id = Number(value)
  selectedProject.value = Number.isFinite(id) ? id : null
}

// The entered instance's own key page, so creating the key is one click away.
const instanceKeysUrl = computed(() => {
  const base = urlInput.value.trim().replace(/\/+$/, '')
  if (!/^https?:\/\//i.test(base)) return ''
  return `${base}/settings/user-api-keys`
})

function openDocs() {
  void Browser.OpenURL(API_KEY_DOCS)
}

function openInstanceKeys() {
  if (instanceKeysUrl.value) void Browser.OpenURL(instanceKeysUrl.value)
}

async function onLoadProjects() {
  if (!canLoad.value) return
  const ok = await loadProjects(urlInput.value.trim(), tokenInput.value.trim())
  if (ok) selectedProject.value = projects.value[0]?.id ?? null
}

async function onConnect() {
  if (selectedProject.value === null) return
  const ok = await connect(urlInput.value.trim(), tokenInput.value.trim(), selectedProject.value)
  if (ok) {
    // The key stays so a second project can be connected without re-pasting it.
    selectedProject.value = null
    reset()
  }
}

function onStartOver() {
  selectedProject.value = null
  reset()
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
  <DrawerSheet ariaLabel="PostHog settings" testid="posthog-integration-drawer" backdrop-testid="posthog-integration-backdrop" :default-size="380" :min="320" :max="560" @close="emit('close')">
    <template #header>
      <div class="flex items-center gap-2.5">
        <span class="flex size-[26px] items-center justify-center rounded-[7px] bg-chip text-text-2"><IconBug class="size-3.5" /></span>
        <div>
          <div class="text-[14px] font-semibold tracking-[-.01em]">PostHog settings</div>
          <div class="font-mono text-[11px] text-text-3">Connected projects</div>
        </div>
      </div>
    </template>

    <SettingsField label="Connected projects" testid="posthog-connected">
      <div v-if="connectedAccounts.length > 0" class="flex flex-col gap-2">
        <div
          v-for="account in connectedAccounts"
          :key="account"
          class="flex items-center justify-between gap-3 rounded-lg border border-border bg-raised px-3 py-2.5"
          :data-testid="`posthog-connected-${account}`"
        >
          <div class="min-w-0 truncate font-mono text-[13px] text-text">{{ account }}</div>
          <BaseButton
            variant="secondary"
            size="sm"
            :busy="disconnecting === account"
            :data-testid="`posthog-disconnect-${account}`"
            @click="onDisconnect(account)"
          >Disconnect</BaseButton>
        </div>
      </div>
      <div v-else class="rounded-lg border border-border bg-raised px-3 py-2.5 text-[13px] text-text-3" data-testid="posthog-connected-empty">
        No project connected
      </div>
    </SettingsField>

    <div class="mt-5">
      <SettingsField label="Connect a project" hint="The key is validated once and stored in your keychain; only the host and project id are written to disk." testid="posthog-connect">
        <div class="mb-2.5 rounded-lg border border-border bg-app px-3 py-2.5 text-xs leading-relaxed text-text-3" data-testid="posthog-connect-help">
          Create a <span class="text-text-2">personal API key</span> with the <span class="text-text-2">project:read</span>, <span class="text-text-2">error_tracking:read</span> and <span class="text-text-2">alert:read</span> scopes, then paste it below. One key can connect several projects — connect each one separately to route them to different feeds.
          <div class="mt-1.5 flex flex-wrap gap-x-3 gap-y-1">
            <button type="button" class="cursor-pointer text-accent hover:underline" data-testid="posthog-connect-docs" @click="openDocs">PostHog docs ↗</button>
            <button v-if="instanceKeysUrl" type="button" class="cursor-pointer text-accent hover:underline" data-testid="posthog-connect-instance-link" @click="openInstanceKeys">API keys on your instance ↗</button>
          </div>
        </div>

        <div class="flex flex-col gap-2">
          <input
            v-model="urlInput"
            type="url"
            :disabled="picking"
            placeholder="https://us.posthog.com"
            class="w-full rounded-lg border border-strong bg-app px-[11px] py-[9px] font-mono text-[13px] text-text outline-none focus:border-accent disabled:opacity-60"
            data-testid="posthog-connect-url"
          >
          <input
            v-model="tokenInput"
            type="password"
            :disabled="picking"
            placeholder="phx_…"
            class="w-full rounded-lg border border-strong bg-app px-[11px] py-[9px] font-mono text-[13px] text-text outline-none focus:border-accent disabled:opacity-60"
            data-testid="posthog-connect-token"
          >

          <!-- Step two. A personal API key spans projects, so the project is
               picked from what the key can actually see rather than typed. -->
          <template v-if="picking">
            <AppSelect
              :model-value="selectedProjectValue"
              :options="projectOptions"
              aria-label="PostHog project"
              testid="posthog-connect-project"
              @update:model-value="chooseProject"
            />
            <div class="flex gap-2">
              <BaseButton size="sm" :busy="busy" :disabled="selectedProject === null" data-testid="posthog-connect-submit" @click="onConnect">Connect</BaseButton>
              <BaseButton variant="secondary" size="sm" data-testid="posthog-connect-back" @click="onStartOver">Use another key</BaseButton>
            </div>
          </template>
          <div v-else>
            <BaseButton size="sm" :busy="busy" :disabled="!canLoad" data-testid="posthog-connect-load" @click="onLoadProjects">Find projects</BaseButton>
          </div>
        </div>
      </SettingsField>
    </div>
    <p v-if="error" class="mt-2 text-xs text-severity-error" data-testid="posthog-connect-error">{{ error }}</p>

    <template #footer>
      <div class="flex items-center justify-end gap-2.5">
        <BaseButton variant="secondary" size="sm" data-testid="posthog-settings-close" @click="emit('close')">Close</BaseButton>
      </div>
    </template>
  </DrawerSheet>
</template>
