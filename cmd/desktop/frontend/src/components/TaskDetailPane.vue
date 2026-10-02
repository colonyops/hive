<script setup lang="ts">
// Detail pane for the selected task. Self-contained via useTasks() (the same
// module singleton TasksView reads) rather than props/emit — it owns
// everything about the selection: identity, the status control and its
// confirms (cancel; epic → done/cancelled cascade), rendered description and
// comments, blockers, and delete. TasksView owns the list/tree only.
import { computed, ref, watch } from 'vue'
import { Browser } from '@wailsio/runtime'
import IconBan from '~icons/lucide/ban'
import IconBookmarkCheck from '~icons/lucide/bookmark-check'
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconLink2 from '~icons/lucide/link-2'
import IconTrash2 from '~icons/lucide/trash-2'
import AppSelect, { type AppSelectOption } from './AppSelect.vue'
import BaseBadge from './BaseBadge.vue'
import ConfirmationDialog from './ConfirmationDialog.vue'
import PanelResizeHandle from './PanelResizeHandle.vue'
import { useClipboard } from '../composables/useClipboard'
import { useResizablePanel } from '../composables/useResizablePanel'
import { useTasks } from '../composables/useTasks'
import { useTerminalSessions } from '../stores/useTerminalSessions'
import { errorText } from '../lib/appError'
import { relativeAge } from '../lib/age'
import { renderGithubMarkdown } from '../lib/githubMarkdown'
import { externalMarkdownHref } from '../lib/markdownLinks'
import { cascadeCount, checkpointBody, isCheckpoint, matchesTaskFilter, statusMeta } from '../lib/tasksPresentation'
import type { TaskComment } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'

const { detail, items, selectedId, setStatus, remove, select } = useTasks()

const STATUS_OPTIONS: AppSelectOption[] = [
  { value: 'open', label: 'Open' },
  { value: 'in_progress', label: 'In Progress' },
  { value: 'done', label: 'Done' },
  { value: 'cancelled', label: 'Cancelled' },
]

const typeLabel = computed(() => (detail.value?.type === 'epic' ? 'Epic' : 'Task'))
const bodyHtml = computed(() => (detail.value?.desc ? renderGithubMarkdown(detail.value.desc) : ''))

function agoLabel(timestamp: number): string {
  const label = relativeAge(timestamp)
  return label === 'now' ? 'now' : `${label} ago`
}

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
const pendingStatus = ref<string | null>(null)
const cascadeConfirmCount = ref(0)
const cascadeConfirmOpen = ref(false)
const cancelConfirmOpen = ref(false)
const statusBusy = ref(false)
const statusError = ref<string | null>(null)

function requestStatusChange(next: string): void {
  const current = detail.value
  if (!current) return
  const cascade =
    current.type === 'epic' && (next === 'done' || next === 'cancelled') ? cascadeCount(items.value, current.id) : 0
  if (cascade > 0) {
    pendingStatus.value = next
    cascadeConfirmCount.value = cascade
    cascadeConfirmOpen.value = true
    return
  }
  if (next === 'cancelled') {
    pendingStatus.value = next
    cancelConfirmOpen.value = true
    return
  }
  void applyStatus(next)
}

async function applyStatus(status: string): Promise<void> {
  const id = detail.value?.id
  if (!id) return
  statusBusy.value = true
  statusError.value = null
  try {
    await setStatus(id, status)
    cascadeConfirmOpen.value = false
    cancelConfirmOpen.value = false
    pendingStatus.value = null
  } catch (err) {
    statusError.value = errorText(err, 'Could not update status.')
  } finally {
    statusBusy.value = false
  }
}

function confirmPendingStatus(): void {
  if (pendingStatus.value) void applyStatus(pendingStatus.value)
}

function cancelPendingStatus(): void {
  cascadeConfirmOpen.value = false
  cancelConfirmOpen.value = false
  pendingStatus.value = null
  statusError.value = null
}

const cascadeDescription = computed(() => {
  const label = statusMeta(pendingStatus.value ?? '').label.toLowerCase()
  const count = cascadeConfirmCount.value
  return `Marking this epic ${label} also closes ${count} open task${count === 1 ? '' : 's'} nested under it.`
})

// ── Delete ───────────────────────────────────────────────────────────────
const deleteConfirmOpen = ref(false)
const deleteBusy = ref(false)
const deleteError = ref<string | null>(null)

function openDeleteConfirm(): void {
  deleteError.value = null
  deleteConfirmOpen.value = true
}

// A new selection must not inherit the previous one's failures or half-open
// confirms — a poll can clear a vanished selection while a dialog is up, and
// a lingering error would blame the wrong task.
watch(selectedId, () => {
  statusError.value = null
  deleteError.value = null
  pendingStatus.value = null
  cascadeConfirmOpen.value = false
  cancelConfirmOpen.value = false
  deleteConfirmOpen.value = false
})

async function confirmDelete(): Promise<void> {
  const id = detail.value?.id
  if (!id) return
  deleteBusy.value = true
  deleteError.value = null
  try {
    await remove(id)
    deleteConfirmOpen.value = false
  } catch (err) {
    deleteError.value = errorText(err, 'Could not delete task.')
  } finally {
    deleteBusy.value = false
  }
}

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
            class="min-w-0 flex-1 text-[16.5px] font-semibold leading-[1.3] tracking-[-.01em]"
            data-testid="task-detail-title"
          >
            {{ detail.title }}
          </h1>
          <button
            type="button"
            class="flex shrink-0 items-center gap-1.5 rounded border border-card px-2 py-1 font-mono text-[10.5px] text-text-3 hover:border-strong hover:text-text"
            data-testid="task-detail-copy-id"
            @click="copyId"
          >
            <span class="max-w-[110px] truncate">{{ idCopied ? 'Copied' : detail.id }}</span>
            <component :is="idCopied ? IconCheck : IconCopy" class="size-3 shrink-0" />
          </button>
        </div>

        <div
          class="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11.5px] text-text-3"
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
          <span :title="absoluteTime(detail.createdAt)">Created {{ agoLabel(Date.parse(detail.createdAt)) }}</span>
          <span>·</span>
          <span :title="absoluteTime(detail.updatedAt)">Updated {{ agoLabel(Date.parse(detail.updatedAt)) }}</span>
        </div>

        <div
          v-if="detail.blocked"
          class="mt-2 flex items-center gap-1.5 text-[11.5px] font-medium text-severity-error"
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
          <p
            v-if="statusError && !cascadeConfirmOpen && !cancelConfirmOpen"
            class="mt-1.5 text-[11.5px] text-severity-error"
            data-testid="task-status-error"
          >
            {{ statusError }}
          </p>
        </div>

        <!-- eslint-disable vue/no-v-html -- renderGithubMarkdown escapes raw HTML (githubMarkdown.spec.ts) -->
        <div
          v-if="bodyHtml"
          class="markdown-body mt-3.5 text-[13.5px] leading-[1.65] text-text-2"
          data-testid="task-detail-body"
          @click="onBodyClick"
          v-html="bodyHtml"
        />
        <!-- eslint-enable vue/no-v-html -->
      </div>

      <div class="px-5 pb-5 pt-4">
        <section v-if="(detail.blockers ?? []).length" data-testid="task-blockers">
          <h2 class="mb-2 font-mono text-[10.5px] tracking-[.12em] text-text-3">BLOCKERS</h2>
          <div class="flex flex-wrap gap-1.5">
            <!-- A chip with a title is a live task the user will want to
                 inspect, so it selects it in place; one without (the blocker
                 item was deleted, only the edge's id remains) has nothing to
                 open and stays inert. -->
            <template v-for="blocker in detail.blockers ?? []" :key="blocker.id">
              <button
                v-if="blocker.title"
                type="button"
                class="inline-flex cursor-pointer items-center gap-1.5 rounded-[5px] border border-card px-2 py-1 text-[11.5px] text-text-2 hover:border-strong hover:text-text"
                data-testid="task-blocker-chip"
                @click="select(blocker.id)"
              >
                <IconLink2 class="size-3 shrink-0 text-text-4" />{{ blocker.title }}
                <span
                  class="rounded-[4px] px-1 py-px text-[9.5px] font-medium"
                  :class="statusMeta(blocker.status).classes"
                  data-testid="task-blocker-status"
                  >{{ statusMeta(blocker.status).label }}</span
                >
              </button>
              <span
                v-else
                class="inline-flex items-center gap-1.5 rounded-[5px] border border-card px-2 py-1 text-[11.5px] italic text-text-4"
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
          <h2 class="mb-3 font-mono text-[10.5px] tracking-[.12em] text-text-3">COMMENTS</h2>
          <div class="flex flex-col gap-3.5">
            <div v-for="comment in detail.comments ?? []" :key="comment.id" data-testid="task-comment">
              <div class="mb-1 flex items-center gap-2">
                <BaseBadge
                  v-if="isCheckpoint(comment)"
                  tone="accent"
                  class="px-2 py-0.5 text-[10px] font-semibold"
                  data-testid="task-comment-checkpoint"
                >
                  <IconBookmarkCheck class="size-3" />CHECKPOINT
                </BaseBadge>
                <span class="font-mono text-[10.5px] text-text-4" :title="absoluteTime(comment.createdAt)">{{
                  agoLabel(Date.parse(comment.createdAt))
                }}</span>
              </div>
              <!-- eslint-disable vue/no-v-html -- renderGithubMarkdown escapes raw HTML (githubMarkdown.spec.ts) -->
              <div
                class="markdown-body text-[13px] leading-[1.6] text-text-2"
                @click="onBodyClick"
                v-html="commentHtml(comment)"
              />
              <!-- eslint-enable vue/no-v-html -->
            </div>
          </div>
        </section>

        <div class="mt-6 border-t border-border pt-4">
          <button
            type="button"
            class="flex items-center gap-1.5 rounded-lg border border-severity-error/40 px-3 py-1.5 text-[12px] font-medium text-severity-error hover:bg-severity-error-tint"
            data-testid="task-delete"
            @click="openDeleteConfirm"
          >
            <IconTrash2 class="size-3.5" />Delete
          </button>
        </div>
      </div>
    </template>
    <div v-else class="m-auto font-mono text-xs text-text-4" data-testid="task-detail-empty">
      Select a task to inspect
    </div>

    <ConfirmationDialog
      v-if="cascadeConfirmOpen"
      title="Close nested tasks?"
      :description="cascadeDescription"
      confirm-label="Confirm"
      :busy="statusBusy"
      :error="statusError"
      testid="task-cascade-confirm"
      @confirm="confirmPendingStatus"
      @cancel="cancelPendingStatus"
    />
    <ConfirmationDialog
      v-if="cancelConfirmOpen"
      title="Cancel this task?"
      description="This marks the task cancelled. It stays in the tree but drops out of the open filters."
      confirm-label="Cancel task"
      :busy="statusBusy"
      :error="statusError"
      testid="task-cancel-confirm"
      @confirm="confirmPendingStatus"
      @cancel="cancelPendingStatus"
    />
    <ConfirmationDialog
      v-if="deleteConfirmOpen"
      title="Delete task"
      description="Deletes this item and everything nested under it, comments included. This cannot be undone."
      confirm-label="Delete"
      :busy="deleteBusy"
      :error="deleteError"
      testid="task-delete-confirm"
      @confirm="confirmDelete"
      @cancel="deleteConfirmOpen = false"
    />
  </aside>
</template>
