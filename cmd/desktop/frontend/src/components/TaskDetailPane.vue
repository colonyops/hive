<script setup lang="ts">
// Detail pane for the selected task. Self-contained via useTasks() (the same
// module singleton TasksView reads) rather than props/emit — it owns
// everything about the selection: identity, the status control and its
// confirms (cancel; epic → done/cancelled cascade), rendered description and
// comments, blockers, and delete. TasksView owns the list/tree only.
import InlineError from './ui/InlineError.vue'
import { computed, ref, watch } from 'vue'
import { Browser } from '@wailsio/runtime'
import IconBan from '~icons/lucide/ban'
import IconBookmarkCheck from '~icons/lucide/bookmark-check'
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconLink2 from '~icons/lucide/link-2'
import IconTrash2 from '~icons/lucide/trash-2'
import AppSelect, { type AppSelectOption } from './ui/AppSelect.vue'
import BaseBadge from './ui/BaseBadge.vue'
import ConfirmationHost from './ui/ConfirmationHost.vue'
import PanelResizeHandle from './ui/PanelResizeHandle.vue'
import { useClipboard } from '../composables/useClipboard'
import { useConfirmation } from '../composables/useConfirmation'
import { useResizablePanel } from '../composables/useResizablePanel'
import { useTasks } from '../stores/useTasks'
import { useTerminalSessions } from '../stores/useTerminalSessions'
import { errorText } from '../lib/appError'
import { relativeAgo } from '../lib/age'
import { renderGithubMarkdown } from '../lib/githubMarkdown'
import { externalMarkdownHref } from '../lib/markdownLinks'
import { cascadeCount, checkpointBody, isCheckpoint, matchesTaskFilter, statusMeta } from '../lib/tasksPresentation'
import type { TaskComment } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import BaseButton from './ui/BaseButton.vue'
import EmptyState from './ui/EmptyState.vue'

const { detail, items, selectedId, setStatus, remove, select } = useTasks()

const STATUS_OPTIONS: AppSelectOption[] = [
  { value: 'open', label: 'Open' },
  { value: 'in_progress', label: 'In Progress' },
  { value: 'done', label: 'Done' },
  { value: 'cancelled', label: 'Cancelled' },
]

const typeLabel = computed(() => (detail.value?.type === 'epic' ? 'Epic' : 'Task'))
const bodyHtml = computed(() => (detail.value?.desc ? renderGithubMarkdown(detail.value.desc) : ''))

function absoluteTime(iso: string): string {
  return new Date(iso).toLocaleString()
}

function commentHtml(comment: TaskComment): string {
  return renderGithubMarkdown(checkpointBody(comment))
}

// Rendered bodies are untrusted markdown; links must open in the user's real
// browser rather than navigate the webview away from the app.
function onBodyClick(event: MouseEvent): void {
  const href = externalMarkdownHref(event)
  if (href) void Browser.OpenURL(href)
}

// The name of the linked session, from the same singleton the terminal
// sidebar renders; an ended session has no loaded row, so the raw id stands
// in rather than hiding the linkage.
const { sessions } = useTerminalSessions()
const sessionLabel = computed(() => {
  const id = detail.value?.sessionId
  if (!id) return ''
  return sessions.value.find((row) => row.id === id)?.name ?? id
})

// hc marks an item blocked for open/in_progress direct children OR explicit
// open blockers (hc.Store's fetchHCItem); the explicit ones arrive in
// detail.blockers, so whatever they don't account for is open subtasks.
const blockedReason = computed(() => {
  const current = detail.value
  if (!current?.blocked) return ''
  const parts: string[] = []
  const explicit = (current.blockers ?? []).length
  if (explicit > 0) parts.push(`${explicit} blocking task${explicit === 1 ? '' : 's'}`)
  const openChildren = items.value.filter(
    (item) => item.parentId === current.id && matchesTaskFilter(item, 'open'),
  ).length
  if (openChildren > 0) parts.push(`${openChildren} open subtask${openChildren === 1 ? '' : 's'}`)
  return parts.length ? `Blocked by ${parts.join(' and ')}` : 'Blocked'
})

// ── Copy ID ──────────────────────────────────────────────────────────────
const { copy: copyText, copied: idCopied } = useClipboard()
function copyId(): void {
  if (detail.value) void copyText(detail.value.id)
}

// ── Status control ──────────────────────────────────────────────────────
// Cancelling always confirms. An epic moving to done/cancelled with open
// descendants confirms with the cascade count instead — that copy already
// implies the cancellation, so it supersedes the plain cancel confirm.
const confirmation = useConfirmation()
const statusError = ref<string | null>(null)

function requestStatusChange(next: string): void {
  const current = detail.value
  if (!current) return
  statusError.value = null
  const onConfirm = () => setStatus(current.id, next)
  const cascade =
    current.type === 'epic' && (next === 'done' || next === 'cancelled') ? cascadeCount(items.value, current.id) : 0
  if (cascade > 0) {
    const label = statusMeta(next).label.toLowerCase()
    confirmation.request({
      title: 'Close nested tasks?',
      description: `Marking this epic ${label} also closes ${cascade} open task${cascade === 1 ? '' : 's'} nested under it.`,
      testid: 'task-cascade-confirm',
      onConfirm,
    })
  } else if (next === 'cancelled') {
    confirmation.request({
      title: 'Cancel this task?',
      description: 'This marks the task cancelled. It stays in the tree but drops out of the open filters.',
      confirmLabel: 'Cancel task',
      testid: 'task-cancel-confirm',
      onConfirm,
    })
  } else {
    setStatus(current.id, next).catch((err: unknown) => {
      statusError.value = errorText(err, 'Could not update status.')
    })
  }
}

// ── Delete ───────────────────────────────────────────────────────────────
function requestDelete(): void {
  const id = detail.value?.id
  if (!id) return
  confirmation.request({
    title: 'Delete task',
    description: 'Deletes this item and everything nested under it, comments included. This cannot be undone.',
    confirmLabel: 'Delete',
    testid: 'task-delete-confirm',
    onConfirm: () => remove(id),
  })
}

// A new selection must not inherit the previous one's failure or a half-open
// confirm: a poll can clear a vanished selection while a dialog is up, and a
// lingering error would blame the wrong task.
watch(selectedId, () => {
  statusError.value = null
  confirmation.cancel()
})

// DetailPane.vue's precedent: docked right, handle on the left edge, width
// persisted separately from the tree pane it sits beside.
const {
  size: paneWidth,
  startResize,
  step,
} = useResizablePanel({
  storageKey: 'hive.panel.taskdetail',
  defaultSize: 560,
  min: 320,
  max: 1000,
  edge: 'left',
})
</script>

<template>
  <aside
    class="hive-scroll relative flex shrink-0 flex-col overflow-y-auto bg-pane"
    :style="{ width: paneWidth + 'px' }"
    data-testid="task-detail-pane"
  >
    <PanelResizeHandle edge="left" name="taskdetail" :start="startResize" :step="step" />

    <template v-if="detail">
      <div class="border-b border-border px-5 pb-4 pt-[18px]">
        <div class="flex items-start justify-between gap-3">
          <h1
            class="min-w-0 flex-1 text-heading font-semibold leading-[1.3] tracking-[-.01em]"
            data-testid="task-detail-title"
          >
            {{ detail.title }}
          </h1>
          <button
            type="button"
            class="flex shrink-0 items-center gap-1.5 rounded border border-card px-2 py-1 font-mono text-micro text-text-3 hover:border-strong hover:text-text"
            data-testid="task-detail-copy-id"
            @click="copyId"
          >
            <span class="max-w-[110px] truncate">{{ idCopied ? 'Copied' : detail.id }}</span>
            <component :is="idCopied ? IconCheck : IconCopy" class="size-3 shrink-0" />
          </button>
        </div>

        <div
          class="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-caption text-text-3"
          data-testid="task-detail-meta"
        >
          <span>{{ detail.repoKey }}</span>
          <span>·</span>
          <span>{{ typeLabel }}</span>
          <template v-if="detail.sessionId">
            <span>·</span>
            <span data-testid="task-detail-session">Session {{ sessionLabel }}</span>
          </template>
          <span>·</span>
          <span :title="absoluteTime(detail.createdAt)">Created {{ relativeAgo(Date.parse(detail.createdAt)) }}</span>
          <span>·</span>
          <span :title="absoluteTime(detail.updatedAt)">Updated {{ relativeAgo(Date.parse(detail.updatedAt)) }}</span>
        </div>

        <div
          v-if="detail.blocked"
          class="mt-2 flex items-center gap-1.5 text-caption font-medium text-severity-error"
          data-testid="task-detail-blocked"
        >
          <IconBan class="size-3 shrink-0" aria-hidden="true" />{{ blockedReason }}
        </div>

        <div class="mt-3.5 max-w-[220px]">
          <AppSelect
            :model-value="detail.status"
            :options="STATUS_OPTIONS"
            size="sm"
            aria-label="Status"
            testid="task-status-select"
            @update:model-value="requestStatusChange"
          />
          <InlineError
            v-if="statusError"
            testid="task-status-error"
            variant="line"
            class="mt-1.5"
            :message="statusError"
          />
        </div>

        <!-- eslint-disable vue/no-v-html -- renderGithubMarkdown escapes raw HTML (githubMarkdown.spec.ts) -->
        <div
          v-if="bodyHtml"
          class="markdown-body mt-3.5 text-body leading-[1.65] text-text-2"
          data-testid="task-detail-body"
          @click="onBodyClick"
          v-html="bodyHtml"
        />
        <!-- eslint-enable vue/no-v-html -->
      </div>

      <div class="px-5 pb-5 pt-4">
        <section v-if="(detail.blockers ?? []).length" data-testid="task-blockers">
          <h2 class="mb-2 font-mono text-micro tracking-[.12em] text-text-3">BLOCKERS</h2>
          <div class="flex flex-wrap gap-1.5">
            <!-- A chip with a title is a live task the user will want to
                 inspect, so it selects it in place; one without (the blocker
                 item was deleted, only the edge's id remains) has nothing to
                 open and stays inert. -->
            <template v-for="blocker in detail.blockers ?? []" :key="blocker.id">
              <button
                v-if="blocker.title"
                type="button"
                class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-card px-2 py-1 text-caption text-text-2 hover:border-strong hover:text-text"
                data-testid="task-blocker-chip"
                @click="select(blocker.id)"
              >
                <IconLink2 class="size-3 shrink-0 text-text-4" />{{ blocker.title }}
                <span
                  class="rounded-sm px-1 py-px text-micro font-medium"
                  :class="statusMeta(blocker.status).classes"
                  data-testid="task-blocker-status"
                  >{{ statusMeta(blocker.status).label }}</span
                >
              </button>
              <span
                v-else
                class="inline-flex items-center gap-1.5 rounded-md border border-card px-2 py-1 text-caption italic text-text-4"
                data-testid="task-blocker-chip"
                ><IconLink2 class="size-3 shrink-0 text-text-4" />{{ blocker.id }}</span
              >
            </template>
          </div>
        </section>

        <section
          v-if="(detail.comments ?? []).length"
          class="mt-5 border-t border-border pt-4"
          data-testid="task-comments"
        >
          <h2 class="mb-3 font-mono text-micro tracking-[.12em] text-text-3">COMMENTS</h2>
          <div class="flex flex-col gap-3.5">
            <div v-for="comment in detail.comments ?? []" :key="comment.id" data-testid="task-comment">
              <div class="mb-1 flex items-center gap-2">
                <BaseBadge
                  v-if="isCheckpoint(comment)"
                  tone="accent"
                  class="px-2 py-0.5 text-micro font-semibold"
                  data-testid="task-comment-checkpoint"
                >
                  <IconBookmarkCheck class="size-3" />CHECKPOINT
                </BaseBadge>
                <span class="font-mono text-micro text-text-4" :title="absoluteTime(comment.createdAt)">{{
                  relativeAgo(Date.parse(comment.createdAt))
                }}</span>
              </div>
              <!-- eslint-disable vue/no-v-html -- renderGithubMarkdown escapes raw HTML (githubMarkdown.spec.ts) -->
              <div
                class="markdown-body text-body leading-[1.6] text-text-2"
                @click="onBodyClick"
                v-html="commentHtml(comment)"
              />
              <!-- eslint-enable vue/no-v-html -->
            </div>
          </div>
        </section>

        <div class="mt-6 border-t border-border pt-4">
          <BaseButton variant="danger-outline" size="xs" data-testid="task-delete" @click="requestDelete">
            <template #icon><IconTrash2 class="size-3.5" /></template>Delete
          </BaseButton>
        </div>
      </div>
    </template>
    <EmptyState v-else class="m-auto font-mono" message="Select a task to inspect" data-testid="task-detail-empty" />

    <ConfirmationHost :confirmation="confirmation" />
  </aside>
</template>
