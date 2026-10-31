<script setup lang="ts">
// Which applets Hive found, whether each loaded, and what they have running. The files
// under the applets directory are the registration, so this page reads them and does
// not edit them.
import { useIntervalFn } from '@vueuse/core'
import { computed, onMounted, ref, type DeepReadonly } from 'vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsRow from './settings/SettingsRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import AppSwitch from '@hive/ui/components/ui/AppSwitch.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import CopyButton from '@hive/ui/components/ui/CopyButton.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import {
  Dir,
  Processes,
  SpawnKill,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice'
import type { Applet, AppletProcess } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { useSystemSettings } from '../composables/useSystemSettings'
import { useApplets } from '../stores/useApplets'

const PROCESS_POLL_MS = 2000
const BUILDER_ID = 'applet-builder'

const applets = useApplets()
const { revealPath } = useSystemSettings()
const dir = ref('')
const processes = ref<AppletProcess[]>([])
const processError = ref<string | null>(null)

const running = computed(() => {
  const counts = new Map<string, number>()
  for (const process of processes.value) counts.set(process.applet, (counts.get(process.applet) ?? 0) + 1)
  return counts
})

async function loadDir(): Promise<void> {
  try {
    dir.value = (await Dir()) ?? ''
  } catch (e) {
    processError.value = e instanceof Error ? e.message : 'Could not read the applets folder.'
  }
}

async function pollProcesses(): Promise<void> {
  try {
    processes.value = (await Processes()) ?? []
    processError.value = null
  } catch (e) {
    processError.value = e instanceof Error ? e.message : 'Could not read the running processes.'
  }
}

async function stop(process: AppletProcess): Promise<void> {
  try {
    await SpawnKill(process.handle)
  } catch (e) {
    processError.value = e instanceof Error ? e.message : 'Could not stop the process.'
  }
  await pollProcesses()
}

function status(applet: DeepReadonly<Applet>): { tone: 'success' | 'danger' | 'accent'; label: string } {
  if (!applet.runnable) return { tone: 'danger', label: 'Invalid' }
  if (applet.error) return { tone: 'accent', label: 'Running last good file' }
  return { tone: 'success', label: 'Ready' }
}

function hint(applet: DeepReadonly<Applet>): string {
  const parts = [`applet.${applet.id}`]
  if (applet.builtin) parts.push('ships with Hive')
  const count = running.value.get(applet.id)
  if (count) parts.push(`${count} running`)
  return parts.join(' · ')
}

function startedAt(process: AppletProcess): string {
  return new Date(process.startedMillis).toLocaleTimeString()
}

useIntervalFn(() => void pollProcesses(), PROCESS_POLL_MS)

onMounted(() => {
  void applets.reload()
  void pollProcesses()
  void loadDir()
})
</script>

<template>
  <SettingsPage testid="settings-applets">
    <SettingsSection
      title="Registered applets"
      description="Every directory under the applets folder with an applet.yaml. Hive picks up changes as they are saved. Bind one to a key in Keyboard shortcuts."
      boxed
    >
      <template #actions>
        <BaseButton
          variant="secondary"
          size="sm"
          data-testid="settings-applets-builder"
          @click="applets.toggle(BUILDER_ID, '')"
          >Build an applet</BaseButton
        >
      </template>
      <SettingsRow
        v-for="applet in applets.applets.value"
        :key="applet.id"
        :label="applet.title"
        :hint="applet.error || hint(applet)"
        :testid="`settings-applet-${applet.id}`"
      >
        <div class="flex items-center gap-2">
          <BaseBadge
            :tone="status(applet).tone"
            variant="pill"
            dot
            class="px-2.5 py-0.5 text-caption font-medium"
            :data-testid="`settings-applet-${applet.id}-state`"
            >{{ status(applet).label }}</BaseBadge
          >
          <AppSwitch
            v-if="applet.titlebar"
            size="sm"
            label="Title bar"
            :model-value="applets.isInTitlebar(applet.id)"
            :disabled="!applet.runnable"
            :testid="`settings-applet-${applet.id}-titlebar`"
            @update:model-value="applets.setInTitlebar(applet.id, $event)"
          />
          <BaseButton
            variant="secondary"
            size="xs"
            :data-testid="`settings-applet-${applet.id}-open`"
            @click="applets.toggle(applet.id, '')"
            >Open</BaseButton
          >
        </div>
      </SettingsRow>
    </SettingsSection>

    <SettingsSection
      title="Running processes"
      description="Long-running commands started by applets. Closing a panel or dropdown stops what it started, and a title bar page's keep running while it is in the title bar."
      boxed
    >
      <InlineError v-if="processError" variant="line" class="px-4 py-3.5" :message="processError" />
      <EmptyState
        v-else-if="!processes.length"
        variant="inline"
        class="px-4 py-3.5"
        message="No applet has a process running."
        data-testid="settings-applets-processes-empty"
      />
      <SettingsRow
        v-for="process in processes"
        :key="process.handle"
        :label="process.applet"
        :hint="`Since ${startedAt(process)}: ${process.command}`"
        :testid="`settings-applet-process-${process.handle}`"
      >
        <BaseButton
          variant="secondary"
          size="xs"
          :data-testid="`settings-applet-process-${process.handle}-stop`"
          @click="stop(process)"
          >Stop</BaseButton
        >
      </SettingsRow>
    </SettingsSection>

    <SettingsSection title="Location" boxed>
      <SettingsRow label="Applets folder" :hint="dir" testid="settings-applets-dir">
        <div class="flex items-center gap-2">
          <CopyButton v-if="dir" :text="dir" size="xs" />
          <BaseButton
            variant="secondary"
            size="xs"
            :disabled="!dir"
            data-testid="settings-applets-reveal"
            @click="revealPath(dir)"
            >Reveal</BaseButton
          >
        </div>
      </SettingsRow>
    </SettingsSection>
  </SettingsPage>
</template>
