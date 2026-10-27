<script setup lang="ts">
import AppSelect from '@hive/ui/components/ui/AppSelect.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import ConfirmationHost from '@hive/ui/components/ui/ConfirmationHost.vue'
import CopyButton from '@hive/ui/components/ui/CopyButton.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import FormField from '@hive/ui/components/ui/FormField.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import SegmentedControl from '@hive/ui/components/ui/SegmentedControl.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import TextInput from '@hive/ui/components/ui/TextInput.vue'
import { useConfirmation } from '@hive/ui/composables/useConfirmation'
import { errorText } from '@hive/ui/lib/errorText'
import { ref } from 'vue'
import { api } from '../api'
import { formatTime } from '../lib/format'
import { useLoad } from '../lib/load'
import type { CreatedToken, Token, TokenScope } from '../types'

const expiryOptions = [
  { value: '', label: 'Never expires' },
  { value: '1d', label: '1 day' },
  { value: '7d', label: '7 days' },
  { value: '30d', label: '30 days' },
  { value: '90d', label: '90 days' },
  { value: '365d', label: '365 days' },
]
const scopeOptions: { value: TokenScope; label: string }[] = [
  { value: 'read', label: 'Read' },
  { value: 'push', label: 'Push' },
]

const confirmation = useConfirmation()
const { data: tokens, error: loadError, reload: load } = useLoad(() => api.tokens(), 'Could not load tokens.')
const name = ref('')
const scope = ref<TokenScope>('read')
const expiresIn = ref('')
const busy = ref(false)
const error = ref<string | null>(null)
// The secret is never stored by the server again, so it lives in this page only.
const created = ref<CreatedToken | null>(null)

async function create(): Promise<void> {
  if (busy.value || !name.value.trim()) return
  busy.value = true
  error.value = null
  try {
    created.value = await api.createToken({ name: name.value.trim(), scope: scope.value, expires_in: expiresIn.value })
    name.value = ''
    await load()
  } catch (cause) {
    error.value = errorText(cause, 'Could not create the token.')
  } finally {
    busy.value = false
  }
}

function requestRevoke(token: Token): void {
  confirmation.request({
    title: 'Revoke this token?',
    description: `Anything that uses "${token.name}" stops working.`,
    confirmLabel: 'Revoke',
    testid: 'revoke-dialog',
    onConfirm: async () => {
      await api.revokeToken(token.id)
      await load()
    },
  })
}
</script>

<template>
  <div class="mx-auto flex max-w-3xl flex-col gap-6 px-6 py-6">
    <h1 class="text-heading font-semibold text-text">Tokens</h1>
    <p class="text-body text-text-3">
      A token lets the hive CLI sign in to this server. Read tokens pull. Push tokens also upload and share.
    </p>

    <section
      v-if="created"
      class="flex flex-col gap-2 rounded-lg border border-card bg-raised px-4 py-3"
      data-testid="token-secret"
    >
      <p class="text-body text-text">Copy this token now. It is not shown again.</p>
      <div class="flex items-center gap-2">
        <code class="min-w-0 flex-1 break-all rounded-md bg-chip px-2 py-1 font-mono text-small text-text">{{
          created.secret
        }}</code>
        <CopyButton :text="created.secret" />
      </div>
    </section>

    <form class="flex flex-col gap-4" data-testid="token-form" @submit.prevent="create">
      <FormField label="Name">
        <template #default="{ id }">
          <TextInput :id="id" v-model="name" data-testid="token-name" />
        </template>
      </FormField>
      <div class="flex flex-wrap gap-4">
        <SegmentedControl v-model="scope" label="Scope" :options="scopeOptions" />
        <FormField label="Expires">
          <AppSelect v-model="expiresIn" :options="expiryOptions" aria-label="Expiry" />
        </FormField>
      </div>
      <InlineError v-if="error" :message="error" testid="token-error" />
      <div>
        <BaseButton type="submit" :busy="busy" :disabled="!name.trim()" data-testid="token-create">
          Create token
        </BaseButton>
      </div>
    </form>

    <InlineError v-if="loadError" :message="loadError" testid="tokens-error" />
    <Spinner v-if="!tokens && !loadError" data-testid="tokens-loading" />
    <EmptyState
      v-else-if="tokens && !tokens.length"
      variant="boxed"
      message="No tokens yet."
      data-testid="tokens-empty"
    />
    <ul
      v-else-if="tokens"
      class="flex flex-col divide-y divide-border rounded-lg border border-card bg-raised"
      data-testid="tokens"
    >
      <li
        v-for="token in tokens"
        :key="token.id"
        class="flex flex-wrap items-center gap-3 px-4 py-3"
        data-testid="token-row"
      >
        <span class="min-w-0 flex-1 truncate text-body text-text">{{ token.name }}</span>
        <BaseBadge tone="neutral" class="px-2 py-0.5 text-caption">{{ token.scope }}</BaseBadge>
        <span class="text-small text-text-3">
          {{ token.expires_at ? `expires ${formatTime(token.expires_at)}` : 'never expires' }}
        </span>
        <span class="text-small text-text-3">
          {{ token.last_used_at ? `used ${formatTime(token.last_used_at)}` : 'never used' }}
        </span>
        <BaseButton
          variant="danger-outline"
          size="xs"
          class="h-7"
          data-testid="token-revoke"
          @click="requestRevoke(token)"
        >
          Revoke
        </BaseButton>
      </li>
    </ul>
    <ConfirmationHost :confirmation="confirmation" />
  </div>
</template>
