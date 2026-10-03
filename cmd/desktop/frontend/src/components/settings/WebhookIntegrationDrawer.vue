<script setup lang="ts">
import InlineError from '../ui/InlineError.vue'
// Local webhook listener settings. Both controls here are startup-time
// decisions — the listener binds a port and serves flow-declared routes — so
// the drawer's job is to persist them and be honest about the pending
// restart rather than to pretend they applied live.
import { computed, onMounted, ref, watch } from 'vue'
import IconWebhook from '~icons/lucide/webhook'
import IconRefresh from '~icons/lucide/refresh-cw'
import AppSwitch from '../ui/AppSwitch.vue'
import BaseButton from '../ui/BaseButton.vue'
import DrawerSheet from '../ui/DrawerSheet.vue'
import FormField from '../ui/FormField.vue'
import TextInput from '../ui/TextInput.vue'
import { useWebhookSettings } from '../../stores/useWebhookSettings'
import CopyButton from '../ui/CopyButton.vue'

const emit = defineEmits<{ close: [] }>()

const { settings, loading, error, reload, save, generatePort } = useWebhookSettings()

const enabled = ref(true)
const port = ref('')
const saving = ref(false)

// The drawer edits a draft; the persisted view seeds it on load and after a
// save, so Cancel is simply "close without saving".
watch(
  settings,
  (value) => {
    if (!value) return
    enabled.value = value.enabled
    port.value = String(value.port)
  },
  { immediate: true },
)

const parsedPort = computed(() => Number(port.value))
const portValid = computed(
  () =>
    Number.isInteger(parsedPort.value) &&
    (parsedPort.value === 0 || (parsedPort.value >= 1024 && parsedPort.value <= 65535)),
)
const overridden = computed(() => settings.value?.portOverridden === true)
const portHint = computed(() =>
  overridden.value
    ? 'Fixed by HIVE_DESKTOP_HTTP_PORT for this session — edits here have no effect until the variable is unset.'
    : `Use 0 to allocate a port when the listener is enabled, or choose a stable port. Generated candidates come from ${settings.value?.portMin ?? 20000}–${settings.value?.portMax ?? 32767}.`,
)

const baseUrl = computed(() => settings.value?.baseUrl ?? '')
const dirty = computed(() => {
  if (!settings.value) return false
  return enabled.value !== settings.value.enabled || (portValid.value && parsedPort.value !== settings.value.port)
})
// A restart is pending when the saved configuration differs from the running
// listener, or when the draft has unsaved changes that will need one.
const restartPending = computed(() => settings.value?.restartRequired === true || dirty.value)

const status = computed(() => {
  const value = settings.value
  if (!value) return { tone: 'neutral', label: 'Unknown', detail: '' }
  if (value.startError) return { tone: 'error', label: 'Failed to start', detail: value.startError }
  if (value.running)
    return { tone: 'success', label: 'Running', detail: `Listening on ${value.host}:${value.boundPort}` }
  if (!value.enabled) return { tone: 'neutral', label: 'Disabled', detail: 'No local port is bound.' }
  return { tone: 'neutral', label: 'Not running', detail: 'Enabled but not started in this session.' }
})

const statusToneClass = computed(
  () =>
    ({
      success: 'border-severity-success-border bg-severity-success-tint text-severity-success',
      error: 'border-severity-error-border bg-severity-error-tint text-severity-error',
      neutral: 'border-border bg-chip text-text-2',
    })[status.value.tone as 'success' | 'error' | 'neutral'],
)

async function onGeneratePort() {
  const next = await generatePort()
  if (next !== undefined) port.value = String(next)
}

async function onSave() {
  if (!portValid.value || saving.value) return
  saving.value = true
  try {
    if (await save({ enabled: enabled.value, port: parsedPort.value })) emit('close')
  } finally {
    saving.value = false
  }
}

onMounted(() => void reload())
</script>

<template>
  <DrawerSheet
    title="Webhook settings"
    subtitle="Local listener"
    :icon="IconWebhook"
    testid="webhook-integration-drawer"
    backdrop-testid="webhook-integration-backdrop"
    :default-size="420"
    :min="340"
    :max="620"
    @close="emit('close')"
  >
    <div class="flex flex-col gap-5">
      <div
        class="flex items-center gap-3 rounded-lg border px-3 py-2.5"
        :class="statusToneClass"
        data-testid="webhook-settings-status"
      >
        <span class="size-2 shrink-0 rounded-full bg-current" />
        <div class="min-w-0 flex-1">
          <div class="text-small font-semibold">{{ status.label }}</div>
          <div v-if="status.detail" class="mt-0.5 break-words text-caption opacity-80">{{ status.detail }}</div>
        </div>
      </div>

      <AppSwitch
        v-model="enabled"
        label="Enable webhook listener"
        :hint="`Serves flow-declared /hooks/ routes on ${settings?.host ?? '127.0.0.1'}. Takes effect after restarting Hive.`"
        testid="webhook-settings-enabled"
      />

      <FormField v-slot="{ id }" label="Port" :hint="portHint" testid="webhook-settings-port">
        <div class="flex items-center gap-2">
          <TextInput
            :id="id"
            v-model="port"
            type="number"
            inputmode="numeric"
            min="1024"
            max="65535"
            step="1"
            data-testid="webhook-settings-port-input"
            :disabled="loading || overridden"
            size="sm"
            monospace
            :invalid="!portValid"
            class="min-w-0 flex-1"
          />
          <button
            type="button"
            class="flex size-[34px] shrink-0 cursor-pointer items-center justify-center rounded-lg border border-card text-text-3 hover:border-strong hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
            title="Pick a new random port"
            aria-label="Pick a new random port"
            data-testid="webhook-settings-port-generate"
            :disabled="loading || overridden"
            @click="onGeneratePort"
          >
            <IconRefresh class="size-[14px]" />
          </button>
        </div>
      </FormField>
      <InlineError v-if="!portValid" testid="webhook-settings-port-error" variant="line" class="-mt-3">
        Enter 0 for automatic allocation or a whole number between 1024 and 65535.
      </InlineError>

      <FormField
        v-if="baseUrl"
        label="Base URL"
        hint="Each sources.webhook node appends its own path to this."
        testid="webhook-settings-base-url"
      >
        <div class="flex items-center gap-2">
          <code
            class="min-w-0 flex-1 truncate rounded-lg border border-strong bg-app px-3 py-2 font-mono text-small text-text-2"
            data-testid="webhook-settings-base-url-value"
            >{{ baseUrl }}</code
          >
          <CopyButton :text="baseUrl" data-testid="webhook-settings-copy-url" />
        </div>
      </FormField>

      <p
        v-if="restartPending"
        class="rounded-lg border border-border bg-severity-info-tint px-3 py-2.5 text-small leading-relaxed text-text-2"
        data-testid="webhook-settings-restart-note"
      >
        Restart Hive to apply the listener's enabled state and port.
      </p>

      <InlineError v-if="error" :message="error" testid="webhook-settings-error" />
    </div>

    <template #footer>
      <div class="flex items-center justify-end gap-2.5">
        <BaseButton variant="secondary" size="sm" data-testid="webhook-settings-cancel" @click="emit('close')"
          >Cancel</BaseButton
        >
        <BaseButton
          size="sm"
          :busy="loading || saving"
          :disabled="!portValid"
          data-testid="webhook-settings-save"
          @click="onSave"
          >Save</BaseButton
        >
      </div>
    </template>
  </DrawerSheet>
</template>
