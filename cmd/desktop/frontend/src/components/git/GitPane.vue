<script setup lang="ts">
// The session's changed files. Read-only: every read goes through the session
// API, and nothing here writes to the checkout.
import { computed, toRef } from 'vue'
import { useStorage } from '@vueuse/core'
import IconGitBranch from '~icons/lucide/git-branch'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import SegmentedControl, { type SegmentedControlOption } from '@hive/ui/components/ui/SegmentedControl.vue'
import GitTree from './GitTree.vue'
import { useSessionChanges, type DiffBase } from '../../composables/useSessionChanges'
import { useToasts } from '../../composables/useToasts'
import { errorText } from '../../lib/appError'
import type { GitLayout } from '../../lib/gitTree'
import { OpenSessionInEditor } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'

const props = defineProps<{
  sessionId: string
  /** The file the route pins. */
  path: string | null
  layout: GitLayout
  /** Whether the pane is on screen. It is mounted, but hidden, on a trip to the hub. */
  active: boolean
}>()
const emit = defineEmits<{ pick: [path: string] }>()

const base = useStorage<DiffBase>('hive.git.base', 'branch')
const { changes, error } = useSessionChanges(toRef(props, 'sessionId'), base, () => props.active)

const shownBase = computed<DiffBase>(() => (changes.value?.base as DiffBase | undefined) || base.value)
const baseOptions = computed<SegmentedControlOption<DiffBase>[]>(() => [
  ...(changes.value?.branchAvailable
    ? [{ value: 'branch' as const, label: 'Branch', title: `Against ${changes.value.defaultBranch}` }]
    : []),
  { value: 'uncommitted', label: 'Uncommitted', title: 'Against HEAD' },
])

const files = computed(() => changes.value?.files ?? [])
const additions = computed(() => files.value.reduce((sum, file) => sum + file.additions, 0))
const deletions = computed(() => files.value.reduce((sum, file) => sum + file.deletions, 0))

const emptyMessage = computed(() =>
  shownBase.value === 'branch'
    ? `No changes against ${changes.value?.defaultBranch ?? 'the default branch'}`
    : 'No uncommitted changes',
)

const toasts = useToasts()
function openInEditor(): void {
  OpenSessionInEditor(props.sessionId).catch((err: unknown) =>
    toasts.showToast(errorText(err, 'Could not open the editor.'), { severity: 'error' }),
  )
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col" data-testid="git-pane">
    <div class="flex shrink-0 items-center gap-1.5 border-b border-border px-2 py-1">
      <IconGitBranch class="size-3 shrink-0 text-text-4" aria-hidden="true" />
      <span class="min-w-0 truncate font-mono text-caption text-text-2" data-testid="git-pane-branch">{{
        changes?.branch
      }}</span>
      <span class="min-w-0 flex-1" />
      <span class="shrink-0 font-mono text-caption" data-testid="git-pane-total">
        <span class="text-severity-success">+{{ additions }}</span>
        <span class="ml-1 text-severity-error">-{{ deletions }}</span>
      </span>
    </div>
    <div class="flex shrink-0 items-center border-b border-border px-2 py-1">
      <SegmentedControl
        :model-value="shownBase"
        :options="baseOptions"
        variant="compact"
        size="sm"
        aria-label="Compare against"
        testid="git-pane-base"
        @update:model-value="base = $event"
      />
    </div>
    <InlineError v-if="error" class="mx-2 mt-2 shrink-0" :message="error" testid="git-pane-error" />
    <div class="hive-scroll min-h-0 flex-1 overflow-y-auto">
      <EmptyState v-if="changes && !files.length" :message="emptyMessage" data-testid="git-pane-empty" />
      <GitTree
        v-else
        :files="files"
        :layout="layout"
        :selected="path"
        @pick="emit('pick', $event)"
        @open-editor="openInEditor"
      />
      <p v-if="changes?.truncated" class="px-3 pb-3 text-xs text-text-3" data-testid="git-pane-truncated">
        Showing the first {{ files.length }} files. Open the session in an editor to see the rest.
      </p>
    </div>
  </div>
</template>
