<script setup lang="ts">
import { computed } from 'vue'
import { Browser } from '@wailsio/runtime'
import IconExternalLink from '~icons/lucide/external-link'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import AppSwitch from './ui/AppSwitch.vue'
import InlineError from './ui/InlineError.vue'
import SettingsRow from './settings/SettingsRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import { useHiveCommand } from '../stores/useHiveCommand'

const hiveCLIDocsURL = 'https://colonyops.github.io/hive/'

const { status, error, saving, setInstall } = useHiveCommand()

const pathHint = computed(() => {
  const dir = status.value?.linkDir
  return dir ? `export PATH="${dir}:$PATH"` : ''
})

// A `hive` earlier on PATH than the app's own link, or one the app found at
// its install path, is reported and left alone.
const otherCommand = computed(() => {
  const s = status.value
  if (!s?.resolved || s.resolved === s.link.path) return ''
  return s.resolved
})

function openHiveCLIDocs(): void {
  void Browser.OpenURL(hiveCLIDocsURL)
}
</script>

<template>
  <SettingsSection title="Hive command" boxed testid="hive-command">
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

    <SettingsRow
      label="Install the hive command"
      :hint="
        status?.unsupported ||
        `Links ${status?.link.path ?? '~/.local/bin/hive'} to this app, so the hive in your terminal is always this version. Turning it off removes only the link Hive created.`
      "
      testid="hive-command-install"
    >
      <AppSwitch
        :model-value="status?.enabled ?? false"
        :disabled="!status || !!status.unsupported || saving"
        aria-label="Install the hive command"
        testid="hive-command-switch"
        @update:model-value="setInstall($event)"
      />
    </SettingsRow>

    <div
      v-if="status"
      class="flex flex-col gap-1.5 px-4 py-3.5 text-small text-text-2"
      data-testid="hive-command-versions"
    >
      <div class="flex justify-between gap-4">
        <span class="text-text-3">This app</span>
        <span class="font-mono" data-testid="hive-command-app-version">{{ status.appVersion }}</span>
      </div>
      <div class="flex justify-between gap-4">
        <span class="text-text-3">hive in your terminal</span>
        <span class="min-w-0 truncate font-mono" data-testid="hive-command-version">
          {{ status.resolved ? `${status.commandVersion || 'unknown version'} · ${status.resolved}` : 'not found' }}
        </span>
      </div>
    </div>

    <p
      v-if="status?.conflict"
      class="flex items-start gap-2 px-4 py-3.5 text-small leading-relaxed text-text-2"
      data-testid="hive-command-conflict"
    >
      <IconTriangleAlert class="mt-px size-3.5 shrink-0 text-severity-warning" />
      <span>
        <span class="font-mono text-text">{{ status.link.path }}</span> belongs to another install, so Hive left it
        alone. Remove it to let Hive manage the command.
      </span>
    </p>
    <p
      v-else-if="status?.link.appOwned && otherCommand"
      class="flex items-start gap-2 px-4 py-3.5 text-small leading-relaxed text-text-2"
      data-testid="hive-command-shadowed"
    >
      <IconTriangleAlert class="mt-px size-3.5 shrink-0 text-severity-warning" />
      <span>
        Your shell runs <span class="font-mono text-text">{{ otherCommand }}</span> first. Uninstall that copy (for
        Homebrew, <span class="font-mono text-text">brew uninstall --cask hive</span>) or put
        <span class="font-mono text-text">{{ status.linkDir }}</span> earlier on your PATH.
      </span>
    </p>
    <p
      v-if="status?.link.appOwned && !status.linkDirOnPath"
      class="flex items-start gap-2 px-4 py-3.5 text-small leading-relaxed text-text-2"
      data-testid="hive-command-not-on-path"
    >
      <IconTriangleAlert class="mt-px size-3.5 shrink-0 text-severity-warning" />
      <span>
        <span class="font-mono text-text">{{ status.linkDir }}</span> is not on your shell's PATH. Add
        <span class="font-mono text-text">{{ pathHint }}</span> to your shell profile.
      </span>
    </p>
    <p
      v-else-if="status?.versionsDiffer"
      class="flex items-start gap-2 px-4 py-3.5 text-small leading-relaxed text-text-2"
      data-testid="hive-command-version-differs"
    >
      <IconTriangleAlert class="mt-px size-3.5 shrink-0 text-severity-warning" />
      <span>The hive in your terminal is a different version from this app. Both use the same database.</span>
    </p>

    <InlineError v-if="error" :message="error" testid="hive-command-error" class="px-4 py-3.5" />
  </SettingsSection>
</template>
