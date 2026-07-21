<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import IconGithub from '~icons/lucide/github'
import SettingsField from './SettingsField.vue'
import * as SettingsService from '../../../bindings/github.com/colonyops/hive/desktop/settingsservice'

const emit = defineEmits<{ close: [] }>()

const pollIntervalSeconds = ref('60')
const minPollIntervalSeconds = ref(60)
const loading = ref(true)
const saving = ref(false)
const error = ref('')

const parsedInterval = computed(() => Number(pollIntervalSeconds.value))
const valid = computed(() => Number.isInteger(parsedInterval.value) && parsedInterval.value >= minPollIntervalSeconds.value)
const minimumHint = computed(() => `Minimum ${minPollIntervalSeconds.value}s — GitHub's polling contract`)

function messageFor(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const settings = await SettingsService.GithubSettings()
    pollIntervalSeconds.value = String(settings.pollIntervalSeconds)
    minPollIntervalSeconds.value = settings.minPollIntervalSeconds
  } catch (err) {
    error.value = messageFor(err)
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!valid.value || saving.value) return
  saving.value = true
  error.value = ''
  try {
    await SettingsService.SetGithubSettings({
      pollIntervalSeconds: parsedInterval.value,
      minPollIntervalSeconds: minPollIntervalSeconds.value,
    })
    emit('close')
  } catch (err) {
    error.value = messageFor(err)
  } finally {
    saving.value = false
  }
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') emit('close')
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  void load()
})
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-40 bg-backdrop" data-testid="github-integration-backdrop" @click="emit('close')" />
    <aside
      class="fixed inset-y-0 right-0 z-40 flex w-[380px] max-w-full flex-col overflow-hidden border-l border-strong bg-pane text-text shadow-[-30px_0_60px_-20px_rgba(0,0,0,.5)]"
      role="dialog"
      aria-label="GitHub settings"
      aria-modal="true"
      data-testid="github-integration-drawer"
    >
      <header class="flex shrink-0 items-center gap-2.5 border-b border-row bg-pane px-[18px] py-[15px]">
        <span class="flex size-[26px] items-center justify-center rounded-[7px] bg-chip text-text-2"><IconGithub class="size-3.5" /></span>
        <div>
          <div class="text-[14px] font-semibold tracking-[-.01em]">GitHub settings</div>
          <div class="font-mono text-[11px] text-text-3">Integration polling</div>
        </div>
      </header>

      <div class="hive-scroll min-h-0 flex-1 overflow-y-auto px-[18px] py-[15px]">
        <SettingsField label="Poll interval" :hint="minimumHint" testid="github-poll-interval">
          <div class="relative">
            <input
              v-model="pollIntervalSeconds"
              type="number"
              inputmode="numeric"
              :min="minPollIntervalSeconds"
              step="1"
              class="w-full rounded-lg border border-strong bg-app px-[11px] py-[9px] pr-16 text-[13px] text-text outline-none focus:border-accent"
              :class="!valid ? 'border-severity-error' : ''"
              data-testid="github-poll-interval-input"
              :disabled="loading"
            >
            <span class="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-text-3">seconds</span>
          </div>
        </SettingsField>
        <p v-if="!valid" class="mt-2 text-xs text-severity-error" data-testid="github-poll-interval-error">Enter a whole number of at least {{ minPollIntervalSeconds }} seconds.</p>
        <p v-if="error" class="mt-4 rounded-md border border-severity-error/40 bg-severity-error-tint px-3 py-2 text-xs text-severity-error" data-testid="github-settings-error">{{ error }}</p>
      </div>

      <footer class="flex shrink-0 items-center justify-end gap-2.5 border-t border-row bg-raised px-[18px] py-[13px]">
        <button
          type="button"
          class="cursor-pointer rounded-lg border border-card px-[15px] py-2 text-[13px] text-text-2 hover:text-text"
          data-testid="github-settings-cancel"
          @click="emit('close')"
        >Cancel</button>
        <button
          type="button"
          class="cursor-pointer rounded-lg bg-accent px-[18px] py-2 text-[13px] font-semibold text-accent-contrast hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-50"
          data-testid="github-settings-save"
          :disabled="loading || saving || !valid"
          @click="save"
        >Save</button>
      </footer>
    </aside>
  </Teleport>
</template>
