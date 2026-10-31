<script setup lang="ts">
// Chooses what the Changes view compares: a kind of change or one commit, and
// for a branch comparison the branch to compare with.
import { computed } from 'vue'
import IconChevronLeft from '~icons/lucide/chevron-left'
import IconChevronRight from '~icons/lucide/chevron-right'
import AppSelect, { type AppSelectOption } from '@hive/ui/components/ui/AppSelect.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import type { DiffBase } from '../../composables/useSessionChanges'
import { relativeAgo } from '../../lib/age'
import { branchName, isRemoteRef, shortCommit } from '../../lib/diffTarget'
import type { SessionChanges } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const props = defineProps<{
  changes: SessionChanges | null
  branches: readonly string[]
  base: DiffBase
  /** The commit picked by id. Empty follows HEAD. */
  commit: string
}>()
const emit = defineEmits<{
  'pick-target': [base: DiffBase, commit: string]
  'pick-against': [ref: string]
  'step-commit': [delta: 1 | -1]
}>()

const LAST_COMMIT = 'last-commit'

const commits = computed(() => props.changes?.commits ?? [])
const againstLabel = computed(() =>
  props.changes?.against ? branchName(props.changes.against) : (props.changes?.defaultBranch ?? 'the default branch'),
)

const targetOptions = computed<AppSelectOption[]>(() => [
  ...(props.changes?.branchAvailable
    ? [{ value: 'branch', label: 'Branch changes', hint: `Everything since ${againstLabel.value}, committed or not` }]
    : []),
  { value: 'uncommitted', label: 'Uncommitted', hint: 'Staged and unstaged, against HEAD' },
  { value: 'staged', label: 'Staged', hint: 'What the next commit holds' },
  { value: 'unstaged', label: 'Unstaged', hint: 'Not staged yet, plus untracked files' },
  { value: LAST_COMMIT, label: 'Last commit', hint: commits.value[0]?.subject },
  ...commits.value.map((commit) => ({
    value: commit.id,
    label: commit.subject,
    hint: `${shortCommit(commit.id)} · ${commit.author} · ${relativeAgo(commit.unix * 1000)}`,
    group: 'Commits',
  })),
])

const targetValue = computed(() => (props.base === 'commit' ? props.commit || LAST_COMMIT : props.base))

function pickTarget(value: string): void {
  if (value === LAST_COMMIT) emit('pick-target', 'commit', '')
  else if (commits.value.some((commit) => commit.id === value)) emit('pick-target', 'commit', value)
  else emit('pick-target', value as DiffBase, '')
}

const againstOptions = computed<AppSelectOption[]>(() => {
  const option = (ref: string, group: string) => ({ value: ref, label: branchName(ref), group })
  return [
    { value: '', label: `${props.changes?.defaultBranch ?? 'Default branch'} (default)` },
    ...props.branches.filter((ref) => !isRemoteRef(ref)).map((ref) => option(ref, 'Local')),
    ...props.branches.filter(isRemoteRef).map((ref) => option(ref, 'Remote')),
  ]
})

// Newest first, so an older commit has the higher index.
const commitIndex = computed(() => commits.value.findIndex((commit) => commit.id === props.changes?.commit))
const hasOlder = computed(() => commitIndex.value >= 0 && commitIndex.value < commits.value.length - 1)
const hasNewer = computed(() => commitIndex.value > 0)
</script>

<template>
  <div class="flex shrink-0 items-center gap-1.5">
    <AppSelect
      class="w-64"
      :model-value="targetValue"
      :options="targetOptions"
      size="sm"
      searchable
      search-placeholder="Search commits…"
      aria-label="What to review"
      testid="changes-target"
      @update:model-value="pickTarget"
    />
    <template v-if="base === 'branch'">
      <span class="text-caption text-text-3">against</span>
      <AppSelect
        class="w-36"
        :model-value="changes?.against ?? ''"
        :options="againstOptions"
        size="sm"
        searchable
        search-placeholder="Search branches…"
        aria-label="Compare against"
        testid="changes-against"
        @update:model-value="emit('pick-against', $event)"
      />
    </template>
    <template v-if="base === 'commit'">
      <IconButton
        label="Older commit"
        tooltip="Older commit ([)"
        :icon="IconChevronLeft"
        size="sm"
        :disabled="!hasOlder"
        data-testid="changes-commit-older"
        @click="emit('step-commit', 1)"
      />
      <span class="font-mono text-caption text-text-3" data-testid="changes-commit-position">
        {{
          commitIndex >= 0 ? `${commits.length - commitIndex}/${commits.length}` : shortCommit(changes?.commit ?? '')
        }}
      </span>
      <IconButton
        label="Newer commit"
        tooltip="Newer commit (])"
        :icon="IconChevronRight"
        size="sm"
        :disabled="!hasNewer"
        data-testid="changes-commit-newer"
        @click="emit('step-commit', -1)"
      />
    </template>
  </div>
</template>
