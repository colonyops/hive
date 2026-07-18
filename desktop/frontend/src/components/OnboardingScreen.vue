<script setup lang="ts">
import { ref } from 'vue'
import { Browser } from '@wailsio/runtime'
import IconGithub from '~icons/lucide/github'
import type { DeviceFlowInfo } from '../types/auth'
import type { OnboardingCard } from '../composables/useAuth'

const props = defineProps<{
  card: OnboardingCard
  deviceFlow: DeviceFlowInfo | null
  error: string | null
  busy: boolean
}>()

const emit = defineEmits<{
  startDeviceFlow: []
  useTokenInstead: []
  backToStart: []
  submitToken: [token: string]
}>()

const tokenInput = ref('')
const copied = ref(false)

const steps = [
  { label: 'Connect GitHub', active: true },
  { label: 'Create your first workspace', active: false },
  { label: 'Add feeds & tasks', active: false },
]

async function openVerification() {
  const uri = props.deviceFlow?.verificationUri
  if (!uri) return
  try {
    await Browser.OpenURL(uri)
  } catch {
    window.open(uri, '_blank')
  }
}

async function copyCode() {
  const code = props.deviceFlow?.userCode
  if (!code) return
  try {
    await navigator.clipboard.writeText(code)
    copied.value = true
    setTimeout(() => { copied.value = false }, 1600)
  } catch (err) {
    console.warn('Unable to copy code', err)
  }
}

function submit() {
  if (tokenInput.value.trim()) emit('submitToken', tokenInput.value.trim())
}
</script>

<template>
  <div class="flex min-h-0 flex-1" data-testid="onboarding">
    <!-- Left: brand panel with onboarding steps -->
    <aside class="flex w-[470px] shrink-0 flex-col border-r border-border bg-raised px-11 py-12">
      <div class="mb-10 flex items-center gap-3">
        <div class="flex size-[38px] items-center justify-center rounded-[11px] bg-accent-tint font-mono text-[17px] font-bold text-accent">h</div>
        <span class="font-mono text-[17px] font-semibold">hive</span>
      </div>
      <h1 class="mb-3 text-[26px] font-semibold leading-[1.25] tracking-[-.02em]">Triage GitHub and<br>spin up sessions.</h1>
      <p class="mb-11 max-w-[330px] text-sm leading-relaxed text-text-3">Connect your account to pull PRs, issues, and notifications into workspaces you control.</p>
      <ol class="flex flex-col gap-5">
        <li v-for="(step, index) in steps" :key="step.label" class="flex items-center gap-3.5">
          <span
            class="flex size-[30px] shrink-0 items-center justify-center rounded-full text-[13px] font-semibold"
            :class="step.active ? 'bg-accent text-accent-contrast' : 'border border-strong bg-chip text-text-3'"
          >{{ index + 1 }}</span>
          <span class="text-sm" :class="step.active ? 'font-medium text-text' : 'text-text-3'">{{ step.label }}</span>
        </li>
      </ol>
      <div class="flex-1" />
      <p class="text-xs text-text-4">Tokens are stored in your OS keychain.</p>
    </aside>

    <!-- Right: connect card -->
    <section class="flex flex-1 items-center justify-center bg-pane p-10">
      <div class="w-[420px] text-center">
        <div class="mx-auto mb-5 flex size-[60px] items-center justify-center rounded-[15px] border border-strong bg-chip text-text">
          <IconGithub class="size-[30px]" />
        </div>
        <h2 class="mb-2 text-xl font-semibold tracking-[-.01em]">Connect to GitHub</h2>

        <!-- idle: not started -->
        <template v-if="card === 'idle'">
          <p class="mb-6 text-[13.5px] leading-relaxed text-text-3">Sign in from this device to pull your PRs, issues, and notifications into Hive.</p>
          <button
            class="primary-button"
            :disabled="busy"
            data-testid="onboarding-connect"
            @click="emit('startDeviceFlow')"
          >Connect GitHub</button>
          <p v-if="error" class="mt-4 text-xs text-kind-issue" data-testid="onboarding-error">{{ error }}</p>
          <p class="mt-4 text-xs text-text-4">
            <button class="link-quiet" data-testid="onboarding-use-token" @click="emit('useTokenInstead')">Use a token instead</button>
          </p>
        </template>

        <!-- device: waiting for authorization -->
        <template v-else-if="card === 'device'">
          <p class="mb-6 text-[13.5px] leading-relaxed text-text-3">Open the link and enter this code to authorize Hive on your account.</p>
          <div class="mb-2.5 rounded-xl border border-strong bg-app px-4 py-[18px] font-mono text-[32px] font-semibold tracking-[.28em] text-accent" data-testid="onboarding-user-code">{{ deviceFlow?.userCode }}</div>
          <div class="mb-6 flex items-center justify-center gap-2 text-xs text-text-3">
            <span class="size-1.5 rounded-full bg-accent [animation:hivePulse_1.6s_ease-in-out_infinite]" />
            Waiting for authorization…
          </div>
          <button class="primary-button" data-testid="onboarding-open-verification" @click="openVerification">Open github.com/login/device ↗</button>
          <p v-if="error" class="mt-4 text-xs text-kind-issue" data-testid="onboarding-error">{{ error }}</p>
          <p class="mt-4 text-xs text-text-4">
            <button class="link-quiet" data-testid="onboarding-copy-code" @click="copyCode">{{ copied ? 'Copied' : 'Copy code' }}</button>
            ·
            <button class="link-quiet" data-testid="onboarding-use-token" @click="emit('useTokenInstead')">Use a token instead</button>
          </p>
        </template>

        <!-- token: personal access token fallback -->
        <template v-else>
          <p class="mb-6 text-[13.5px] leading-relaxed text-text-3">Paste a personal access token with <span class="font-mono text-text-2">repo</span> and <span class="font-mono text-text-2">notifications</span> scopes.</p>
          <input
            v-model="tokenInput"
            type="password"
            placeholder="ghp_…"
            class="mb-3 w-full rounded-lg border border-strong bg-app px-3.5 py-2.5 font-mono text-[13.5px] text-text outline-none placeholder:text-text-4 focus:border-accent"
            data-testid="onboarding-token-input"
            @keydown.enter="submit"
          >
          <button
            class="primary-button"
            :disabled="busy || !tokenInput.trim()"
            data-testid="onboarding-token-submit"
            @click="submit"
          >Save token</button>
          <p v-if="error" class="mt-4 text-xs text-kind-issue" data-testid="onboarding-error">{{ error }}</p>
          <p class="mt-4 text-xs text-text-4">
            <button class="link-quiet" data-testid="onboarding-back" @click="emit('backToStart')">Back to device sign-in</button>
          </p>
        </template>
      </div>
    </section>
  </div>
</template>

<style scoped>
.primary-button {
  width: 100%;
  border-radius: 9px;
  background: var(--color-accent);
  color: var(--color-accent-contrast);
  padding: 12px;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: filter .12s ease;
}
.primary-button:hover:not(:disabled) { filter: brightness(1.08); }
.primary-button:disabled { opacity: .55; cursor: default; }
.link-quiet { color: inherit; cursor: pointer; }
.link-quiet:hover { color: var(--color-text); }
</style>
