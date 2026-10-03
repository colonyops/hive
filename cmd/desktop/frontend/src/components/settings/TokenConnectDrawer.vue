<script setup lang="ts">
import { computed, ref, type Component } from 'vue'
import { Browser } from '@wailsio/runtime'
import BaseButton from '../ui/BaseButton.vue'
import DrawerSheet from '../ui/DrawerSheet.vue'
import FormField from '../ui/FormField.vue'
import InlineError from '../ui/InlineError.vue'
import TextInput from '../ui/TextInput.vue'
import ConnectedAccountsField from './ConnectedAccountsField.vue'
import { useIntegrations } from '../../composables/useIntegrations'
import { useTokenConnection } from '../../composables/useTokenConnection'

export interface TokenProvider {
  /** The integration key, and the prefix of every test id in the drawer. */
  id: string
  name: string
  icon: Component
  /** One connected account: `account`, `stack`, `project`. */
  noun: string
  connectLabel: string
  hint?: string
  docsUrl: string
  /** Where the entered instance mints a token, so making one is a click away. */
  tokenPage: { path: string; label: string }
  urlPlaceholder: string
  tokenPlaceholder: string
  defaultUrl?: string
  /** Omit it to replace the Connect button with the `extra-step` slot. */
  connect?: (url: string, token: string) => Promise<unknown>
  disconnect: (account: string) => Promise<void>
}

const props = defineProps<{
  provider: TokenProvider
  /** Freezes the URL and token while the `extra-step` slot finishes the connect. */
  locked?: boolean
}>()
const emit = defineEmits<{ close: [] }>()

// eslint-disable-next-line vue/no-setup-props-reactivity-loss -- a provider is a constant
const { busy, error, run, disconnect } = useTokenConnection(props.provider.disconnect, props.provider.noun)
const { accountsFor } = useIntegrations()
const accounts = computed(() => accountsFor(props.provider.id))

// eslint-disable-next-line vue/no-setup-props-reactivity-loss -- seeds the field once
const url = ref(props.provider.defaultUrl ?? '')
const token = ref('')
const canConnect = computed(() => url.value.trim() !== '' && token.value.trim() !== '')

const tokenPageUrl = computed(() => {
  const base = url.value.trim().replace(/\/+$/, '')
  return /^https?:\/\//i.test(base) ? base + props.provider.tokenPage.path : ''
})

async function onConnect(): Promise<void> {
  const connect = props.provider.connect
  if (!connect || !canConnect.value) return
  const ok = await run(
    () => connect(url.value.trim(), token.value.trim()),
    `${props.provider.name} rejected the connection.`,
  )
  if (ok) {
    url.value = ''
    token.value = ''
  }
}

function clearError(): void {
  error.value = null
}
</script>

<template>
  <DrawerSheet
    :title="`${provider.name} settings`"
    :subtitle="`Connected ${provider.noun}s`"
    :icon="provider.icon"
    :testid="`${provider.id}-integration-drawer`"
    :backdrop-testid="`${provider.id}-integration-backdrop`"
    :default-size="380"
    :min="320"
    :max="560"
    @close="emit('close')"
  >
    <ConnectedAccountsField :accounts="accounts" :noun="provider.noun" :testid="provider.id" :disconnect="disconnect" />

    <FormField
      :label="provider.connectLabel"
      :hint="
        provider.hint ?? 'The token is validated once and stored in your keychain; only the URL is written to disk.'
      "
      :testid="`${provider.id}-connect`"
      class="mt-5"
    >
      <div
        class="mb-2.5 rounded-lg border border-border bg-app px-3 py-2.5 text-xs leading-relaxed text-text-3"
        :data-testid="`${provider.id}-connect-help`"
      >
        <slot />
        <div class="mt-1.5 flex flex-wrap gap-x-3 gap-y-1">
          <button
            type="button"
            class="cursor-pointer text-accent hover:underline"
            :data-testid="`${provider.id}-connect-docs`"
            @click="Browser.OpenURL(provider.docsUrl)"
          >
            {{ provider.name }} docs ↗
          </button>
          <button
            v-if="tokenPageUrl"
            type="button"
            class="cursor-pointer text-accent hover:underline"
            :data-testid="`${provider.id}-connect-instance-link`"
            @click="Browser.OpenURL(tokenPageUrl)"
          >
            {{ provider.tokenPage.label }} ↗
          </button>
        </div>
      </div>
      <div class="flex flex-col gap-2">
        <TextInput
          v-model="url"
          type="url"
          :disabled="locked"
          :placeholder="provider.urlPlaceholder"
          :data-testid="`${provider.id}-connect-url`"
          size="sm"
          monospace
        />
        <TextInput
          v-model="token"
          type="password"
          :disabled="locked"
          :placeholder="provider.tokenPlaceholder"
          :data-testid="`${provider.id}-connect-token`"
          size="sm"
          monospace
        />
        <slot
          name="extra-step"
          :url="url.trim()"
          :token="token.trim()"
          :ready="canConnect"
          :busy="busy"
          :run="run"
          :clear-error="clearError"
        >
          <div>
            <BaseButton
              size="sm"
              :busy="busy"
              :disabled="!canConnect"
              :data-testid="`${provider.id}-connect-submit`"
              @click="onConnect"
              >Connect</BaseButton
            >
          </div>
        </slot>
      </div>
    </FormField>
    <InlineError v-if="error" :testid="`${provider.id}-connect-error`" variant="line" class="mt-2" :message="error" />

    <template #footer>
      <div class="flex items-center justify-end gap-2.5">
        <BaseButton variant="secondary" size="sm" :data-testid="`${provider.id}-settings-close`" @click="emit('close')"
          >Close</BaseButton
        >
      </div>
    </template>
  </DrawerSheet>
</template>
