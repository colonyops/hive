<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import IconArrowDown from '~icons/lucide/arrow-down'
import IconCheck from '~icons/lucide/check'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronUp from '~icons/lucide/chevron-up'
import IconCircleAlert from '~icons/lucide/circle-alert'
import IconCopy from '~icons/lucide/copy'
import IconInfo from '~icons/lucide/info'
import IconListTodo from '~icons/lucide/list-todo'
import IconPlay from '~icons/lucide/play'
import IconRefreshCw from '~icons/lucide/refresh-cw'
import IconSearch from '~icons/lucide/search'
import IconTerminal from '~icons/lucide/terminal'
import IconX from '~icons/lucide/x'
import IconButton from '../ui/IconButton.vue'
import BaseButton from '../ui/BaseButton.vue'
import PaneStatusBar from '../PaneStatusBar.vue'
import SessionStatusChips from '../SessionStatusChips.vue'
import TerminalTab from '../TerminalTab.vue'
import { useClipboard } from '../../composables/useClipboard'
import { useEditorSettings } from '../../composables/useEditorSettings'
import { useSessionStatus } from '../../composables/useSessionStatus'
import { activePaneOf } from '../../lib/terminalLayout'
import { useTerminalStatusBar } from '../../stores/useTerminalStatusBar'
import {
  OpenSessionInEditor,
  RevealSession,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import { useTerminalModeContext } from './terminalModeContext'

const props = defineProps<{ active: boolean }>()
const emit = defineEmits<{ 'open-tasks': []; 'session-repo-key': [repoKey: string] }>()

const { pool, tree, attach, ops } = useTerminalModeContext()
const visible = computed(() => pool.visible)

const tabs = computed(() => visible.value?.tabs.value ?? [])
const activeWindowId = computed(() => visible.value?.activeWindowId.value ?? '')
const activeScrolledUp = computed(() =>
  tabs.value.some((tab) => tab.windowId === activeWindowId.value && activePaneOf(tab)?.scrolledUp),
)
const status = computed(() => visible.value?.status.value ?? 'connecting')
const endReason = computed(() => visible.value?.endReason.value ?? null)
const notStarted = computed(() => endReason.value === 'not-started' || endReason.value === 'start-failed')
const canRestart = computed(() => ['terminated', 'completed', 'stopped'].includes(endReason.value ?? ''))
const endedTitle = computed(() => {
  switch (endReason.value) {
    case 'terminated':
      return 'Tmux session terminated'
    case 'completed':
      return 'Session completed'
    case 'stopped':
      return 'Terminal stopped'
    case 'disconnected':
      return 'Terminal connection lost'
    case 'exited':
      return 'Terminal connection ended'
    default:
      return 'Terminal error'
  }
})
const scratchAttached = computed(() => !!tree.attachedRow && tree.isScratch(tree.attachedRow))
const chatAttached = computed(() => !!tree.attachedRow && tree.isChat(tree.attachedRow))
const sessionError = computed(() => visible.value?.error.value ?? '')
const actionError = computed(() => visible.value?.actionError.value || ops.treeError)
const sizeConstraint = computed(() => visible.value?.sizeConstraint.value ?? null)
const outputDropped = computed(() => visible.value?.outputDropped.value ?? false)
const { copy: copyStartError, status: startErrorCopyStatus } = useClipboard()
const startErrorCopyLabel = computed(() => {
  if (startErrorCopyStatus.value === 'success') return 'Copied'
  if (startErrorCopyStatus.value === 'error') return 'Copy failed'
  return 'Copy error'
})

const search = computed(() => visible.value?.search.value ?? { open: false, query: '', matches: 0, index: 0 })
const searchInput = ref<HTMLInputElement | null>(null)

// The find bar opens from inside the pane (the terminal owns its keys), so the
// focus move follows the state rather than the handler that set it.
watch(
  () => search.value.open,
  async (open) => {
    if (!open) return
    await nextTick()
    searchInput.value?.select()
  },
)

function searchLabel(): string {
  if (!search.value.query) return ''
  if (search.value.matches < 0) return 'many'
  if (!search.value.matches) return 'no results'
  return `${search.value.index}/${search.value.matches}`
}

// Only a hive session gets a status bar: the scratch terminal and the pinned
// chats are tmux sessions with no checkout behind them.
const { showStatusBar } = useTerminalStatusBar()
const { title: editorTitle, refresh: reloadEditor } = useEditorSettings()
watch(
  () => props.active,
  (active) => {
    if (active) void reloadEditor()
  },
  { immediate: true },
)

const statusBarRow = computed(() => {
  if (!showStatusBar.value || !pool.visibleSlug) return null
  return tree.activeSessions.find((row) => row.slug === pool.visibleSlug) ?? null
})
const statusBarSessionId = computed(() => statusBarRow.value?.id ?? '')
const {
  git: sessionGit,
  pullRequest: sessionPullRequest,
  pullRequestError: sessionPullRequestError,
  refresh: refreshSessionStatus,
} = useSessionStatus(statusBarSessionId)
const statusBarError = ref('')

async function runStatusBarAction(action: (id: string) => Promise<void>): Promise<void> {
  const id = statusBarSessionId.value
  if (!id) return
  statusBarError.value = ''
  try {
    await action(id)
  } catch (error) {
    statusBarError.value = error instanceof Error ? error.message : String(error)
  }
}

// owner/repo is exactly the hc repoKey format. Reported continuously, so App.vue
// can scope Tasks to the attached session's repo from any entry point and not
// only from this bar's own button.
const sessionRepoKey = computed(() => {
  const git = sessionGit.value
  return git?.resolved && git.owner && git.repo ? `${git.owner}/${git.repo}` : ''
})
watch(sessionRepoKey, (key) => emit('session-repo-key', key), { immediate: true })
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <div
      v-if="!visible"
      class="flex flex-1 flex-col items-center justify-center gap-2 px-10 text-center"
      data-testid="terminal-no-session"
    >
      <IconTerminal class="size-6 text-text-4" />
      <p class="text-xs text-text-3">Select a session to attach.</p>
    </div>

    <!-- Outside the started/not-started split: a stopped session still has a checkout to open. -->
    <PaneStatusBar
      v-if="statusBarRow"
      testid="terminal-pane-statusbar"
      :error="statusBarError"
      :editor-title="editorTitle"
      @open-editor="runStatusBarAction(OpenSessionInEditor)"
      @reveal="runStatusBarAction(RevealSession)"
    >
      <SessionStatusChips
        :git="sessionGit"
        :pull-request="sessionPullRequest"
        :pull-request-error="sessionPullRequestError"
        @refresh-pull-request="refreshSessionStatus({ refreshPullRequest: true })"
      />
      <template #actions>
        <IconButton
          label="Tasks"
          :icon="IconListTodo"
          data-testid="terminal-statusbar-tasks"
          @click="emit('open-tasks')"
        />
      </template>
    </PaneStatusBar>

    <p
      v-if="actionError"
      class="shrink-0 border-b border-border px-3 py-1.5 text-caption text-severity-error"
      data-testid="terminal-action-error"
    >
      {{ actionError }}
    </p>

    <template v-if="visible && !notStarted">
      <!-- Recovering the view without naming what it cost would hide the gap. -->
      <div
        v-if="outputDropped"
        class="flex shrink-0 items-start gap-2 border-b border-border bg-raised px-3 py-2"
        data-testid="terminal-output-dropped"
      >
        <IconInfo class="mt-px size-3.5 shrink-0 text-severity-warning" />
        <p class="min-w-0 flex-1 text-caption leading-relaxed text-text-3">
          Output arrived faster than this window could draw it, so some of it was dropped. These panes were repainted
          from tmux — what they show now is current, and their scrollback is tmux's, not what streamed here before the
          gap.
        </p>
        <IconButton
          label="Dismiss"
          :icon="IconX"
          size="sm"
          data-testid="terminal-output-dropped-dismiss"
          @click="visible?.dismissOutputDropped()"
        />
      </div>

      <!-- tmux's rule, not a fault: another attached client decides the grid size. -->
      <div
        v-if="sizeConstraint"
        class="flex shrink-0 items-start gap-2 border-b border-border bg-raised px-3 py-2"
        data-testid="terminal-size-constraint"
      >
        <IconInfo class="mt-px size-3.5 shrink-0 text-text-4" />
        <p class="min-w-0 flex-1 text-caption leading-relaxed text-text-3">
          tmux is drawing this window at
          <span class="font-mono text-text-2">{{ sizeConstraint.granted.cols }}×{{ sizeConstraint.granted.rows }}</span
          >, not the
          <span class="font-mono text-text-2">{{ sizeConstraint.voted.cols }}×{{ sizeConstraint.voted.rows }}</span>
          this pane fits. Every client attached to a session shares one grid per window, so another attached client is
          deciding the size. Detach it, or change tmux's
          <span class="font-mono text-text-2">window-size</span> option, to use the whole pane.
        </p>
        <IconButton
          label="Dismiss"
          :icon="IconX"
          size="sm"
          data-testid="terminal-size-constraint-dismiss"
          @click="visible?.dismissSizeConstraint()"
        />
      </div>
    </template>

    <!-- Hidden rather than unmounted without a session: a pooled session's terminals keep
         their elements, or each would lose its screen. -->
    <div class="relative min-h-0 min-w-0 flex-1 flex-col" :class="visible ? 'flex' : 'hidden'">
      <template v-for="pane in pool.paneSessions" :key="pane.slug">
        <TerminalTab
          v-for="tab in pane.tabs"
          :key="tab.uid"
          :tab="tab"
          :session="pane.entry"
          :active="pane.entry === visible && tab.windowId === pane.activeWindowId"
        />
      </template>
      <template v-if="visible">
        <div
          v-if="!tabs.length && status !== 'ended'"
          class="flex flex-1 items-center justify-center font-mono text-xs text-text-4"
        >
          Attaching…
        </div>

        <!-- Floated rather than in the column: shrinking the pane's box would change the
             window size this client votes to tmux, for every attached client. -->
        <div
          v-if="search.open && status !== 'ended'"
          class="absolute right-5 top-3 z-10 flex items-center gap-1 rounded-md border border-strong bg-raised/95 py-1 pl-2 pr-1 shadow-lg"
          data-testid="terminal-search"
        >
          <IconSearch class="size-3 shrink-0 text-text-4" />
          <input
            ref="searchInput"
            :value="search.query"
            type="text"
            placeholder="Find"
            spellcheck="false"
            class="w-44 bg-transparent text-caption text-text outline-none placeholder:text-text-4"
            data-testid="terminal-search-input"
            @input="visible?.setSearchQuery(($event.target as HTMLInputElement).value)"
            @keydown.enter.exact.prevent="visible?.findNext()"
            @keydown.enter.shift.prevent="visible?.findPrevious()"
            @keydown.esc.prevent="visible?.closeSearch()"
          />
          <span
            class="min-w-[54px] shrink-0 text-right font-mono text-micro text-text-4"
            data-testid="terminal-search-count"
            >{{ searchLabel() }}</span
          >
          <IconButton
            label="Previous match"
            :icon="IconChevronUp"
            size="sm"
            data-testid="terminal-search-prev"
            @click="visible?.findPrevious()"
          />
          <IconButton
            label="Next match"
            :icon="IconChevronDown"
            size="sm"
            data-testid="terminal-search-next"
            @click="visible?.findNext()"
          />
          <IconButton
            label="Close find"
            :icon="IconX"
            size="sm"
            data-testid="terminal-search-close"
            @click="visible?.closeSearch()"
          />
        </div>

        <Transition name="tail-pill">
          <button
            v-if="activeScrolledUp && status !== 'ended'"
            type="button"
            class="absolute bottom-3 right-5 z-10 flex cursor-pointer items-center gap-1.5 rounded-full border border-strong bg-raised/95 px-3 py-1.5 text-caption text-text-2 shadow-lg hover:text-text"
            data-testid="terminal-scroll-to-bottom"
            @click="visible?.scrollToBottom()"
          >
            <IconArrowDown class="size-3" />Scroll to bottom
          </button>
        </Transition>

        <!-- Starting runs the session's agent command, so it is offered, never done on selection. -->
        <div
          v-if="notStarted"
          class="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-app/95 px-10 text-center"
          data-testid="terminal-session-not-started"
        >
          <template v-if="attach.startError">
            <div
              class="flex size-11 items-center justify-center rounded-xl border border-severity-error-border bg-severity-error-tint"
            >
              <IconCircleAlert class="size-5 text-severity-error" />
            </div>
            <div class="text-title font-semibold" data-testid="terminal-start-error-title">
              {{
                chatAttached
                  ? 'Chat failed to resume'
                  : scratchAttached
                    ? 'Terminal failed to start'
                    : 'Session failed to start'
              }}
            </div>
            <p class="max-w-[560px] text-xs leading-relaxed text-text-3">
              Hive could not start the configured terminal for
              <span class="font-mono text-text-2">{{ pool.activeSlug }}</span
              >.
            </p>
            <div
              class="w-full max-w-[680px] overflow-hidden rounded-xl border border-severity-error-border bg-sunken text-left shadow-lg"
              data-testid="terminal-start-error"
            >
              <div class="flex h-9 items-center border-b border-severity-error-border bg-severity-error-tint px-3">
                <div class="flex items-center gap-1.5" aria-hidden="true">
                  <span class="size-2 rounded-full bg-severity-error" />
                  <span class="size-2 rounded-full bg-severity-warning" />
                  <span class="size-2 rounded-full bg-severity-success" />
                </div>
                <div class="flex min-w-0 flex-1 items-center justify-center gap-1.5 font-mono text-micro text-text-3">
                  <IconTerminal class="size-3" />
                  <span>Startup error</span>
                </div>
                <IconButton
                  :label="startErrorCopyLabel"
                  :icon="startErrorCopyStatus === 'success' ? IconCheck : IconCopy"
                  size="sm"
                  tone="danger"
                  data-testid="terminal-start-error-copy"
                  @click="copyStartError(attach.startError)"
                />
              </div>
              <pre
                class="hive-scroll max-h-[280px] min-h-24 select-text overflow-auto whitespace-pre-wrap break-words p-4 font-mono text-small leading-6 text-text-2"
                data-testid="terminal-start-error-output"
                >{{ attach.startError }}</pre>
            </div>
          </template>
          <template v-else>
            <IconTerminal class="size-6 text-text-4" />
            <div class="text-body font-semibold">
              {{ chatAttached ? 'Chat not running' : scratchAttached ? 'Terminal not started' : 'Session not started' }}
            </div>
            <p v-if="chatAttached" class="max-w-[420px] text-xs leading-relaxed text-text-3">
              This chat is stopped. Resuming it launches the agent again in its workspace, picking the conversation back
              up where the agent itself can.
            </p>
            <p v-else-if="scratchAttached" class="max-w-[420px] text-xs leading-relaxed text-text-3">
              The scratch terminal is not running. Starting it opens a shell in your home directory, and every tab you
              add opens there too.
            </p>
            <p v-else class="max-w-[420px] text-xs leading-relaxed text-text-3">
              No terminal is running for <span class="font-mono text-text-2">{{ pool.activeSlug }}</span> yet. Starting
              it opens this session's configured windows and runs its agent command.
            </p>
          </template>
          <div class="mt-1 flex items-center gap-2">
            <BaseButton
              size="sm"
              :busy="attach.starting === pool.activeSlug"
              data-testid="terminal-start-session"
              @click="attach.startSession(pool.activeSlug)"
            >
              <template #icon>
                <IconRefreshCw v-if="attach.startError" class="size-3.5" />
                <IconPlay v-else class="size-3.5" />
              </template>
              {{
                attach.starting === pool.activeSlug
                  ? chatAttached
                    ? 'Resuming…'
                    : 'Starting…'
                  : attach.startError
                    ? 'Try again'
                    : chatAttached
                      ? 'Resume chat'
                      : scratchAttached
                        ? 'Start terminal'
                        : 'Start session'
              }}
            </BaseButton>
            <BaseButton variant="secondary" size="sm" data-testid="terminal-close-session" @click="attach.closeSession"
              >Close</BaseButton
            >
          </div>
        </div>

        <div
          v-else-if="status === 'ended'"
          class="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-app/95 px-10 text-center"
          data-testid="terminal-session-ended"
        >
          <div class="text-body font-semibold" data-testid="terminal-session-ended-title">{{ endedTitle }}</div>
          <div class="w-full max-w-[680px] overflow-hidden rounded-xl border border-strong bg-sunken text-left">
            <div class="flex items-center justify-between border-b border-strong px-3 py-2">
              <span class="font-mono text-micro text-text-3">Terminal report</span>
              <IconButton
                :label="startErrorCopyLabel"
                :icon="startErrorCopyStatus === 'success' ? IconCheck : IconCopy"
                size="sm"
                data-testid="terminal-ended-copy"
                @click="copyStartError(sessionError)"
              />
            </div>
            <pre
              class="hive-scroll max-h-[280px] select-text overflow-auto whitespace-pre-wrap break-words p-4 font-mono text-small text-text-2"
              data-testid="terminal-session-ended-reason"
              >{{ sessionError }}</pre>
          </div>
          <div class="mt-1 flex items-center gap-2">
            <BaseButton
              v-if="canRestart"
              size="sm"
              :disabled="attach.starting === pool.activeSlug"
              data-testid="terminal-restart"
              @click="attach.startSession(pool.activeSlug)"
            >
              <template #icon><IconRefreshCw class="size-3" /></template>
              {{ attach.starting === pool.activeSlug ? 'Starting…' : 'Restart' }}
            </BaseButton>
            <BaseButton v-else size="sm" data-testid="terminal-reconnect" @click="visible?.reconnect()">
              <template #icon><IconRefreshCw class="size-3" /></template>Reconnect
            </BaseButton>
            <BaseButton variant="secondary" size="sm" data-testid="terminal-close-session" @click="attach.closeSession">
              Close session
            </BaseButton>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
