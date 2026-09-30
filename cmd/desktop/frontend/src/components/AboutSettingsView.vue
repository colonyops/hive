<script setup lang="ts">
// About: which build is running, where it came from, and the auto-update
// controls that replace it. Not a settings group — it is the answer to "what
// am I running", which is why it is its own pane rather than a card at the
// bottom of System, and why it reads as a dashboard: identity and update state
// at the top, one card per build fact, then the places to take them.
//
// Nothing here links to source: the repository is private, so a commit or tag
// page is a 404 for everyone but its author. The build facts are made
// copyable instead, since quoting them in a report is what they are for.
//
// Installing an available update stays with the title-bar chip, which owns the
// confirm-and-relaunch flow; this pane checks and points at it.
import { computed, onMounted, ref, type Component } from 'vue'
import IconCalendar from '~icons/lucide/calendar-days'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconCpu from '~icons/lucide/cpu'
import IconExternalLink from '~icons/lucide/external-link'
import IconGitCommit from '~icons/lucide/git-commit-horizontal'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconTag from '~icons/lucide/tag'
import AppSwitch from './AppSwitch.vue'
import BaseBadge from './BaseBadge.vue'
import HiveMark from './marks/HiveMark.vue'
import ReleaseNoteBody from './ReleaseNoteBody.vue'
import SettingsError from './settings/SettingsError.vue'
import SettingsPage from './settings/SettingsPage.vue'
import SettingsRow from './settings/SettingsRow.vue'
import SettingsSection from './settings/SettingsSection.vue'
import { relativeTimeLabel } from '../lib/age'
import { useAboutSettings } from '../composables/useAboutSettings'
import { useClipboard } from '../composables/useClipboard'
import { useReportDialog } from '../composables/useReportDialog'
import { useReleaseNotes } from '../composables/useReleaseNotes'
import type { ReleaseNote } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const {
  build,
  update,
  autoUpdate,
  checking,
  error,
  released,
  refresh,
  setAutoUpdate,
  checkForUpdates,
  openDocs,
  openUpdatesDoc,
} = useAboutSettings()

const { copy, copied } = useClipboard()
const { reportProblem } = useReportDialog()

const osNames: Record<string, string> = { darwin: 'macOS', windows: 'Windows', linux: 'Linux' }

const channelLabel = computed(() => build.value?.channel || 'unreleased')

// The four states the pane can be in, in the order they outrank each other: a
// pending update first, then a build self-update cannot reach at all, then
// whether a check has ever landed (a background poll counts).
const status = computed<{ label: string; tone: 'accent' | 'success' | 'neutral' }>(() => {
  if (update.value?.available) return { label: 'Update available', tone: 'accent' }
  if (!released.value) return { label: 'Unreleased build', tone: 'neutral' }
  if (update.value?.checkedAt) return { label: 'Up to date', tone: 'success' }
  return { label: 'Not checked yet', tone: 'neutral' }
})

const checkedLabel = computed(() => {
  if (!released.value) return 'Self-update is off for builds without a published release.'
  const at = update.value?.checkedAt
  if (!at) return 'No update check has run yet.'
  const time = new Date(at)
  return Number.isNaN(time.getTime()) ? '' : `Checked ${relativeTimeLabel(time.getTime())}.`
})

interface Stat {
  key: string
  icon: Component
  label: string
  value: string
  hint: string
}

const stats = computed<Stat[]>(() => {
  const info = build.value
  if (!info) return []
  const built = new Date(info.date)
  const dated = !Number.isNaN(built.getTime())
  return [
    {
      key: 'version',
      icon: IconTag,
      label: 'Version',
      value: info.version,
      hint: `${channelLabel.value} channel`,
    },
    {
      key: 'commit',
      icon: IconGitCommit,
      label: 'Commit',
      value: info.commit,
      hint: /^[0-9a-f]{7,40}$/.test(info.commit) ? 'Revision this build was cut from' : 'Not stamped',
    },
    {
      key: 'date',
      icon: IconCalendar,
      label: 'Built',
      value: dated ? built.toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' }) : info.date,
      hint: dated ? relativeTimeLabel(built.getTime()) : 'Built from source',
    },
    {
      key: 'platform',
      icon: IconCpu,
      label: 'Platform',
      value: `${osNames[info.os] ?? info.os} · ${info.arch}`,
      hint: info.goVersion,
    },
  ]
})

// Three lines rather than one, because its destination is an issue body.
const buildSummary = computed(() => {
  const info = build.value
  if (!info) return ''
  return [
    `Hive Desktop ${info.version}${info.channel ? ` (${info.channel})` : ''}`,
    `Commit ${info.commit} · built ${info.date}`,
    `${info.os}/${info.arch} · ${info.goVersion}`,
  ].join('\n')
})

interface Link {
  key: string
  label: string
  hint: string
  external: boolean
  open: () => void
}

const links: Link[] = [
  { key: 'docs', label: 'Documentation', hint: 'Guides and reference', external: true, open: openDocs },
  { key: 'updates', label: 'How updates work', hint: 'Channels and releases', external: true, open: openUpdatesDoc },
  { key: 'report', label: 'Report a problem', hint: 'Open a GitHub issue', external: true, open: () => reportProblem() },
]

// The changelog ships inside the binary, so this list is complete offline. It
// leads with the draft — what this build carries that no stable release does —
// followed by the stable releases themselves.
const { loadHistory } = useReleaseNotes()
const history = ref<ReleaseNote[]>([])
const expanded = ref<string[]>([])

function keyOf(entry: ReleaseNote): string {
  return entry.draft ? 'draft' : entry.version
}

function toggle(key: string): void {
  expanded.value = expanded.value.includes(key)
    ? expanded.value.filter((k) => k !== key)
    : [...expanded.value, key]
}

onMounted(async () => {
  void refresh()
  history.value = await loadHistory()
  // The newest set of notes opens by default; the rest are one click away.
  if (history.value.length > 0) expanded.value = [keyOf(history.value[0])]
})
</script>

<template>
  <SettingsPage testid="settings-about">
    <SettingsError v-if="error" :message="error" testid="about-error" />

    <section
      v-if="build"
      class="overflow-hidden rounded-[11px] border border-card bg-raised"
      data-testid="about-identity"
    >
      <div class="flex flex-col gap-3.5 p-4 @[560px]/pane:flex-row @[560px]/pane:items-center @[560px]/pane:gap-4">
        <span class="flex size-[34px] shrink-0 items-center justify-center rounded-[9px] border border-card bg-chip text-accent">
          <HiveMark class="size-[18px]" />
        </span>
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-x-2.5 gap-y-1">
            <h2 class="text-[13.5px] font-semibold text-text">Hive Desktop</h2>
            <span class="font-mono text-[11px] text-text-4" data-testid="about-build-version">{{ build.version }}</span>
            <BaseBadge
              :tone="status.tone"
              variant="pill"
              dot
              class="px-2.5 py-0.5 text-[11px] font-medium"
              data-testid="about-update-status"
            >{{ status.label }}</BaseBadge>
          </div>
          <p class="mt-1 text-[12.5px] leading-relaxed text-text-3" data-testid="about-update-checked">{{ checkedLabel }}</p>
        </div>
        <button
          type="button"
          class="flex shrink-0 cursor-pointer items-center gap-1.5 self-start rounded-lg border border-card px-3.5 py-2 text-[12.5px] font-medium text-text-2 hover:border-strong hover:text-text disabled:cursor-not-allowed disabled:opacity-50 @[560px]/pane:self-auto"
          :disabled="checking"
          data-testid="about-check-update"
          @click="checkForUpdates"
        >
          <IconRefreshCw class="size-3.5" :class="checking ? 'animate-spin' : ''" />
          {{ checking ? 'Checking…' : 'Check for updates' }}
        </button>
      </div>

      <div
        v-if="update?.available"
        class="border-t border-border bg-accent-tint/40 px-4 py-3.5"
        data-testid="about-update-available"
      >
        <div class="text-[13px] font-semibold text-text">{{ update.latestVersion }} is available</div>
        <div class="mt-0.5 text-[11.5px] text-text-3">Install it from the update badge in the title bar — Hive relaunches into the new version.</div>
        <p v-if="update.notes" class="mt-2 whitespace-pre-line text-[12px] text-text-2" data-testid="about-update-notes">{{ update.notes }}</p>
      </div>
    </section>

    <SettingsSection
      v-if="build"
      title="Build"
      description="The build of Hive you're running — include this when reporting an issue."
      testid="about-build"
    >
      <template #actions>
        <button
          type="button"
          class="flex cursor-pointer items-center gap-1.5 rounded-lg border border-card px-3 py-1.5 text-[12px] font-medium text-text-2 hover:border-strong hover:text-text"
          data-testid="about-copy-build"
          @click="copy(buildSummary)"
        >
          <component :is="copied ? IconCheck : IconCopy" class="size-3.5" />
          {{ copied ? 'Copied' : 'Copy build info' }}
        </button>
      </template>
      <!-- Hairlines rather than gaps: four facets of one build, not four cards. -->
      <div class="grid grid-cols-1 overflow-hidden rounded-[11px] border border-card bg-raised @[440px]/pane:grid-cols-2 @[800px]/pane:grid-cols-4">
        <div
          v-for="stat in stats"
          :key="stat.key"
          class="-ml-px -mt-px flex min-w-0 flex-col gap-1.5 border-l border-t border-border px-4 py-3.5"
          :data-testid="`about-stat-${stat.key}`"
        >
          <span class="flex items-center gap-1.5 font-mono text-[10px] font-semibold uppercase tracking-[.12em] text-text-3">
            <component :is="stat.icon" class="size-3" />{{ stat.label }}
          </span>
          <span class="truncate font-mono text-[16px] tabular-nums text-text" :data-testid="`about-build-${stat.key}`">{{ stat.value }}</span>
          <span class="truncate text-[11px] text-text-4">{{ stat.hint }}</span>
        </div>
      </div>
    </SettingsSection>

    <SettingsSection
      v-if="build"
      title="Updates"
      description="How Hive replaces itself with a newer build."
      boxed
    >
      <SettingsRow
        label="Automatic updates"
        hint="Check for and install new versions in the background."
      >
        <AppSwitch
          :model-value="autoUpdate"
          aria-label="Automatic updates"
          testid="about-auto-update"
          @update:model-value="setAutoUpdate"
        />
      </SettingsRow>
    </SettingsSection>

    <SettingsSection v-if="build" title="Elsewhere">
      <div class="grid grid-cols-1 gap-3 @[560px]/pane:grid-cols-3">
        <button
          v-for="link in links"
          :key="link.key"
          type="button"
          class="flex cursor-pointer flex-col gap-1 rounded-[11px] border border-card bg-raised px-4 py-3.5 text-left transition-colors hover:border-strong"
          :data-testid="`about-link-${link.key}`"
          @click="link.open()"
        >
          <span class="flex items-center justify-between gap-2">
            <span class="truncate text-[13px] font-semibold text-text">{{ link.label }}</span>
            <IconExternalLink v-if="link.external" class="size-3 shrink-0 text-text-4" />
            <IconChevronRight v-else class="size-3.5 shrink-0 text-text-4" />
          </span>
          <span class="truncate text-[12px] text-text-3">{{ link.hint }}</span>
        </button>
      </div>
    </SettingsSection>

    <SettingsSection
      v-if="history.length > 0"
      title="Release notes"
      description="What changed in each release, and what this build has that no release does yet."
      boxed
      testid="about-release-notes"
    >
      <div v-for="entry in history" :key="keyOf(entry)" class="px-4 py-3">
        <button
          type="button"
          class="flex w-full cursor-pointer items-baseline gap-2.5 text-left"
          :aria-expanded="expanded.includes(keyOf(entry))"
          :data-testid="`about-release-${keyOf(entry)}`"
          @click="toggle(keyOf(entry))"
        >
          <IconChevronRight
            class="size-3.5 shrink-0 self-center text-text-4 transition-transform"
            :class="expanded.includes(keyOf(entry)) ? 'rotate-90' : ''"
          />
          <span class="font-mono text-[13px] font-semibold text-text">
            {{ entry.draft ? 'Unreleased' : entry.version }}
          </span>
          <span v-if="entry.date" class="shrink-0 text-[11px] text-text-3">{{ entry.date }}</span>
          <span v-if="entry.summary" class="min-w-0 flex-1 truncate text-[12px] text-text-3">{{ entry.summary }}</span>
        </button>
        <div v-if="expanded.includes(keyOf(entry))" class="mt-2.5 pl-6">
          <p v-if="entry.summary" class="mb-2 text-[13px] leading-[1.6] text-text-2">{{ entry.summary }}</p>
          <ReleaseNoteBody :body="entry.body" />
        </div>
      </div>
    </SettingsSection>
  </SettingsPage>
</template>
