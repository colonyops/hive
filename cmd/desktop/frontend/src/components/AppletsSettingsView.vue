<script setup lang="ts">
// Which applets Hive found, whether each loaded, and what they have running. The files
// under the applets directory are the registration, so this page only reads them, apart
// from importing a new one and deleting one.
import { useIntervalFn } from '@vueuse/core'
import { computed, onMounted, ref } from 'vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsRow from './settings/SettingsRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import ConfirmationHost from '@hive/ui/components/ui/ConfirmationHost.vue'
import CopyButton from '@hive/ui/components/ui/CopyButton.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { useConfirmation } from '@hive/ui/composables/useConfirmation'
import AppletExportDialog from './applets/AppletExportDialog.vue'
import AppletImportDialog from './applets/AppletImportDialog.vue'
import AppletSettingsRow from './applets/AppletSettingsRow.vue'
import {
  Dir,
  ExportPreview,
  PrepareImport,
  Processes,
  SpawnKill,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice'
import type {
  AppletExportPreview,
  AppletImportPreview,
  AppletProcess,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { useKeybindings } from '../composables/useKeybindings'
import { useSystemSettings } from '../composables/useSystemSettings'
import { BUILDER_ID, useApplets } from '../stores/useApplets'

const PROCESS_POLL_MS = 2000
const MAX_ARCHIVE_BYTES = 32 << 20

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

const importInput = ref<HTMLInputElement | null>(null)
const importPreview = ref<AppletImportPreview | null>(null)
const exportPreview = ref<AppletExportPreview | null>(null)
const shareError = ref('')
// Reading a large file takes a moment, and a second choice must not be overtaken by the first.
let latestImport = 0

function readBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve((reader.result as string).split(',')[1] ?? '')
    reader.onerror = () => reject(new Error('Could not read that file.'))
    reader.readAsDataURL(file)
  })
}

async function onImportFile(): Promise<void> {
  const input = importInput.value
  const file = input?.files?.[0]
  if (!input || !file) return
  const request = ++latestImport
  shareError.value = ''
  try {
    if (file.size > MAX_ARCHIVE_BYTES) throw new Error('This file is larger than the 32 MB limit.')
    const preview = await PrepareImport(await readBase64(file))
    if (request === latestImport) importPreview.value = preview
  } catch (e) {
    if (request === latestImport) shareError.value = e instanceof Error ? e.message : 'Could not read that file.'
  } finally {
    // So choosing the same file again still fires a change.
    input.value = ''
  }
}

async function startExport(id: string): Promise<void> {
  shareError.value = ''
  try {
    exportPreview.value = await ExportPreview(id)
  } catch (e) {
    shareError.value = e instanceof Error ? e.message : 'Could not export that applet.'
  }
}

const builderOn = computed(() => applets.applets.value.some((applet) => applet.id === BUILDER_ID && applet.enabled))

async function setEnabled(id: string, enabled: boolean): Promise<void> {
  shareError.value = ''
  try {
    await applets.setEnabled(id, enabled)
  } catch (e) {
    shareError.value = e instanceof Error ? e.message : 'Could not change that applet.'
  }
}

const keybindings = useKeybindings()
const deletion = useConfirmation()

function requestDelete(id: string, title: string): void {
  deletion.request({
    title: 'Delete applet',
    description: `Delete ${title}? This removes its folder and saved data from this machine, along with its title bar place and shortcuts. It cannot be undone.`,
    confirmLabel: 'Delete applet',
    testid: 'settings-applet-delete',
    onConfirm: async () => {
      await applets.remove(id)
      await keybindings.reload()
    },
  })
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
      description="Every folder under the applets folder with an applet.yaml. Changes load as they are saved. An applet that is off does not run or show in the palette. Bind one to a key in Keyboard shortcuts."
      boxed
    >
      <template #actions>
        <input
          ref="importInput"
          type="file"
          accept=".hiveapplet"
          class="hidden"
          data-testid="settings-applets-import-input"
          @change="onImportFile"
        />
        <BaseButton variant="secondary" size="sm" data-testid="settings-applets-import" @click="importInput?.click()"
          >Import</BaseButton
        >
        <BaseButton
          variant="secondary"
          size="sm"
          :disabled="!builderOn"
          data-testid="settings-applets-builder"
          @click="applets.toggle(BUILDER_ID, '')"
          >Build an applet</BaseButton
        >
      </template>
      <InlineError
        v-if="shareError"
        variant="line"
        class="px-4 py-3.5"
        :message="shareError"
        testid="settings-applets-share-error"
      />
      <AppletSettingsRow
        v-for="applet in applets.applets.value"
        :key="applet.id"
        :applet="applet"
        :running="running.get(applet.id) ?? 0"
        @open="applets.toggle(applet.id, '')"
        @export="startExport(applet.id)"
        @delete="requestDelete(applet.id, applet.title)"
        @enabled="setEnabled(applet.id, $event)"
      />
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
    <AppletImportDialog
      v-if="importPreview"
      :key="importPreview.token"
      :preview="importPreview"
      @close="importPreview = null"
    />
    <AppletExportDialog
      v-if="exportPreview"
      :key="exportPreview.token"
      :preview="exportPreview"
      @close="exportPreview = null"
    />
    <ConfirmationHost :confirmation="deletion" />
  </SettingsPage>
</template>
