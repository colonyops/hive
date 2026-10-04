<script setup lang="ts">
import { computed, ref } from 'vue'
import IconPlus from '~icons/lucide/plus'
import IconKeyRound from '~icons/lucide/key-round'
import {
  CreateToken,
  ListTokens,
  RevokeToken,
  ServerURL,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/orchestrationservice'
import type { OrchestratorToken } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/data/stores/models'
import type { CreatedOrchestratorToken } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import SettingsHeading from './settings/SettingsHeading.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsSection from './settings/SettingsSection.vue'
import SettingsRow from './settings/SettingsRow.vue'
import BaseButton from './ui/BaseButton.vue'
import BaseModal from './ui/BaseModal.vue'
import ConfirmationHost from './ui/ConfirmationHost.vue'
import CopyButton from './ui/CopyButton.vue'
import EmptyState from './ui/EmptyState.vue'
import FormField from './ui/FormField.vue'
import InlineError from './ui/InlineError.vue'
import TextInput from './ui/TextInput.vue'
import { useConfirmation } from '../composables/useConfirmation'
import { useResource } from '../stores/useResource'
import { errorText } from '../lib/appError'

const tokens = useResource(async () => (await ListTokens()) ?? [], {
  initial: [] as OrchestratorToken[],
  errorFallback: 'Could not load access tokens.',
})
const serverURL = useResource(() => ServerURL(), { initial: '', errorFallback: 'Could not read the server address.' })
void tokens.reload()
void serverURL.reload()

const confirmation = useConfirmation()
const creating = ref(false)
const name = ref('')
const saving = ref(false)
const createError = ref<string | null>(null)
const created = ref<CreatedOrchestratorToken | null>(null)

const mcpConfig = computed(() =>
  JSON.stringify(
    {
      mcpServers: {
        'hive-orchestrator': {
          type: 'http',
          url: serverURL.data.value || 'http://127.0.0.1:<port>/mcp/orchestrator',
          headers: { Authorization: 'Bearer ${HIVE_ORCHESTRATOR_TOKEN}' },
        },
      },
    },
    null,
    2,
  ),
)

function openCreate(): void {
  name.value = ''
  createError.value = null
  creating.value = true
}

async function create(): Promise<void> {
  if (saving.value) return
  saving.value = true
  try {
    created.value = await CreateToken(name.value)
    creating.value = false
    await tokens.reload()
  } catch (err) {
    createError.value = errorText(err, 'Could not create the token.')
  } finally {
    saving.value = false
  }
}

function requestRevoke(token: OrchestratorToken): void {
  confirmation.request({
    title: 'Revoke access token',
    description: `Revoke ${token.name}? Anything using it loses access to the orchestrator server at once.`,
    confirmLabel: 'Revoke token',
    onConfirm: async () => {
      await RevokeToken(token.id)
      await tokens.reload()
    },
  })
}

function used(token: OrchestratorToken): string {
  return token.lastUsedAt ? `Last used ${new Date(token.lastUsedAt).toLocaleString()}` : 'Never used'
}
</script>

<template>
  <SettingsPage testid="orchestrator-settings">
    <SettingsHeading
      title="Orchestrator access"
      description="Access tokens let an agent or worker outside a Hive workspace use the Hive Orchestrator MCP server: start sessions, type into them, and use the message bus. Chats in a workspace that lists the server need no token."
    >
      <template #actions>
        <BaseButton size="sm" data-testid="orchestrator-token-create" @click="openCreate">
          <template #icon><IconPlus class="size-3.5" :stroke-width="2.4" /></template>New token
        </BaseButton>
      </template>
    </SettingsHeading>

    <SettingsSection title="Server" boxed>
      <SettingsRow label="Address" hint="Only reachable from this machine." testid="orchestrator-server-url">
        <code class="text-small text-text-2">{{ serverURL.data.value || 'The local HTTP server is off' }}</code>
      </SettingsRow>
    </SettingsSection>

    <InlineError v-if="tokens.error.value" :message="tokens.error.value" testid="orchestrator-tokens-error" />

    <SettingsSection title="Tokens" boxed testid="orchestrator-tokens">
      <SettingsRow
        v-for="token in tokens.data.value"
        :key="token.id"
        :label="token.name"
        :hint="`…${token.hint} · ${used(token)}`"
        :testid="`orchestrator-token-${token.id}`"
      >
        <BaseButton
          size="sm"
          variant="danger-outline"
          :data-testid="`orchestrator-token-revoke-${token.id}`"
          @click="requestRevoke(token)"
        >
          Revoke
        </BaseButton>
      </SettingsRow>
      <EmptyState v-if="tokens.loaded.value && !tokens.data.value.length" message="No access tokens." />
    </SettingsSection>

    <BaseModal
      v-if="creating"
      title="New access token"
      :icon="IconKeyRound"
      :busy="saving"
      testid="orchestrator-token-dialog"
      @close="creating = false"
    >
      <form class="flex flex-col gap-3 px-5 py-4" @submit.prevent="create">
        <FormField v-slot="{ id }" label="Name" hint="What will use it, such as the build worker." :error="createError">
          <TextInput :id="id" v-model="name" data-testid="orchestrator-token-name" />
        </FormField>
      </form>
      <template #footer>
        <BaseButton variant="secondary" @click="creating = false">Cancel</BaseButton>
        <BaseButton :busy="saving" :disabled="!name.trim()" data-testid="orchestrator-token-save" @click="create">
          Create token
        </BaseButton>
      </template>
    </BaseModal>

    <BaseModal
      v-if="created"
      :title="`Token for ${created.name}`"
      :icon="IconKeyRound"
      :width="680"
      testid="orchestrator-token-created"
      @close="created = null"
    >
      <div class="flex flex-col gap-3 px-5 py-4 text-small text-text-2">
        <p>Copy the token now. Hive keeps only a hash of it and cannot show it again.</p>
        <div class="flex items-center gap-2">
          <code
            class="min-w-0 flex-1 break-all rounded-md bg-sunken px-2 py-1.5"
            data-testid="orchestrator-token-value"
            >{{ created.token }}</code
          >
          <CopyButton :text="created.token" />
        </div>
        <p>Set it as <code>HIVE_ORCHESTRATOR_TOKEN</code> where the client runs, and point the client at the server:</p>
        <div class="flex items-start gap-2">
          <pre class="min-w-0 flex-1 overflow-x-auto rounded-md bg-sunken px-2 py-1.5 text-xs">{{ mcpConfig }}</pre>
          <CopyButton :text="mcpConfig" />
        </div>
      </div>
      <template #footer>
        <BaseButton data-testid="orchestrator-token-done" @click="created = null">Done</BaseButton>
      </template>
    </BaseModal>

    <ConfirmationHost :confirmation="confirmation" />
  </SettingsPage>
</template>
