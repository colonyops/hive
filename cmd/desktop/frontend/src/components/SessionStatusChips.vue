<script setup lang="ts">
// The session half of the status bar. Session-only — a chat has no branch —
// which is why it sits in PaneStatusBar's slot rather than in the bar itself.
import { computed } from 'vue'
import { Browser } from '@wailsio/runtime'
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconFileDiff from '~icons/lucide/file-diff'
import IconGitBranch from '~icons/lucide/git-branch'
import IconGitPullRequest from '~icons/lucide/git-pull-request'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import IconUpload from '~icons/lucide/upload'
import AppTooltip from './ui/AppTooltip.vue'
import { useClipboard } from '../composables/useClipboard'
import { markdownPullRequestLink } from '../lib/prLink'
import type {
  SessionGitStatus,
  SessionPullRequest,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'

const props = defineProps<{
  git: SessionGitStatus | null
  pullRequest: SessionPullRequest | null
  /** Why the pull-request lookup failed; shown instead of a PR chip. */
  pullRequestError: string
}>()

const emit = defineEmits<{ 'refresh-pull-request': [] }>()

const showBranch = computed(() => !!props.git?.resolved && !!props.git.branch)
const showDiff = computed(() => !!props.git?.resolved && !!(props.git.additions || props.git.deletions))

// Tracked so the rule between the two halves appears only when it has
// something on both sides to separate.
const showGitGroup = computed(() => {
  const git = props.git
  if (!git) return false
  return showBranch.value || showDiff.value || !!git.error || (git.resolved && (git.dirty || git.unpushed))
})
const showPRGroup = computed(() => props.pullRequest?.status === 'found' || !!props.pullRequestError)

// A cached pull request was known before the bar painted, so there is nothing
// to announce.
const animateArrival = computed(() => !props.pullRequest?.cached)

// The other statuses say why there is no pull request, and none of them is
// worth a chip: a branch without one is the normal state of an unpushed session.
const pr = computed(() => (props.pullRequest?.status === 'found' ? props.pullRequest : null))

const prLabel = computed(() => {
  const found = pr.value
  if (!found) return ''
  if (found.state === 'MERGED') return `#${found.number} merged`
  if (found.state === 'CLOSED') return `#${found.number} closed`
  return found.isDraft ? `#${found.number} draft` : `#${found.number}`
})

const prTone = computed(() => {
  const found = pr.value
  if (!found) return 'text-text-3'
  if (found.state === 'MERGED') return 'text-accent'
  if (found.state === 'CLOSED') return 'text-text-4'
  if (found.reviewDecision === 'CHANGES_REQUESTED') return 'text-severity-warning'
  if (found.reviewDecision === 'APPROVED') return 'text-severity-success'
  return 'text-text-3'
})

const checksTone = computed(() => {
  switch (pr.value?.checks) {
    case 'passing':
      return 'text-severity-success'
    case 'failing':
      return 'text-severity-error'
    default:
      return 'text-severity-warning'
  }
})

function openPullRequest(): void {
  const url = pr.value?.url
  if (url) void Browser.OpenURL(url)
}

const { copy, copied } = useClipboard()

async function copyLink(): Promise<void> {
  const found = pr.value
  // The repository name alone, as the shell script used: the owner is implied
  // by wherever this is being pasted.
  const repo = props.git?.repo
  if (!found || !repo) return
  await copy(markdownPullRequestLink(found, repo))
}
</script>

<template>
  <!-- The branch is the only item allowed to shrink, so a narrow pane truncates
       the branch name and everything else stays whole. It carries no tooltip,
       so a name cut this way cannot be read back. -->
  <div v-if="git" class="flex min-w-0 items-center text-[11px]" data-testid="session-status-chips">
    <div v-if="showGitGroup" class="flex min-w-0 items-center gap-1">
      <span
        v-if="showBranch"
        class="flex h-6 min-w-0 items-center gap-1 px-1.5 text-text-3"
        data-testid="session-status-branch"
      >
        <IconGitBranch class="size-3 shrink-0 text-text-4" aria-hidden="true" />
        <span class="truncate font-mono">{{ git.branch }}</span>
      </span>

      <AppTooltip v-if="showDiff" text="Lines changed against the default branch">
        <span class="flex h-6 shrink-0 items-center gap-1 px-1.5 font-mono" data-testid="session-status-diff">
          <span class="text-severity-success">+{{ git.additions }}</span>
          <span class="text-severity-error">−{{ git.deletions }}</span>
        </span>
      </AppTooltip>

      <!-- Icon-only, so the tooltip is the whole explanation — AppTooltip
           rather than `title`, which WebKit takes ~1.5s to show. -->
      <AppTooltip v-if="git.resolved && git.dirty" text="Uncommitted changes in this checkout">
        <span
          class="flex size-6 shrink-0 items-center justify-center text-severity-warning"
          role="img"
          aria-label="Uncommitted changes in this checkout"
          data-testid="session-status-dirty"
          ><IconFileDiff class="size-3.5"
        /></span>
      </AppTooltip>

      <AppTooltip v-if="git.resolved && git.unpushed" text="Commits on this branch that the remote does not have">
        <span
          class="flex size-6 shrink-0 items-center justify-center text-text-3"
          role="img"
          aria-label="Commits on this branch that the remote does not have"
          data-testid="session-status-unpushed"
          ><IconUpload class="size-3.5"
        /></span>
      </AppTooltip>

      <!-- Saying the read failed beats silently reporting a clean branch the
           bar never managed to look at. -->
      <AppTooltip v-if="git.error" :text="git.error">
        <span
          class="flex h-6 shrink-0 items-center gap-1 px-1.5 text-severity-error"
          data-testid="session-status-git-error"
          ><IconTriangleAlert class="size-3" aria-hidden="true" />git failed</span
        >
      </AppTooltip>
    </div>

    <!-- Enter only: a leave transition would make switching sessions flicker. -->
    <Transition :css="animateArrival" name="pr-arrive">
      <div v-if="showPRGroup" class="flex shrink-0 items-center">
        <!-- Outside the gap-1 group below on purpose: as a child of it the
             group's gap would land on the rule's right only. -->
        <span v-if="showGitGroup" class="mx-2 h-3.5 w-px shrink-0 bg-border" aria-hidden="true" />

        <div class="flex shrink-0 items-center gap-1">
          <!-- h-6/rounded-[7px] is PaneStatusBar's button metric; these share a
             row with its editor and Finder buttons. -->
          <button
            v-if="pr"
            type="button"
            class="flex h-6 shrink-0 cursor-pointer items-center gap-1 rounded-[7px] px-1.5 hover:bg-chip"
            :class="prTone"
            data-testid="session-status-pr"
            @click="openPullRequest"
          >
            <IconGitPullRequest class="size-3" aria-hidden="true" />
            <span class="font-mono">{{ prLabel }}</span>
            <span v-if="pr.checks" :class="checksTone" data-testid="session-status-checks">{{ pr.checks }}</span>
          </button>

          <AppTooltip v-if="pr" :text="copied ? 'Copied' : 'Copy link to this pull request'">
            <button
              type="button"
              class="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-[7px] text-text-4 hover:bg-chip hover:text-text"
              :class="{ 'text-severity-success': copied }"
              aria-label="Copy link to this pull request"
              data-testid="session-status-copy"
              @click="copyLink"
            >
              <IconCheck v-if="copied" class="size-3.5" />
              <IconCopy v-else class="size-3.5" />
            </button>
          </AppTooltip>

          <!-- A failed lookup, never rendered as "no pull request": the branch may
           well have one, and claiming otherwise is a fact this cannot support. -->
          <AppTooltip v-else-if="pullRequestError" :text="`${pullRequestError} — click to retry`">
            <button
              type="button"
              class="flex h-6 shrink-0 cursor-pointer items-center gap-1 rounded-[7px] px-1.5 text-severity-error hover:bg-chip"
              data-testid="session-status-pr-error"
              @click="emit('refresh-pull-request')"
            >
              <IconTriangleAlert class="size-3" aria-hidden="true" />PR failed
            </button>
          </AppTooltip>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
/* Short and small: the row is chrome, and anything longer pulls the eye off
   whatever the terminal below is doing. */
.pr-arrive-enter-active {
  transition:
    opacity 180ms ease-out,
    transform 180ms ease-out;
}

.pr-arrive-enter-from {
  opacity: 0;
  transform: translateX(-4px);
}

@media (prefers-reduced-motion: reduce) {
  .pr-arrive-enter-active {
    transition: none;
  }
}
</style>
