<script setup lang="ts">
import { computed, type DeepReadonly } from 'vue'
import IconGitPullRequest from '~icons/lucide/git-pull-request'
import IconCircleAlert from '~icons/lucide/circle-alert'
import { useSessionStatus } from '../../composables/useSessionStatus'
import type { ProjectRepoView } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const props = defineProps<{ repo: DeepReadonly<ProjectRepoView>; last: boolean }>()

const { git, pullRequest } = useSessionStatus(computed(() => (props.repo.missing ? '' : props.repo.sessionId)))

const branch = computed(() => git.value?.branch || props.repo.branch)
const title = computed(() =>
  props.repo.missing
    ? `${props.repo.dir}: its session is gone`
    : [props.repo.remote, branch.value, props.repo.cloneStrategy, props.repo.path].filter(Boolean).join('\n'),
)
</script>

<template>
  <div
    class="project-repo-row"
    :class="{ 'project-repo-row-last': last }"
    :title="title"
    data-testid="terminal-project-repo"
    :data-dir="repo.dir"
  >
    <component
      :is="repo.missing ? IconCircleAlert : 'span'"
      class="size-3 shrink-0"
      :class="repo.missing ? 'text-severity-error' : 'size-1.5 rounded-full bg-text-4'"
      aria-hidden="true"
    />
    <span class="min-w-0 truncate font-mono text-caption text-text-2">{{ repo.dir }}</span>
    <span class="min-w-0 flex-1 truncate font-mono text-micro text-text-4">{{ branch }}</span>
    <span
      v-if="git?.dirty"
      class="shrink-0 font-mono text-micro"
      data-testid="terminal-project-repo-dirty"
      :title="`${git.additions} added, ${git.deletions} removed, not committed`"
    >
      <span class="text-severity-success">+{{ git.additions }}</span>
      <span class="ml-0.5 text-severity-error">-{{ git.deletions }}</span>
    </span>
    <span
      v-if="pullRequest?.number"
      class="flex shrink-0 items-center gap-0.5 font-mono text-micro"
      :class="pullRequest.isDraft ? 'text-text-4' : 'text-accent'"
      :title="pullRequest.title"
      data-testid="terminal-project-repo-pr"
    >
      <IconGitPullRequest class="size-2.5" aria-hidden="true" />{{ pullRequest.number }}
    </span>
  </div>
</template>

<style scoped>
.project-repo-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  margin-left: 22px;
  padding: 2px 10px 2px 10px;
  border-left: 1px solid var(--color-border);
}

.project-repo-row-last {
  margin-bottom: 4px;
}
</style>
