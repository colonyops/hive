<script setup lang="ts">
// The selected task, laid out after Linear's issue panel. The list docks it in
// TaskDetailPane; the board opens it in a drawer. Self-contained via useTasks()
// (the same module singleton TasksView reads) rather than props/emit — it owns
// everything about the selection: identity, the status control, description,
// sub-tasks, blockers, activity, and delete.
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import { computed, ref, watch } from 'vue'
import { useResizeObserver } from '@vueuse/core'
import { Browser } from '@wailsio/runtime'
import IconBan from '~icons/lucide/ban'
import IconBookmarkCheck from '~icons/lucide/bookmark-check'
import IconCheck from '~icons/lucide/check'
import IconCopy from '~icons/lucide/copy'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconLayers from '~icons/lucide/layers'
import IconCircleDot from '~icons/lucide/circle-dot'
import IconLink2 from '~icons/lucide/link-2'
import IconTerminal from '~icons/lucide/terminal'
import IconTrash2 from '~icons/lucide/trash-2'
import IconX from '~icons/lucide/x'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import AppSelect, { type AppSelectOption } from '@hive/ui/components/ui/AppSelect.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import ConfirmationHost from '@hive/ui/components/ui/ConfirmationHost.vue'
import { useClipboard } from '@hive/ui/composables/useClipboard'
import { useCodeCopy } from '@hive/ui/composables/useCodeCopy'
import { useTasks } from '../../stores/useTasks'
import { useTerminalSessions } from '../../stores/useTerminalSessions'
import { relativeAgo } from '../../lib/age'
import { renderGithubMarkdown } from '@hive/ui/lib/githubMarkdown'
import { useCodeHighlight } from '@hive/ui/lib/highlight'
import { externalMarkdownHref } from '@hive/ui/lib/markdownLinks'
import { checkpointBody, isCheckpoint, matchesTaskFilter, statusMeta, TASK_STATUSES } from '../../lib/tasksPresentation'
import type { TaskComment } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import TaskStatusIcon from './TaskStatusIcon.vue'
import { taskStatusIcon } from './taskStatusIcon'
import { useTaskDelete } from './useTaskDelete'
import { useTaskStatusChange } from './useTaskStatusChange'

defineProps<{ closable?: boolean }>()
const emit = defineEmits<{ close: [] }>()

const { detail, items, selectedId, select } = useTasks()

const STATUS_OPTIONS: AppSelectOption[] = TASK_STATUSES.map((status) => ({
  value: status,
  label: statusMeta(status).label,
  icon: taskStatusIcon(status),
}))

const parent = computed(() => {
  const parentId = detail.value?.parentId
  return parentId ? (items.value.find((item) => item.id === parentId) ?? null) : null
})

const subtasks = computed(() => {
  const id = detail.value?.id
  if (!id) return []
  return items.value
    .filter((item) => item.parentId === id)
    .slice()
    .sort((a, b) => Date.parse(a.createdAt) - Date.parse(b.createdAt))
})
const subtasksDone = computed(
  () => subtasks.value.filter((item) => item.status === 'done' || item.status === 'cancelled').length,
)

const typeLabel = computed(() => (detail.value?.type === 'epic' ? 'Epic' : 'Task'))
const { highlight } = useCodeHighlight()
const bodyHtml = computed(() => (detail.value?.desc ? renderGithubMarkdown(detail.value.desc, highlight) : ''))

// A plan-sized description would push sub-tasks and activity off screen, so
// a long one starts as a preview.
const DESCRIPTION_PREVIEW_PX = 280
const descriptionEl = ref<HTMLElement | null>(null)
const descriptionLong = ref(false)
const descriptionExpanded = ref(false)
const descriptionClamped = computed(() => descriptionLong.value && !descriptionExpanded.value)
useResizeObserver(descriptionEl, () => {
  descriptionLong.value = (descriptionEl.value?.scrollHeight ?? 0) > DESCRIPTION_PREVIEW_PX + 40
})

function absoluteTime(iso: string): string {
  return new Date(iso).toLocaleString()
}

function commentHtml(comment: TaskComment): string {
  return renderGithubMarkdown(checkpointBody(comment), highlight)
}

// Rendered bodies are untrusted markdown; links must open in the user's real
// browser rather than navigate the webview away from the app.
const { copyCode } = useCodeCopy()

function onBodyClick(event: MouseEvent): void {
  if (copyCode(event)) return
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
const statusChange = useTaskStatusChange()
const statusError = statusChange.error

function requestStatusChange(next: string): void {
  if (detail.value) statusChange.request(detail.value, next)
}

const taskDelete = useTaskDelete()

function requestDelete(): void {
  if (detail.value) taskDelete.request(detail.value.id)
}

// A new selection must not inherit the previous one's failure or a half-open
// confirm: a poll can clear a vanished selection while a dialog is up, and a
// lingering error would blame the wrong task.
watch(selectedId, () => {
  descriptionExpanded.value = false
  statusChange.reset()
  taskDelete.confirmation.cancel()
})
</script>

<template>
  <template v-if="detail">
    <div
      class="flex h-11 shrink-0 items-center gap-1.5 border-b border-row bg-pane pl-5 pr-3 text-small text-text-3"
      data-testid="task-detail-breadcrumb"
    >
      <span class="shrink-0 truncate">{{ detail.repoKey }}</span>
      <template v-if="parent">
        <IconChevronRight class="size-3 shrink-0 text-text-4" aria-hidden="true" />
        <button
          type="button"
          class="inline-flex min-w-0 cursor-pointer items-center gap-1 rounded px-1 py-0.5 hover:bg-row-hover hover:text-text"
          data-testid="task-detail-parent"
          @click="select(parent.id)"
        >
          <IconLayers class="size-3 shrink-0 text-accent" aria-hidden="true" /><span class="truncate">{{
            parent.title
          }}</span>
        </button>
      </template>
      <div class="min-w-2 flex-1" />
      <button
        type="button"
        class="flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md px-1.5 py-1 font-mono text-caption text-text-3 hover:bg-row-hover hover:text-text"
        :aria-label="`Copy ${detail.id}`"
        data-testid="task-detail-copy-id"
        @click="copyId"
      >
        <span class="max-w-[110px] truncate">{{ idCopied ? 'Copied' : detail.id }}</span>
        <component :is="idCopied ? IconCheck : IconCopy" class="size-3 shrink-0" />
      </button>
      <IconButton
        label="Delete task"
        :icon="IconTrash2"
        tone="danger"
        data-testid="task-delete"
        @click="requestDelete"
      />
      <IconButton
        v-if="closable"
        label="Close details"
        :icon="IconX"
        data-testid="task-detail-close"
        @click="emit('close')"
      />
    </div>

    <div class="hive-scroll min-h-0 flex-1 overflow-y-auto" data-testid="task-detail-scroll">
      <div class="flex flex-col gap-6 px-6 pb-8 pt-5">
        <div>
          <h1
            class="text-heading font-semibold leading-[1.35] tracking-[-.01em] text-text"
            data-testid="task-detail-title"
          >
            {{ detail.title }}
          </h1>
          <div
            v-if="detail.blocked"
            class="mt-3 flex items-center gap-1.5 rounded-md border border-severity-error-border bg-severity-error-tint px-2.5 py-1.5 text-small font-medium text-severity-error"
            data-testid="task-detail-blocked"
          >
            <IconBan class="size-3 shrink-0" aria-hidden="true" />{{ blockedReason }}
          </div>
        </div>

        <div>
          <div class="flex flex-wrap items-center gap-1.5" data-testid="task-detail-properties">
            <div class="w-[136px] rounded-md border border-card">
              <AppSelect
                :model-value="detail.status"
                :options="STATUS_OPTIONS"
                size="sm"
                variant="flat"
                class="-m-px"
                aria-label="Status"
                testid="task-status-select"
                @update:model-value="requestStatusChange"
              />
            </div>
            <span
              class="inline-flex h-[31px] items-center gap-1.5 rounded-md border border-card px-2.5 text-caption text-text-2"
              data-testid="task-detail-type"
            >
              <component
                :is="detail.type === 'epic' ? IconLayers : IconCircleDot"
                class="size-3.5"
                :class="detail.type === 'epic' ? 'text-accent' : 'text-text-4'"
                aria-hidden="true"
              />{{ typeLabel }}
            </span>
            <span
              v-if="detail.sessionId"
              class="inline-flex h-[31px] min-w-0 max-w-[220px] items-center gap-1.5 rounded-md border border-card px-2.5 text-caption text-text-2"
              :title="`Session ${sessionLabel}`"
              data-testid="task-detail-session"
            >
              <IconTerminal class="size-3.5 shrink-0 text-text-4" aria-hidden="true" /><span class="truncate">{{
                sessionLabel
              }}</span>
            </span>
          </div>
          <InlineError
            v-if="statusError"
            testid="task-status-error"
            variant="line"
            class="mt-1.5"
            :message="statusError"
          />
          <div class="mt-2.5 flex gap-1.5 text-small text-text-4" data-testid="task-detail-meta">
            <span :title="absoluteTime(detail.createdAt)">Created {{ relativeAgo(Date.parse(detail.createdAt)) }}</span>
            <span aria-hidden="true">·</span>
            <span :title="absoluteTime(detail.updatedAt)">Updated {{ relativeAgo(Date.parse(detail.updatedAt)) }}</span>
          </div>
        </div>

        <section v-if="subtasks.length" data-testid="task-subtasks">
          <h2 class="mb-2 flex items-center gap-2 text-small font-medium text-text-3">
            Sub-tasks
            <span class="font-mono text-caption text-text-4">{{ subtasksDone }}/{{ subtasks.length }}</span>
          </h2>
          <div class="overflow-hidden rounded-lg border border-card">
            <button
              v-for="subtask in subtasks"
              :key="subtask.id"
              type="button"
              class="flex h-9 w-full cursor-pointer items-center gap-2.5 border-b border-row px-3 text-left text-body text-text-2 last:border-b-0 hover:bg-row-hover hover:text-text"
              :data-id="subtask.id"
              data-testid="task-subtask"
              @click="select(subtask.id)"
            >
              <TaskStatusIcon :status="subtask.status" />
              <span class="min-w-0 flex-1 truncate">{{ subtask.title }}</span>
              <IconBan v-if="subtask.blocked" class="size-3 shrink-0 text-severity-error" aria-label="Blocked" />
              <span class="shrink-0 font-mono text-caption text-text-4">{{ subtask.id }}</span>
            </button>
          </div>
        </section>

        <section v-if="(detail.blockers ?? []).length" data-testid="task-blockers">
          <h2 class="mb-2 text-small font-medium text-text-3">Blocked by</h2>
          <div class="overflow-hidden rounded-lg border border-card">
            <!-- A row with a title is a live task the user will want to
               inspect, so it selects it in place; one without (the blocker
               item was deleted, only the edge's id remains) has nothing to
               open and stays inert. -->
            <template v-for="blocker in detail.blockers ?? []" :key="blocker.id">
              <button
                v-if="blocker.title"
                type="button"
                class="flex h-9 w-full cursor-pointer items-center gap-2.5 border-b border-row px-3 text-left text-body text-text-2 last:border-b-0 hover:bg-row-hover hover:text-text"
                data-testid="task-blocker-chip"
                @click="select(blocker.id)"
              >
                <TaskStatusIcon :status="blocker.status" />
                <span class="min-w-0 flex-1 truncate">{{ blocker.title }}</span>
                <span
                  class="shrink-0 rounded-sm px-1.5 py-px text-caption font-medium"
                  :class="statusMeta(blocker.status).classes"
                  data-testid="task-blocker-status"
                  >{{ statusMeta(blocker.status).label }}</span
                >
              </button>
              <span
                v-else
                class="flex h-9 items-center gap-2.5 border-b border-row px-3 text-body italic text-text-4 last:border-b-0"
                data-testid="task-blocker-chip"
                ><IconLink2 class="size-3.5 shrink-0" aria-hidden="true" />{{ blocker.id }}</span
              >
            </template>
          </div>
        </section>

        <section data-testid="task-detail-description">
          <h2 class="mb-2 text-small font-medium text-text-3">Description</h2>
          <template v-if="bodyHtml">
            <div class="relative" :class="descriptionClamped ? 'max-h-[280px] overflow-hidden' : ''">
              <!-- eslint-disable vue/no-v-html -- renderGithubMarkdown escapes raw HTML (githubMarkdown.spec.ts) -->
              <div
                ref="descriptionEl"
                class="markdown-body text-reading leading-[1.65] text-text-2"
                data-testid="task-detail-body"
                @click="onBodyClick"
                v-html="bodyHtml"
              />
              <!-- eslint-enable vue/no-v-html -->
              <div
                v-if="descriptionClamped"
                class="pointer-events-none absolute inset-x-0 bottom-0 h-16 bg-linear-to-t from-pane to-transparent"
                aria-hidden="true"
              />
            </div>
            <button
              v-if="descriptionLong"
              type="button"
              class="mt-1.5 cursor-pointer text-small font-medium text-accent hover:underline"
              :aria-expanded="descriptionExpanded"
              data-testid="task-detail-body-toggle"
              @click="descriptionExpanded = !descriptionExpanded"
            >
              {{ descriptionExpanded ? 'Show less' : 'Show more' }}
            </button>
          </template>
          <p v-else class="text-body italic text-text-4" data-testid="task-detail-no-body">No description.</p>
        </section>

        <section v-if="(detail.comments ?? []).length" data-testid="task-comments">
          <h2 class="mb-3 text-small font-medium text-text-3">Activity</h2>
          <ol class="flex flex-col">
            <li
              v-for="comment in detail.comments ?? []"
              :key="comment.id"
              class="relative border-l border-card pb-4 pl-4 last:pb-0"
              data-testid="task-comment"
            >
              <span
                class="absolute -left-[4.5px] top-1.5 size-2 rounded-full"
                :class="isCheckpoint(comment) ? 'bg-accent' : 'bg-strong'"
                aria-hidden="true"
              />
              <div class="mb-1.5 flex items-center gap-2">
                <BaseBadge
                  v-if="isCheckpoint(comment)"
                  tone="accent"
                  class="px-2 py-0.5 text-caption font-semibold"
                  data-testid="task-comment-checkpoint"
                >
                  <IconBookmarkCheck class="size-3" />CHECKPOINT
                </BaseBadge>
                <span class="text-small text-text-4" :title="absoluteTime(comment.createdAt)">{{
                  relativeAgo(Date.parse(comment.createdAt))
                }}</span>
              </div>
              <!-- eslint-disable vue/no-v-html -- renderGithubMarkdown escapes raw HTML (githubMarkdown.spec.ts) -->
              <div
                class="markdown-body rounded-lg border border-card bg-raised px-3.5 py-2.5 text-body leading-[1.6] text-text-2"
                @click="onBodyClick"
                v-html="commentHtml(comment)"
              />
              <!-- eslint-enable vue/no-v-html -->
            </li>
          </ol>
        </section>
      </div>
    </div>
  </template>
  <EmptyState v-else class="m-auto font-mono" message="Select a task to inspect" data-testid="task-detail-empty" />

  <ConfirmationHost :confirmation="taskDelete.confirmation" />
  <ConfirmationHost :confirmation="statusChange.confirmation" />
</template>
