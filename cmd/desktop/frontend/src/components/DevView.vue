<script setup lang="ts">
import { useIntervalFn } from '@vueuse/core'
import { computed, ref } from 'vue'
import IconBell from '~icons/lucide/bell'
import { Notify as NotifyNative } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/notificationservice'
import { useNotificationSettings } from '../composables/useNotificationSettings'
import { notifySeverityMapping, useNotify, type NotifySeverity } from '../composables/useNotify'
import { PAYLOAD_BYTES, useWailsLatency } from '../composables/useWailsLatency'
import { useToasts } from '../composables/useToasts'
import { formatBytes } from '../lib/bytes'
import AppCheckbox from './ui/AppCheckbox.vue'
import AppSelect from './ui/AppSelect.vue'
import BaseButton from './ui/BaseButton.vue'
import BaseCard from './ui/BaseCard.vue'
import SettingsField from './settings/SettingsField.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsSection from './settings/SettingsSection.vue'
import { useEscapeToClose } from '../composables/useEscapeToClose'
import ViewHeader from './ui/ViewHeader.vue'

const emit = defineEmits<{ close: [] }>()

type NotificationTestChannel = 'auto' | 'force-toast' | 'force-system'

type ResultTone = 'ok' | 'warn' | 'error' | 'muted'
interface TestResult {
  tone: ResultTone
  text: string
}

const severityOptions: Array<{ value: NotifySeverity; label: string }> = [
  { value: 'info', label: 'Info' },
  { value: 'success', label: 'Success' },
  { value: 'warning', label: 'Warning' },
  { value: 'error', label: 'Error' },
]

const channelOptions: Array<{ value: NotificationTestChannel; label: string; hint: string }> = [
  {
    value: 'auto',
    label: 'Follow app settings',
    hint: 'Records in Activity, then follows your delivery preference, window focus and OS permission.',
  },
  {
    value: 'force-toast',
    label: 'Force an in-app toast',
    hint: 'Shows a toast right now, bypassing settings, focus and Activity.',
  },
  {
    value: 'force-system',
    label: 'Force a system banner',
    hint: 'Asks the OS for a banner right now, bypassing settings, focus and Activity.',
  },
]

const resultToneClass: Record<ResultTone, string> = {
  ok: 'text-severity-success',
  warn: 'text-severity-warning',
  error: 'text-severity-error',
  muted: 'text-text-3',
}

const delaySeconds = 3

const severity = ref<NotifySeverity>('info')
const channel = ref<NotificationTestChannel>('auto')
const delay = ref(false)
const remaining = ref(0)
const result = ref<TestResult | null>(null)
const pending = computed(() => remaining.value > 0)

let sequence = 0

const { showToast } = useToasts()
const { notificationSound } = useNotificationSettings()

const testTitle = 'Test notification'
const testSubtitle = 'Hive Dev tools'

function testBody(selectedChannel: NotificationTestChannel): string {
  switch (selectedChannel) {
    case 'auto':
      return 'Dev tools auto test: uses focus and notification settings.'
    case 'force-toast':
      return 'Dev tools forced toast test: bypasses focus and Activity.'
    case 'force-system':
      return 'Dev tools forced system test: bypasses focus and Activity.'
  }
}

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

// useNotify picks between a toast, an OS banner and Activity-only from settings
// and focus, and returns none of that. Wrapping the two seams it already takes
// as injectable dependencies reports where the notification actually went
// without re-deriving that decision here, where it would drift.
async function deliverAuto(selectedSeverity: NotifySeverity, body: string): Promise<TestResult> {
  const surfaced: Array<'toast' | 'banner'> = []
  let bannerError: unknown

  const { notify } = useNotify({
    showToast: (message, options) => {
      surfaced.push('toast')
      return showToast(message, options)
    },
    osNotify: async (input) => {
      surfaced.push('banner')
      try {
        await NotifyNative(input)
      } catch (error) {
        bannerError = error
        throw error
      }
    },
  })

  try {
    await notify({ title: testTitle, body, severity: selectedSeverity, category: 'system', source: 'dev-view' })
  } catch (error) {
    return { tone: 'error', text: `Notification failed: ${errorText(error)}` }
  }

  switch (surfaced.at(-1)) {
    case 'banner':
      return { tone: 'ok', text: 'Recorded in Activity and shown as a system banner.' }
    case 'toast':
      return bannerError === undefined
        ? { tone: 'ok', text: 'Recorded in Activity and shown as an in-app toast.' }
        : {
            tone: 'warn',
            text: `Recorded in Activity. The system banner failed (${errorText(bannerError)}), so it fell back to an in-app toast.`,
          }
    default:
      return { tone: 'muted', text: 'Recorded in Activity only — notifications are switched off, so nothing surfaced.' }
  }
}

async function deliver(
  selectedChannel: NotificationTestChannel,
  selectedSeverity: NotifySeverity,
): Promise<TestResult> {
  const body = testBody(selectedChannel)

  if (selectedChannel === 'auto') return deliverAuto(selectedSeverity, body)

  if (selectedChannel === 'force-toast') {
    const id = showToast(testTitle, { body, severity: notifySeverityMapping[selectedSeverity].toast })
    return { tone: 'ok', text: `Toast #${id} shown in-app. Nothing was recorded in Activity.` }
  }

  try {
    await NotifyNative({
      title: testTitle,
      subtitle: testSubtitle,
      body,
      severity: selectedSeverity,
      sound: notificationSound.value,
      data: { source: 'dev-view', channel: 'force-system', severity: selectedSeverity },
    })
    return { tone: 'ok', text: 'The OS accepted a system banner. Nothing was recorded in Activity.' }
  } catch (error) {
    return { tone: 'error', text: `System banner failed: ${errorText(error)}` }
  }
}

async function dispatchTest(selectedChannel: NotificationTestChannel, selectedSeverity: NotifySeverity): Promise<void> {
  const token = ++sequence
  result.value = { tone: 'muted', text: 'Sending…' }
  const outcome = await deliver(selectedChannel, selectedSeverity)
  // A newer send owns the result line by the time a slower one resolves.
  if (token !== sequence) return
  result.value = outcome
}

let scheduledSend: (() => void) | null = null
const ticker = useIntervalFn(
  () => {
    remaining.value -= 1
    if (remaining.value > 0) return
    const send = scheduledSend
    stopCountdown()
    send?.()
  },
  1000,
  { immediate: false },
)

function stopCountdown(): void {
  scheduledSend = null

  ticker.pause()
  remaining.value = 0
}

function cancelPending(): void {
  if (!pending.value) return
  stopCountdown()
  result.value = { tone: 'muted', text: 'Scheduled test cancelled.' }
}

function sendTest(): void {
  stopCountdown()

  const selectedChannel = channel.value
  const selectedSeverity = severity.value
  if (!delay.value) {
    void dispatchTest(selectedChannel, selectedSeverity)
    return
  }

  // One ticker drives both the countdown and the send, so the number on screen
  // is the time actually left rather than a second clock beside it.
  result.value = null
  remaining.value = delaySeconds
  scheduledSend = () => void dispatchTest(selectedChannel, selectedSeverity)
  ticker.resume()
}

const { report: latency, measuring, error: latencyError, measure } = useWailsLatency()

// The header no longer carries a close button, so Escape is the way out.
useEscapeToClose(() => emit('close'))
</script>

<template>
  <div class="flex h-full min-h-0 flex-1 flex-col" data-testid="dev-view">
    <ViewHeader>
      <template #title>
        <span class="text-[13px] font-semibold text-text">Developer tools</span>
        <span class="font-mono text-[11px] text-text-4">internal</span>
      </template>
    </ViewHeader>

    <div class="hive-scroll @container/pane min-h-0 flex-1 overflow-y-auto px-6 py-6">
      <SettingsPage>
        <SettingsSection
          title="Wails round-trip"
          description="What one call across the frontend↔Go boundary costs, empty and carrying a payload."
          testid="dev-latency"
        >
          <div class="flex flex-col gap-3.5 rounded-[11px] border border-card bg-raised p-4">
            <div class="flex flex-col gap-3.5 @[420px]/pane:flex-row @[420px]/pane:items-center @[420px]/pane:gap-4">
              <span
                class="flex size-[34px] shrink-0 items-center justify-center rounded-[9px] border border-card bg-chip font-mono text-[13px] font-semibold text-accent"
                >ms</span
              >
              <div class="min-w-0 flex-1">
                <div class="text-[13.5px] font-semibold text-text">Measure the boundary</div>
                <div class="mt-0.5 text-xs leading-relaxed text-text-3">
                  40 calls each, issued one at a time after a warm-up, through the same bound-method plumbing every
                  service call uses.
                </div>
              </div>
              <BaseButton
                size="sm"
                class="shrink-0 self-start @[420px]/pane:self-auto"
                :busy="measuring"
                data-testid="dev-latency-measure"
                @click="measure"
              >
                {{ measuring ? 'Measuring…' : 'Measure' }}
              </BaseButton>
            </div>

            <div
              v-if="latency"
              class="grid grid-cols-2 overflow-hidden rounded-[9px] border border-card bg-raised"
              data-testid="dev-latency-results"
            >
              <div
                v-for="leg in [
                  { key: 'empty', label: 'Empty call', value: latency.empty },
                  { key: 'payload', label: `${formatBytes(PAYLOAD_BYTES)} payload`, value: latency.payload },
                ]"
                :key="leg.key"
                class="-ml-px flex flex-col gap-1.5 border-l border-border px-4 py-3"
                :data-testid="`dev-latency-${leg.key}`"
              >
                <div class="font-mono text-[10px] font-semibold uppercase tracking-[.12em] text-text-3">
                  {{ leg.label }}
                </div>
                <div class="font-mono text-[16px] tabular-nums text-text">
                  {{ leg.value.meanMs.toFixed(3) }}<span class="text-[11px] text-text-4">ms per call</span>
                </div>
                <div class="font-mono text-[11px] tabular-nums text-text-4">
                  {{ leg.value.minMs.toFixed(3) }}–{{ leg.value.maxMs.toFixed(3) }} per batch · n={{ leg.value.calls }}
                </div>
              </div>
            </div>

            <!-- Not a footnote for its own sake: at 1ms granularity a per-call
                 timing is 0 or 1 and nothing else, which is what the batching
                 exists to get around. -->
            <p v-if="latency" class="text-[11px] tabular-nums text-text-4" data-testid="dev-latency-resolution">
              Timed in batches of 20 — this webview's clock resolves to {{ latency.resolutionMs.toFixed(3) }}ms, coarser
              than one call.
            </p>

            <p v-if="latencyError" class="text-xs text-severity-error" data-testid="dev-latency-error">
              {{ latencyError }}
            </p>
          </div>
        </SettingsSection>

        <SettingsSection title="Notifications" description="Exercise notification delivery while developing Hive.">
          <BaseCard
            class="mt-4 items-start rounded-lg border border-border bg-raised"
            data-testid="dev-notification-test"
          >
            <template #icon>
              <span class="flex size-9 items-center justify-center rounded-lg bg-severity-info-tint text-severity-info"
                ><IconBell class="size-4"
              /></span>
            </template>
            <div class="@container/notify-test min-w-0 flex-1" data-testid="dev-notification-form">
              <div class="text-[13.5px] font-semibold text-text">Test notification</div>
              <div class="mt-0.5 text-xs text-text-3">Send one through a delivery path and see where it lands.</div>

              <div
                class="mt-4 grid grid-cols-1 gap-4 @[420px]/notify-test:grid-cols-2"
                data-testid="dev-notification-fields"
              >
                <SettingsField
                  label="Severity"
                  hint="Sets the toast accent and the banner's severity."
                  testid="dev-notification-severity-field"
                >
                  <AppSelect
                    size="sm"
                    class="w-full font-medium"
                    :model-value="severity"
                    :options="severityOptions"
                    testid="dev-notification-severity"
                    aria-label="Notification severity"
                    @update:model-value="severity = $event as NotifySeverity"
                  />
                </SettingsField>

                <SettingsField label="Timing">
                  <AppCheckbox
                    :model-value="delay"
                    label="Delay 3 seconds"
                    hint="Long enough to click away, so the send lands while Hive is unfocused."
                    testid="dev-notification-delay"
                    @update:model-value="delay = $event"
                  />
                </SettingsField>
              </div>

              <div class="mt-4">
                <div class="mb-1.5 text-[12.5px] text-text-2">Delivery channel</div>
                <div
                  class="flex flex-col gap-2"
                  role="radiogroup"
                  aria-label="Delivery channel"
                  data-testid="dev-notification-channel"
                >
                  <label
                    v-for="option in channelOptions"
                    :key="option.value"
                    class="flex cursor-pointer items-start gap-2.5"
                  >
                    <input
                      type="radio"
                      class="mt-0.5 accent-accent"
                      name="dev-notification-channel"
                      :value="option.value"
                      :checked="channel === option.value"
                      :data-testid="`dev-notification-channel-${option.value}`"
                      @change="channel = option.value"
                    />
                    <span>
                      <span class="block text-[12.5px] text-text">{{ option.label }}</span>
                      <span class="block text-[12px] text-text-3">{{ option.hint }}</span>
                    </span>
                  </label>
                </div>
              </div>

              <div
                class="mt-4 flex flex-col items-stretch gap-2 @[420px]/notify-test:flex-row @[420px]/notify-test:items-center"
                data-testid="dev-notification-actions"
              >
                <BaseButton size="sm" data-testid="dev-notification-send" @click="sendTest"
                  >Send test notification</BaseButton
                >
                <div class="min-w-0 text-xs leading-relaxed" role="status" aria-live="polite">
                  <span
                    v-if="pending"
                    class="flex items-center gap-2 text-text-2"
                    data-testid="dev-notification-pending"
                  >
                    <span>Sending in {{ remaining }}s…</span>
                    <button
                      type="button"
                      class="cursor-pointer rounded-md border border-border px-2 py-0.5 text-[11px] font-medium text-text-2 hover:bg-chip hover:text-text"
                      data-testid="dev-notification-cancel"
                      @click="cancelPending"
                    >
                      Cancel
                    </button>
                  </span>
                  <span
                    v-else-if="result"
                    :class="resultToneClass[result.tone]"
                    data-testid="dev-notification-result"
                    >{{ result.text }}</span
                  >
                </div>
              </div>
            </div>
          </BaseCard>
        </SettingsSection>
      </SettingsPage>
    </div>
  </div>
</template>
