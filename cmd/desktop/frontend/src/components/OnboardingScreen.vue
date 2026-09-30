<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Browser } from '@wailsio/runtime'
import IconAlertTriangle from '~icons/lucide/alert-triangle'
import IconBell from '~icons/lucide/bell'
import IconBot from '~icons/lucide/bot'
import IconCheck from '~icons/lucide/check'
import IconFolderGit2 from '~icons/lucide/folder-git-2'
import IconGithub from '~icons/lucide/github'
import HiveSetupForm from './HiveSetupForm.vue'
import type { DeviceFlowInfo } from '../types/github'
import type { ConnectCard } from '../composables/useGitHubConnection'
import type { NotificationPermission } from '../composables/useNotificationSettings'
import type { useHiveSetup } from '../composables/useHiveSetup'
import { useClipboard } from '../composables/useClipboard'

const props = defineProps<{
  card: ConnectCard | 'hive' | 'permissions' | 'agent'
  deviceFlow: DeviceFlowInfo | null
  error: string | null
  busy: boolean
  githubConnected: boolean
  // The live OS permission state; only read on the 'permissions' card.
  permission?: NotificationPermission
  // App.vue owns the single draft instance that the wizard advances.
  hive?: ReturnType<typeof useHiveSetup>
}>()

const emit = defineEmits<{
  startDeviceFlow: []
  useTokenInstead: []
  backToStart: []
  submitToken: [token: string]
  saveHive: []
  // Leaving the Hive step without a save: confirming a found config, or
  // skipping the form.
  finishHive: []
  skipConnect: []
  requestPermission: []
  finishPermissions: []
  startAgent: []
  finishAgent: []
}>()

const tokenInput = ref('')
const { copy, copied } = useClipboard({ resetDelay: 1600 })

// Confirming the skip is local to this screen: it is a warning to read, not a
// state the app has to hold. Leaving the connect step at all drops it.
const confirmingSkip = ref(false)
watch(() => props.card, () => { confirmingSkip.value = false })

// Profile creation is absent because one exists before first run starts and
// connecting seeds it.
const activeStep = computed(() => {
  if (props.card === 'hive') return 1
  if (props.card === 'permissions') return 3
  if (props.card === 'agent') return 4
  return 2
})
const steps = [
  { label: 'Set up your agent and code', step: 1 },
  { label: 'Connect GitHub', step: 2 },
  { label: 'Turn on notifications', step: 3 },
  { label: 'Configure Hive with your agent', step: 4 },
]

const isConnectCard = computed(() => props.card === 'idle' || props.card === 'device' || props.card === 'token')

const heading = computed(() => {
  if (confirmingSkip.value) return 'Skip connecting GitHub?'
  if (props.card === 'hive') return props.hive?.usable.value ? 'Using your Hive config' : 'Set up your agent and code'
  if (props.card === 'permissions') return 'Turn on notifications'
  if (props.card === 'agent') return 'Configure Hive with your agent'
  return 'Connect to GitHub'
})

async function openVerification() {
  const uri = props.deviceFlow?.verificationUri
  if (!uri) return
  try {
    await Browser.OpenURL(uri)
  } catch {
    window.open(uri, '_blank')
  }
}

function copyCode() {
  const code = props.deviceFlow?.userCode
  if (!code) return
  void copy(code)
}

function submit() {
  if (props.busy) return
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
      <p class="mb-11 max-w-[330px] text-sm leading-relaxed text-text-3">{{ card === 'hive'
        ? 'Point Hive at your agent and your repositories, then connect the account it triages.'
        : card === 'agent'
          ? 'Your agent can set up the rest: the profiles and feeds that sort your inbox the way you work.'
          : 'Connect the account Hive pulls PRs, issues, and notifications from.' }}</p>
      <ol class="flex flex-col gap-5">
        <li
          v-for="(step, i) in steps"
          :key="step.label"
          class="flex items-center gap-3.5"
          :aria-current="step.step === activeStep ? 'step' : undefined"
        >
          <span
            aria-hidden="true"
            class="flex size-[30px] shrink-0 items-center justify-center rounded-full text-[13px] font-semibold"
            :class="step.step === activeStep ? 'bg-accent text-accent-contrast' : step.step < activeStep ? 'border border-accent-tint bg-chip text-accent' : 'border border-strong bg-chip text-text-3'"
          ><IconCheck v-if="step.step < activeStep" class="size-3.5" /><template v-else>{{ i + 1 }}</template></span>
          <span class="text-sm" :class="step.step === activeStep ? 'font-medium text-text' : 'text-text-3'">
            {{ step.label }}<span v-if="step.step < activeStep" class="sr-only">, complete</span>
          </span>
        </li>
      </ol>
      <div class="flex-1" />
      <p class="text-xs text-text-4">Tokens are stored in your OS keychain.</p>
    </aside>

    <!-- my-auto lets an over-height card scroll from its top; items-center
         would clip both ends. -->
    <section class="flex min-h-0 flex-1 justify-center overflow-y-auto bg-pane p-10">
      <div class="my-auto" :class="card === 'hive' ? 'w-[560px] max-w-full' : 'w-[420px] text-center'">
        <div class="mx-auto mb-5 flex size-[60px] items-center justify-center rounded-[15px] border border-strong bg-chip text-text">
          <IconAlertTriangle v-if="confirmingSkip" class="size-[30px]" />
          <IconFolderGit2 v-else-if="card === 'hive'" class="size-[30px]" />
          <IconBell v-else-if="card === 'permissions'" class="size-[30px]" />
          <IconBot v-else-if="card === 'agent'" class="size-[30px]" />
          <IconGithub v-else class="size-[30px]" />
        </div>
        <h2 class="mb-2 text-xl font-semibold tracking-[-.01em]" :class="card === 'hive' ? 'text-center' : ''">{{ heading }}</h2>

        <!-- skip: the warning the bypass goes past, not a gate -->
        <template v-if="confirmingSkip">
          <p class="mb-6 text-[13.5px] leading-relaxed text-text-3">Your profile will have no sources, so your feed stays empty until you connect an account under Settings ▸ Integrations.</p>
          <button
            class="primary-button"
            data-testid="onboarding-skip-confirm"
            @click="emit('skipConnect')"
          >Continue without GitHub</button>
          <p class="mt-4 text-xs text-text-4">
            <button class="link-quiet" data-testid="onboarding-skip-back" @click="confirmingSkip = false">Back to connecting</button>
          </p>
        </template>

        <!-- A usable config is confirmed rather than silently adopted.
             "Usable" is not "present": without an agent or workspace, the
             session picker is as empty as it is with no file. -->
        <template v-else-if="card === 'hive' && hive">
          <template v-if="hive.usable.value">
            <p class="mb-6 text-center text-[13.5px] leading-relaxed text-text-3">
              Hive found your configuration and will use it as it is.
            </p>
            <div class="mb-6 rounded-[11px] border border-strong bg-chip px-4 py-3.5 text-left" data-testid="onboarding-hive-existing">
              <p class="truncate font-mono text-[12px] text-text-3">{{ hive.path.value }}</p>
              <p class="mt-2 text-[12.5px] text-text-2">
                Starts sessions with <span class="font-mono text-text">{{ hive.defaultAgent.value }}</span>
                across {{ hive.workspaces.value.length === 1 ? '1 folder' : `${hive.workspaces.value.length} folders` }} of repositories.
              </p>
            </div>
            <button
              class="primary-button"
              data-testid="onboarding-hive-continue"
              @click="emit('finishHive')"
            >Continue</button>
            <p class="mt-4 text-center text-xs text-text-4">You can change any of this later under Settings ▸ Hive CLI.</p>
          </template>

          <template v-else>
            <p class="mb-6 text-center text-[13.5px] leading-relaxed text-text-3">
              Hive starts coding sessions in your repositories. Tell it which agent to run and where your code lives.
            </p>
            <div class="mb-6 text-left">
              <HiveSetupForm
                :agents="hive.agents.value"
                :selected-agents="hive.selectedAgents.value"
                :workspaces="hive.workspaces.value"
                :default-agent="hive.defaultAgent.value"
                :skip-permissions="hive.skipPermissions.value"
                :custom-profiles="hive.customProfiles.value"
                :default-agent-override="hive.defaultAgentOverride.value"
                :busy="busy"
                @toggle-agent="hive.toggleAgent"
                @set-default-agent="hive.setDefaultAgent"
                @set-skip-permissions="hive.setSkipPermissions"
                @add-workspace="hive.addWorkspace"
                @add-workspace-path="hive.addWorkspacePath"
                @remove-workspace="hive.removeWorkspace"
              />
            </div>
            <button
              class="primary-button"
              :disabled="busy || !hive.canSave.value"
              data-testid="onboarding-hive-submit"
              @click="emit('saveHive')"
            >{{ busy ? 'Saving…' : 'Save and continue' }}</button>
            <p v-if="error" class="mt-4 text-center text-xs text-kind-issue" data-testid="onboarding-hive-error">{{ error }}</p>
            <p class="mt-4 text-center text-xs text-text-4">
              <button class="link-quiet underline" data-testid="onboarding-hive-skip" @click="emit('finishHive')">Skip for now</button>
              — the inbox works without this; the new session picker stays empty until you set it.
            </p>
          </template>
        </template>

        <!-- permissions: step 3, the OS notification grant -->
        <template v-else-if="card === 'permissions'">
          <template v-if="permission === 'granted'">
            <p class="mb-6 text-[13.5px] leading-relaxed text-text-3">Notifications are on. Hive will raise a system banner when activity needs you while you are working in another app.</p>
            <button
              class="primary-button"
              data-testid="onboarding-permissions-finish"
              @click="emit('finishPermissions')"
            >Continue</button>
          </template>
          <template v-else-if="permission === 'denied'">
            <p class="mb-6 text-[13.5px] leading-relaxed text-text-3" data-testid="onboarding-permissions-denied-guidance">Notifications are blocked. You can enable them for Hive in your operating system's notification settings whenever you like — until then, activity still lands in Activity and as in-app alerts while Hive is focused.</p>
            <button
              class="primary-button"
              data-testid="onboarding-permissions-finish"
              @click="emit('finishPermissions')"
            >Continue</button>
          </template>
          <template v-else>
            <p class="mb-6 text-[13.5px] leading-relaxed text-text-3">Hive can raise a system banner when new feed activity lands or a session finishes while you are working in another app. Turn it on so nothing slips by in the background.</p>
            <button
              class="primary-button"
              :disabled="busy"
              data-testid="onboarding-permissions-allow"
              @click="emit('requestPermission')"
            >{{ busy ? 'Requesting…' : 'Allow notifications' }}</button>
            <p v-if="error" class="mt-4 text-xs text-kind-issue" data-testid="onboarding-error">{{ error }}</p>
            <p class="mt-4 text-xs leading-relaxed text-text-4">
              Prefer to decide later? <button class="link-quiet underline" data-testid="onboarding-permissions-skip" @click="emit('finishPermissions')">Not now</button> — activity still shows in Activity and as in-app alerts, just without background banners until you enable them in Settings.
            </p>
          </template>
        </template>

        <!-- The Hive workspace includes the app's MCP servers and skills, so
             the agent can build profiles and feeds after the interview. -->
        <template v-else-if="card === 'agent'">
          <p class="mb-6 text-[13.5px] leading-relaxed text-text-3">
            Hive comes with a workspace where your coding agent can configure the app for you. It starts with a short interview about how you work, then sets up your profiles and feeds. Nothing changes without your say-so.
          </p>
          <button
            class="primary-button"
            :disabled="busy"
            data-testid="onboarding-agent-start"
            @click="emit('startAgent')"
          >{{ busy ? 'Starting…' : 'Start with your agent' }}</button>
          <p v-if="error" class="mt-4 text-xs text-kind-issue" data-testid="onboarding-error">{{ error }}</p>
          <p class="mt-4 text-xs leading-relaxed text-text-4">
            <button class="link-quiet underline" data-testid="onboarding-agent-skip" @click="emit('finishAgent')">Not now</button> — the Hive workspace stays in Chats whenever you want it.
          </p>
        </template>

        <!-- idle: not started -->
        <template v-else-if="card === 'idle'">
          <p class="mb-6 text-[13.5px] leading-relaxed text-text-3">Sign in from this device. Hive fills your profile with your open PRs, your assignments, and the notifications inbox.</p>
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

        <!-- Keep this branch explicit so an unknown card renders nothing
             instead of a token form under the wrong heading. -->
        <template v-else-if="card === 'token'">
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

        <!-- Bypassing GitHub is expected to be rare, so it sits below every
             connect card rather than beside the action that is the point. -->
        <p v-if="isConnectCard && !confirmingSkip" class="mt-2.5 text-xs text-text-4">
          <button class="link-quiet" data-testid="onboarding-skip" @click="confirmingSkip = true">Continue without GitHub</button>
        </p>
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
