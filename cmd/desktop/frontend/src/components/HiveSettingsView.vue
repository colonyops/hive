<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { Browser } from '@wailsio/runtime'
import IconAlertTriangle from '~icons/lucide/alert-triangle'
import IconCopy from '~icons/lucide/copy'
import IconExternalLink from '~icons/lucide/external-link'
import IconFilePlus from '~icons/lucide/file-plus'
import IconFolderOpen from '~icons/lucide/folder-open'
import IconInfo from '~icons/lucide/info'
import AppSwitch from '@hive/ui/components/ui/AppSwitch.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsPathRow from './settings/SettingsPathRow.vue'
import SettingsRow from './settings/SettingsRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import { useClipboard } from '@hive/ui/composables/useClipboard'
import { useCommands, type Command } from '../composables/useCommands'
import { Owner, Skip } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/hivecli/models'
import { useHiveCLI } from '../composables/useHiveCLI'
import { useHiveSetup } from '../composables/useHiveSetup'
import { useSystemSettings } from '../composables/useSystemSettings'

const { info, error, refresh, openPath, revealPath, createOrOpenHiveConfig } = useSystemSettings()
// Settings reports parse failures but does not write the config; first run is
// the only writer.
const hive = useHiveSetup()
const cli = useHiveCLI()
const { copy } = useClipboard()

const exportLine = 'export PATH="$HOME/.local/bin:$PATH"'

// Why the switch is off the table, from the reason the app is not managing the
// command. Empty when it is.
const unmanagedReason = computed(() => {
  const cmd = cli.command.value
  if (!cmd) return ''
  switch (cmd.skip) {
    case Skip.SkipDev:
      return 'Development builds do not install the command.'
    case Skip.SkipTranslocated:
      return 'macOS is running Hive from a temporary location. Move Hive to your Applications folder, open it from there, then turn this on.'
    case Skip.SkipUnrecognizable:
      return 'This copy of Hive cannot install the command.'
    default:
      return ''
  }
})

const installHint = computed(
  () =>
    unmanagedReason.value ||
    'Adds hive to ~/.local/bin. It is this app, so it is always the same version and updates with it.',
)

const ownerText = computed(() => {
  switch (cli.command.value?.owner) {
    case Owner.OwnerApp:
      return 'Installed by Hive.'
    case Owner.OwnerForeign:
      return 'A hive you installed another way is here. Hive leaves it alone.'
    default:
      return 'Not installed.'
  }
})

const hiveCLIDocsURL = 'https://colonyops.github.io/hive/'

function openHiveCLIDocs(): void {
  void Browser.OpenURL(hiveCLIDocsURL)
}

useCommands(() => {
  const config = info.value?.hiveConfig
  if (!config) return []

  const commands: Command[] = [
    {
      id: 'hive:config:copy',
      title: 'Copy config path',
      group: 'Hive CLI config',
      scope: 'actions',
      icon: IconCopy,
      run: () => void copy(config.path),
    },
  ]

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
  void cli.load()
})
</script>

<template>
  <SettingsPage testid="settings-hive">
    <InlineError v-if="error" :message="error" testid="hive-settings-error" />
    <InlineError
      v-if="hive.unreadable.value"
      :message="`This config could not be read: ${hive.unreadable.value}. Fix it in your editor, then restart Hive Desktop.`"
      testid="hive-unreadable"
    />

    <SettingsSection
      title="Hive command"
      description="Run hive from any terminal. Hive Desktop and the command share one configuration file."
      boxed
      testid="hive-command"
    >
      <template #actions>
        <button
          type="button"
          class="flex cursor-pointer items-center gap-1.5 text-small font-medium text-accent hover:underline"
          data-testid="hive-cli-docs"
          @click="openHiveCLIDocs"
        >
          Hive CLI documentation
          <IconExternalLink class="size-3" />
        </button>
      </template>
      <InlineError v-if="cli.error.value" :message="cli.error.value" testid="hive-command-error" class="m-4" />
      <template v-if="cli.command.value">
        <SettingsRow label="Install the hive command" :hint="installHint" testid="hive-command-row">
          <AppSwitch
            :model-value="cli.status.value?.install === true"
            :disabled="!cli.managed.value || cli.saving.value"
            aria-label="Install the hive command"
            testid="hive-command-install"
            @update:model-value="cli.setInstall"
          />
        </SettingsRow>
        <div
          v-if="cli.command.value.skip === Skip.SkipUnwritable"
          class="flex items-start gap-3 px-4 py-3.5 text-small leading-relaxed text-text-2"
          data-testid="hive-command-unwritable"
        >
          <IconAlertTriangle class="mt-0.5 size-4 shrink-0 text-severity-warning" />
          <span>Hive could not write {{ cli.command.value.installPath }}: {{ cli.command.value.detail }}</span>
        </div>
        <div class="flex flex-col gap-1.5 px-4 py-3.5 text-small leading-5">
          <div class="flex items-baseline justify-between gap-4">
            <span class="text-text-3">Location</span>
            <span class="min-w-0 truncate font-mono text-text-2" data-testid="hive-command-path">{{
              cli.command.value.installPath
            }}</span>
          </div>
          <p class="text-text-3" data-testid="hive-command-owner">{{ ownerText }}</p>
        </div>
        <div class="flex flex-col gap-1.5 px-4 py-3.5 text-small leading-5">
          <div class="flex items-baseline justify-between gap-4">
            <span class="text-text-3">Hive Desktop version</span>
            <span class="font-mono text-text-2" data-testid="hive-command-app-version">{{
              cli.command.value.appVersion
            }}</span>
          </div>
          <div class="flex items-baseline justify-between gap-4">
            <span class="text-text-3">hive in your terminal</span>
            <span class="min-w-0 truncate font-mono text-text-2" data-testid="hive-command-version">
              <template v-if="cli.command.value.resolved">
                {{ cli.command.value.commandVersion || 'unknown version' }}
                <span class="text-text-4">({{ cli.command.value.resolved }})</span>
              </template>
              <template v-else>not found</template>
            </span>
          </div>
        </div>
        <div
          v-if="cli.command.value.resolved && cli.command.value.differs"
          class="flex items-start gap-3 px-4 py-3.5 text-small leading-relaxed text-text-2"
          data-testid="hive-command-differs"
        >
          <IconAlertTriangle class="mt-0.5 size-4 shrink-0 text-severity-warning" />
          <span>
            The hive in your terminal is not the version of this app. Both share one database, and the older one may not
            understand changes the newer one made.
          </span>
        </div>
        <div
          v-if="cli.command.value.owner === Owner.OwnerApp && !cli.command.value.onPath"
          class="flex flex-col gap-2 px-4 py-3.5 text-small leading-relaxed text-text-2"
          data-testid="hive-command-path-hint"
        >
          <span>
            ~/.local/bin is not on your shell's PATH, so a terminal cannot find hive yet. Add this line to your shell
            profile:
          </span>
          <span class="flex items-center gap-2">
            <code class="rounded-md bg-chip px-2 py-1 font-mono text-text">{{ exportLine }}</code>
            <button
              type="button"
              class="cursor-pointer text-accent hover:underline"
              data-testid="hive-command-copy-path"
              @click="copy(exportLine)"
            >
              Copy
            </button>
          </span>
        </div>
        <div
          v-if="cli.command.value.owner === Owner.OwnerApp && cli.command.value.shadowed"
          class="px-4 py-3.5 text-small leading-relaxed text-text-2"
          data-testid="hive-command-shadowed"
        >
          Another hive, at <span class="font-mono text-text">{{ cli.command.value.resolved }}</span
          >, comes first on your PATH. To use Hive's command, remove it. If you installed it with Homebrew, run
          <code class="rounded-md bg-chip px-1.5 py-0.5 font-mono text-text">brew uninstall --cask hive</code>. If you
          used <code class="rounded-md bg-chip px-1.5 py-0.5 font-mono text-text">go install</code>, delete that file.
        </div>
      </template>
    </SettingsSection>

    <SettingsSection
      v-if="info"
      title="Shared Hive data"
      description="Hive Desktop and the Hive CLI use this directory for sessions, task coordination, context, and logs."
      boxed
    >
      <SettingsPathRow
        label="Hive data directory"
        hint="HIVE_DESKTOP_HIVE_DATA_DIR or HIVE_DATA_DIR selects this location."
        icon="folder"
        tone="accent"
        :path="info.hiveDataDir.path"
        :exists="info.hiveDataDir.exists"
        testid="hive-data-dir"
        @open="openPath(info.hiveDataDir.path)"
        @reveal="revealPath(info.hiveDataDir.path)"
      />
    </SettingsSection>

    <SettingsSection v-if="info" title="Configuration file" description="The same file the hive CLI reads." boxed>
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
      <div class="flex items-center gap-3 px-4 py-3.5" data-testid="hive-restart-notice">
        <IconInfo class="size-4 shrink-0 text-severity-info" />
        <div class="text-small leading-relaxed text-text-2">
          Edit this file in your own editor — the Hive CLI documentation above describes every key. Hive Desktop reads
          it at startup, so restart the app to apply a change.
        </div>
      </div>
      <div
        v-if="!info.hiveConfig.exists"
        class="flex flex-col items-start gap-3 px-4 py-3.5 @[600px]/pane:flex-row @[600px]/pane:items-center"
        data-testid="hive-config-missing"
      >
        <p class="min-w-0 flex-1 text-small leading-5 text-text-3">
          No configuration file yet. Hive uses built-in defaults until you create one.
        </p>
        <button
          type="button"
          class="shrink-0 cursor-pointer rounded-lg border border-accent/45 bg-accent-tint px-3 py-1.5 text-small font-medium text-accent hover:border-accent"
          data-testid="hive-config-create"
          @click="createOrOpenHiveConfig"
        >
          Create and open
        </button>
      </div>
    </SettingsSection>
  </SettingsPage>
</template>
