<script setup lang="ts">
import { onMounted } from 'vue'
import { Browser } from '@wailsio/runtime'
import IconCopy from '~icons/lucide/copy'
import IconExternalLink from '~icons/lucide/external-link'
import IconFilePlus from '~icons/lucide/file-plus'
import IconFolderOpen from '~icons/lucide/folder-open'
import IconInfo from '~icons/lucide/info'
import SettingsError from './settings/SettingsError.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsPathRow from './settings/SettingsPathRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import { useClipboard } from '../composables/useClipboard'
import { useCommands, type Command } from '../composables/useCommands'
import { useHiveSetup } from '../composables/useHiveSetup'
import { useSystemSettings } from '../composables/useSystemSettings'

const {
  info,
  error,
  refresh,
  openPath,
  revealPath,
  createOrOpenHiveConfig,
} = useSystemSettings()
// Settings reports parse failures but does not write the config; first run is
// the only writer.
const hive = useHiveSetup()
const { copy } = useClipboard()

const hiveCLIDocsURL = 'https://colonyops.github.io/hive/'

function openHiveCLIDocs(): void {
  void Browser.OpenURL(hiveCLIDocsURL)
}

useCommands(() => {
  const config = info.value?.hiveConfig
  if (!config) return []

  const commands: Command[] = [{
    id: 'hive:config:copy',
    title: 'Copy config path',
    group: 'Hive CLI config',
    scope: 'actions',
    icon: IconCopy,
    run: () => void copy(config.path),
  }]

  if (!config.exists) {
    commands.push({
      id: 'hive:config:create',
      title: 'Create and open config',
      group: 'Hive CLI config',
      scope: 'actions',
      icon: IconFilePlus,
      run: () => void createOrOpenHiveConfig(),
    })
    return commands
  }

  commands.push(
    {
      id: 'hive:config:open',
      title: 'Open config',
      group: 'Hive CLI config',
      scope: 'actions',
      icon: IconExternalLink,
      run: () => void openPath(config.path),
    },
    {
      id: 'hive:config:reveal',
      title: 'Reveal config',
      group: 'Hive CLI config',
      scope: 'actions',
      icon: IconFolderOpen,
      run: () => void revealPath(config.path),
    },
  )
  return commands
})

onMounted(() => {
  void refresh()
  void hive.load()
})
</script>

<template>
  <SettingsPage testid="settings-hive">
    <SettingsError v-if="error" :message="error" testid="hive-settings-error" />
    <SettingsError v-if="hive.unreadable.value" :message="`This config could not be read: ${hive.unreadable.value}. Fix it in your editor, then restart Hive Desktop.`" testid="hive-unreadable" />

    <SettingsSection title="Included Hive runtime" boxed padded>
      <template #actions>
        <button
          type="button"
          class="flex cursor-pointer items-center gap-1.5 text-[12px] font-medium text-accent hover:underline"
          data-testid="hive-cli-docs"
          @click="openHiveCLIDocs"
        >
          Hive CLI documentation
          <IconExternalLink class="size-3" />
        </button>
      </template>
      <div class="flex flex-col gap-2 text-[12.5px] leading-5 text-text-2">
        <p>Hive Desktop includes the Hive runtime it needs. It does not require or invoke a separately installed Hive CLI.</p>
        <p>If you use the Hive CLI, Desktop shares this configuration file with it and leaves everything it does not ask about alone.</p>
      </div>
    </SettingsSection>

    <SettingsSection
      v-if="info"
      title="Configuration file"
      description="The same file the hive CLI reads."
      boxed
    >
      <SettingsPathRow
        label="Hive CLI config"
        hint="Agent profiles, the default agent, workspaces, tmux, and other Hive behavior."
        icon="log"
        tone="accent"
        :path="info.hiveConfig.path"
        :exists="info.hiveConfig.exists"
        :overridden="info.hiveConfig.overridden"
        overridden-label="HIVE_CONFIG"
        :can-open="info.hiveConfig.exists"
        :can-reveal="info.hiveConfig.exists"
        testid="hive-config"
        @open="openPath(info.hiveConfig.path)"
        @reveal="revealPath(info.hiveConfig.path)"
      />
      <div
        class="flex items-center gap-3 px-4 py-3.5"
        data-testid="hive-restart-notice"
      >
        <IconInfo class="size-4 shrink-0 text-severity-info" />
        <div class="text-[12.5px] leading-relaxed text-text-2">
          Edit this file in your own editor — the Hive CLI documentation above describes every key. Hive Desktop reads it at startup, so restart the app to apply a change.
        </div>
      </div>
      <div
        v-if="!info.hiveConfig.exists"
        class="flex flex-col items-start gap-3 px-4 py-3.5 @[600px]/pane:flex-row @[600px]/pane:items-center"
        data-testid="hive-config-missing"
      >
        <p class="min-w-0 flex-1 text-[12px] leading-5 text-text-3">
          No configuration file yet. Hive uses built-in defaults until you create one.
        </p>
        <button
          type="button"
          class="shrink-0 cursor-pointer rounded-[7px] border border-accent/45 bg-accent-tint px-3 py-1.5 text-[12.5px] font-medium text-accent hover:border-accent"
          data-testid="hive-config-create"
          @click="createOrOpenHiveConfig"
        >Create and open</button>
      </div>
    </SettingsSection>
  </SettingsPage>
</template>
