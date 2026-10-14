<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useRemoteConnection, type RemoteProfile } from '../stores/useRemoteConnection'
import { useCodeInstallation } from '../stores/useCodeInstallation'
import { errorText } from '../lib/appError'
import BaseButton from './ui/BaseButton.vue'
import TextInput from './ui/TextInput.vue'
import InlineError from './ui/InlineError.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsSection from './settings/SettingsSection.vue'
import SettingsRow from './settings/SettingsRow.vue'

const remote = useRemoteConnection()
const { profiles, activeProfile, busy, error } = remote
const installation = useCodeInstallation()
const router = useRouter()
const blank = (): RemoteProfile => ({
  id: '',
  name: '',
  host: '',
  connectionFile: '~/.config/hive/remote/connection.json',
  color: '#a78bfa',
})
const draft = ref<RemoteProfile>(activeProfile.value ? { ...activeProfile.value } : blank())
const colors = [
  { name: 'Purple', value: '#a855f7' },
  { name: 'Blue', value: '#60a5fa' },
  { name: 'Green', value: '#4ade80' },
  { name: 'Orange', value: '#fb923c' },
]
const formError = ref<string | null>(null)

function edit(profile?: RemoteProfile): void {
  draft.value = profile ? { ...profile } : blank()
  formError.value = null
}
function save(): string | null {
  formError.value = null
  try {
    const id = remote.save(draft.value)
    draft.value.id = id
    return id
  } catch (err) {
    formError.value = errorText(err, 'Could not save this connection.')
    return null
  }
}
async function connect(): Promise<void> {
  const id = save()
  if (!id) return
  if (await remote.connect(id)) {
    installation.select('remote')
    await router.push({ name: 'terminal' })
  }
}
function remove(): void {
  remote.remove(draft.value.id)
  edit()
}
</script>

<template>
  <SettingsPage testid="settings-remote">
    <SettingsSection
      title="Remote connections"
      description="Connect through SSH to an existing Hive installation. Hive verifies its HTTPS certificate and retrieves its token automatically. Only one remote connection is active at a time."
      boxed
    >
      <SettingsRow v-for="profile in profiles" :key="profile.id" :label="profile.name" :hint="profile.host">
        <div class="flex items-center gap-3">
          <span
            class="size-3 rounded-full border border-strong"
            :style="{ backgroundColor: profile.color }"
            aria-hidden="true"
          />
          <span v-if="activeProfile?.id === profile.id" class="text-small text-text-3">Connected</span>
          <BaseButton
            size="xs"
            variant="secondary"
            :disabled="busy"
            :data-testid="`remote-edit-${profile.id}`"
            @click="edit(profile)"
            >Configure</BaseButton
          >
        </div>
      </SettingsRow>
      <div class="p-4">
        <BaseButton variant="secondary" :disabled="busy" data-testid="remote-add" @click="edit()"
          >Add connection</BaseButton
        >
      </div>
    </SettingsSection>
    <SettingsSection :title="draft.id ? 'Configure connection' : 'New connection'" boxed>
      <form @submit.prevent="connect">
        <SettingsRow label="Name"
          ><TextInput
            v-model="draft.name"
            required
            aria-label="Connection name"
            data-testid="remote-name"
            :disabled="busy"
        /></SettingsRow>
        <SettingsRow
          label="SSH host"
          hint="Use a host from your SSH config. Its host key must already be trusted, with a key available through SSH or your agent."
        >
          <TextInput v-model="draft.host" required aria-label="SSH host" data-testid="remote-host" :disabled="busy" />
        </SettingsRow>
        <SettingsRow label="Color" hint="Identifies this connection in Inbox, Code, and Chats.">
          <div class="flex items-center gap-2">
            <BaseButton
              v-for="choice in colors"
              :key="choice.name"
              variant="ghost"
              size="xs"
              :aria-label="choice.name"
              :aria-pressed="draft.color === choice.value"
              :title="choice.name"
              :style="{
                color: choice.value,
                border: draft.color === choice.value ? `2px solid ${choice.value}` : '2px solid transparent',
              }"
              :disabled="busy"
              :data-testid="`remote-color-${choice.name.toLowerCase()}`"
              @click="draft.color = choice.value"
            >
              <span class="size-4 rounded-full" :style="{ backgroundColor: choice.value }" aria-hidden="true" />
            </BaseButton>
            <input
              v-model="draft.color"
              type="color"
              aria-label="Connection color"
              data-testid="remote-color"
              class="h-9 w-16 cursor-pointer rounded border border-strong bg-app p-1"
              :disabled="busy"
            />
          </div>
        </SettingsRow>
        <SettingsRow label="Connection file" hint="The private JSON file exported by Hive on the remote machine.">
          <TextInput
            v-model="draft.connectionFile"
            required
            aria-label="Remote connection file"
            data-testid="remote-connection-file"
            :disabled="busy"
          />
        </SettingsRow>
        <InlineError v-if="formError || error" :message="formError || error || ''" testid="remote-settings-error" />
        <div class="flex flex-wrap gap-2 p-4">
          <BaseButton type="submit" :busy="busy" data-testid="remote-connect">Connect</BaseButton>
          <BaseButton variant="secondary" :disabled="busy" data-testid="remote-save" @click="save">Save</BaseButton>
          <BaseButton
            v-if="activeProfile"
            variant="secondary"
            data-testid="remote-disconnect"
            @click="remote.disconnect"
            >Disconnect</BaseButton
          >
          <BaseButton v-if="draft.id" variant="ghost" :disabled="busy" data-testid="remote-remove" @click="remove"
            >Remove</BaseButton
          >
        </div>
      </form>
    </SettingsSection>
  </SettingsPage>
</template>
