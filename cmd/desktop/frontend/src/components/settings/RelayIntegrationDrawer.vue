<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { Browser } from '@wailsio/runtime'
import RelayMark from '../marks/RelayMark.vue'
import BaseButton from '../ui/BaseButton.vue'
import DrawerSheet from '../ui/DrawerSheet.vue'
import FormField from '../ui/FormField.vue'
import InlineError from '../ui/InlineError.vue'
import SegmentedControl from '../ui/SegmentedControl.vue'
import TextInput from '../ui/TextInput.vue'
import ConnectedAccountsField from './ConnectedAccountsField.vue'
import { useIntegrations } from '../../composables/useIntegrations'
import { useTokenConnection } from '../../composables/useTokenConnection'
import {
  ConnectWithKey,
  Disconnect,
  Login,
  Methods,
  StartEmail,
  VerifyEmail,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/relayservice'
import type { Methods as RelayMethods } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/sources/relay/models'

type Method = 'email' | 'password' | 'key'

interface MethodOption {
  value: Method
  label: string
}

const DEFAULT_URL = 'https://relay.hivedesktop.com'
const DOCS_URL = 'https://hivedesktop.com/desktop/inbox/sources/#hive-relay'
const CODE_RE = /^\d{8}$/

const emit = defineEmits<{ close: [] }>()

const { busy, error, run, disconnect } = useTokenConnection(Disconnect, 'account')
const { accountsFor } = useIntegrations()
const accounts = computed(() => accountsFor('relay'))

const url = ref(DEFAULT_URL)
const trimmedUrl = computed(() => url.value.trim())
// The URL the methods were read for. A form for one relay must not submit to
// another, so editing the URL hides the forms until the methods are re-read.
const checkedUrl = ref('')
const methods = ref<RelayMethods | null>(null)
const method = ref<Method>('email')

const email = ref('')
const code = ref('')
const codeSent = ref(false)
const password = ref('')
const apiKey = ref('')

const methodOptions = computed<MethodOption[]>(() => {
  const m = methods.value
  if (!m) return []
  const options: MethodOption[] = []
  if (m.email_code) options.push({ value: 'email', label: 'Email code' })
  if (m.password) options.push({ value: 'password', label: 'Password' })
  if (m.api_key) options.push({ value: 'key', label: 'API key' })
  return options
})
const ready = computed(() => checkedUrl.value !== '' && checkedUrl.value === trimmedUrl.value)

watch(methodOptions, (options) => {
  const first = options[0]
  if (first && !options.some((o) => o.value === method.value)) method.value = first.value
})

const hasEmail = computed(() => email.value.trim() !== '')
const canVerify = computed(() => CODE_RE.test(code.value.trim()))

async function checkMethods(): Promise<void> {
  const target = trimmedUrl.value
  if (target === '' || target === checkedUrl.value) return
  methods.value = null
  checkedUrl.value = ''
  codeSent.value = false
  await run(async () => {
    methods.value = await Methods(target)
    checkedUrl.value = target
  }, 'Could not reach that relay.')
}

function resetForm(): void {
  email.value = ''
  code.value = ''
  codeSent.value = false
  password.value = ''
  apiKey.value = ''
}

async function sendCode(): Promise<void> {
  if (!hasEmail.value) return
  const ok = await run(() => StartEmail(checkedUrl.value, email.value.trim()), 'The relay did not send a code.')
  if (ok) {
    code.value = ''
    codeSent.value = true
  }
}

async function verify(): Promise<void> {
  if (!canVerify.value) return
  const ok = await run(
    () => VerifyEmail(checkedUrl.value, email.value.trim(), code.value.trim()),
    'The relay rejected the code.',
  )
  if (ok) resetForm()
}

async function login(): Promise<void> {
  if (!hasEmail.value || password.value === '') return
  const ok = await run(
    () => Login(checkedUrl.value, email.value.trim(), password.value),
    'The relay rejected the sign-in.',
  )
  if (ok) resetForm()
}

async function connectWithKey(): Promise<void> {
  if (apiKey.value.trim() === '') return
  const ok = await run(() => ConnectWithKey(checkedUrl.value, apiKey.value.trim()), 'The relay rejected the API key.')
  if (ok) resetForm()
}

function changeEmail(): void {
  codeSent.value = false
  code.value = ''
  error.value = null
}

onMounted(() => void checkMethods())
</script>

<template>
  <DrawerSheet
    title="Hive Relay settings"
    subtitle="Connected accounts"
    :icon="RelayMark"
    testid="relay-integration-drawer"
    backdrop-testid="relay-integration-backdrop"
    :default-size="380"
    :min="320"
    :max="560"
    @close="emit('close')"
  >
    <ConnectedAccountsField :accounts="accounts" noun="account" testid="relay" :disconnect="disconnect" />

    <FormField
      label="Connect a relay"
      hint="The token is stored in your keychain; only the URL is written to disk."
      testid="relay-connect"
      class="mt-5"
    >
      <div
        class="mb-2.5 rounded-lg border border-border bg-app px-3 py-2.5 text-xs leading-relaxed text-text-3"
        data-testid="relay-connect-help"
      >
        <template v-if="ready && method === 'key'">
          Paste an API key. An operator mints one with
          <span class="font-mono text-text-2">hive-relay tokens create</span>.
        </template>
        <template v-else>Enter the relay's URL, then sign in with a method it offers.</template>
        <div class="mt-1.5">
          <button
            type="button"
            class="cursor-pointer text-accent hover:underline"
            data-testid="relay-connect-docs"
            @click="Browser.OpenURL(DOCS_URL)"
          >
            Hive Relay docs ↗
          </button>
        </div>
      </div>

      <div class="flex flex-col gap-2">
        <div class="flex gap-2">
          <TextInput
            v-model="url"
            type="url"
            :placeholder="DEFAULT_URL"
            data-testid="relay-connect-url"
            size="sm"
            monospace
            @blur="checkMethods"
            @keydown.enter="checkMethods"
          />
          <BaseButton
            v-if="!ready"
            variant="secondary"
            size="sm"
            :busy="busy"
            :disabled="trimmedUrl === ''"
            data-testid="relay-check"
            @click="checkMethods"
            >Check</BaseButton
          >
        </div>

        <template v-if="ready">
          <SegmentedControl
            v-if="methodOptions.length > 1"
            v-model="method"
            :options="methodOptions"
            aria-label="Sign-in method"
            testid="relay-method"
          />

          <div v-if="methodOptions.length === 0" class="text-xs text-text-3" data-testid="relay-no-methods">
            This relay offers no sign-in method.
          </div>

          <template v-else-if="method === 'email' && !codeSent">
            <TextInput
              v-model="email"
              type="email"
              placeholder="you@example.com"
              autocomplete="email"
              data-testid="relay-connect-email"
              size="sm"
              @keydown.enter="sendCode"
            />
            <div>
              <BaseButton size="sm" :busy="busy" :disabled="!hasEmail" data-testid="relay-send-code" @click="sendCode"
                >Send code</BaseButton
              >
            </div>
          </template>

          <div
            v-else-if="method === 'email'"
            class="rounded-lg border border-border bg-raised px-3 py-2.5"
            data-testid="relay-code-step"
          >
            <div class="text-xs text-text-3">
              Enter the 8-digit code sent to <span class="text-text-2">{{ email.trim() }}</span
              >:
            </div>
            <TextInput
              v-model="code"
              inputmode="numeric"
              autocomplete="one-time-code"
              maxlength="8"
              placeholder="00000000"
              data-testid="relay-connect-code"
              class="mt-2 tracking-[.14em]"
              size="sm"
              monospace
              @keydown.enter="verify"
            />
            <div class="mt-2.5 flex items-center gap-2">
              <BaseButton size="sm" :busy="busy" :disabled="!canVerify" data-testid="relay-verify" @click="verify"
                >Verify</BaseButton
              >
              <BaseButton variant="secondary" size="sm" data-testid="relay-change-email" @click="changeEmail"
                >Back</BaseButton
              >
            </div>
            <button
              type="button"
              class="mt-2 cursor-pointer text-xs text-accent hover:underline disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="busy"
              data-testid="relay-resend"
              @click="sendCode"
            >
              Send a new code
            </button>
          </div>

          <template v-else-if="method === 'password'">
            <TextInput
              v-model="email"
              type="email"
              placeholder="you@example.com"
              autocomplete="username"
              data-testid="relay-connect-email"
              size="sm"
            />
            <TextInput
              v-model="password"
              type="password"
              placeholder="Password"
              autocomplete="current-password"
              data-testid="relay-connect-password"
              size="sm"
              @keydown.enter="login"
            />
            <div>
              <BaseButton
                size="sm"
                :busy="busy"
                :disabled="!hasEmail || password === ''"
                data-testid="relay-login"
                @click="login"
                >Sign in</BaseButton
              >
            </div>
          </template>

          <template v-else>
            <TextInput
              v-model="apiKey"
              type="password"
              placeholder="API key"
              data-testid="relay-connect-token"
              size="sm"
              monospace
              @keydown.enter="connectWithKey"
            />
            <div>
              <BaseButton
                size="sm"
                :busy="busy"
                :disabled="apiKey.trim() === ''"
                data-testid="relay-connect-submit"
                @click="connectWithKey"
                >Connect</BaseButton
              >
            </div>
          </template>
        </template>
      </div>
    </FormField>
    <InlineError v-if="error" testid="relay-error" variant="line" class="mt-2" :message="error" />

    <template #footer>
      <div class="flex items-center justify-end gap-2.5">
        <BaseButton variant="secondary" size="sm" data-testid="relay-settings-close" @click="emit('close')"
          >Close</BaseButton
        >
      </div>
    </template>
  </DrawerSheet>
</template>
