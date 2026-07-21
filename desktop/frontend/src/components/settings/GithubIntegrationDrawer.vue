<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import IconGithub from '~icons/lucide/github'
import BaseButton from '../BaseButton.vue'
import DrawerSheet from '../DrawerSheet.vue'
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

onMounted(() => void load())
</script>

<template>
  <DrawerSheet ariaLabel="GitHub settings" testid="github-integration-drawer" backdrop-testid="github-integration-backdrop" :default-size="380" :min="320" :max="560" @close="emit('close')">
    <template #header>
      <div class="flex items-center gap-2.5">
        <span class="flex size-[26px] items-center justify-center rounded-[7px] bg-chip text-text-2"><IconGithub class="size-3.5" /></span>
        <div>
          <div class="text-[14px] font-semibold tracking-[-.01em]">GitHub settings</div>
          <div class="font-mono text-[11px] text-text-3">Integration polling</div>
        </div>
      </div>
    </template>

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

    <template #footer>
      <div class="flex items-center justify-end gap-2.5">
        <BaseButton
          variant="secondary"
          size="sm"
          data-testid="github-settings-cancel"
          @click="emit('close')"
        >Cancel</BaseButton>
        <BaseButton
          size="sm"
          :busy="loading || saving"
          :disabled="!valid"
          data-testid="github-settings-save"
          @click="save"
        >Save</BaseButton>
      </div>
    </template>
  </DrawerSheet>
</template>
