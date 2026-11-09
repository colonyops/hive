<script setup lang="ts">
import { computed, type DeepReadonly } from 'vue'
import IconCircleAlert from '~icons/lucide/circle-alert'
import IconGitBranch from '~icons/lucide/git-branch'
import IconGitPullRequest from '~icons/lucide/git-pull-request'
import { useSessionStatus } from '../../composables/useSessionStatus'
import { repoDisplayName } from '../../lib/repositories'
import type { ProjectRepoView } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const props = defineProps<{ repo: DeepReadonly<ProjectRepoView> }>()

const { git, pullRequest } = useSessionStatus(computed(() => (props.repo.missing ? '' : props.repo.sessionId)))

const branch = computed(() => git.value?.branch || props.repo.branch)
</script>

<template>
  <li
    class="flex flex-col gap-0.5 rounded-md px-2.25 py-1.75"
    data-testid="project-detail-repo"
    :data-dir="repo.dir"
    :title="repo.remote"
  >
    <div class="flex items-center gap-2">
      <component
        :is="repo.missing ? IconCircleAlert : IconGitBranch"
        class="size-3.5 shrink-0"
        :class="repo.missing ? 'text-severity-error' : 'text-text-3'"
        aria-hidden="true"
      />
      <span class="min-w-0 truncate text-body text-text">{{ repoDisplayName(repo.remote) || repo.remote }}</span>
      <span class="flex-1" />
      <span
        v-if="git?.dirty"
        class="shrink-0 font-mono text-micro"
        data-testid="project-detail-repo-dirty"
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
        data-testid="project-detail-repo-pr"
      >
        <IconGitPullRequest class="size-2.5" aria-hidden="true" />{{ pullRequest.number }}
      </span>
    </div>
    <div class="flex items-center gap-2 pl-5.5 text-caption text-text-4">
      <template v-if="repo.missing">The session behind {{ repo.dir }} is gone.</template>
      <template v-else>
        <span class="shrink-0 font-mono">{{ repo.dir }}/</span>
        <span class="min-w-0 flex-1 truncate font-mono">{{ branch }}</span>
        <span class="shrink-0">{{ repo.cloneStrategy === 'worktree' ? 'Worktree' : 'Full clone' }}</span>
      </template>
    </div>
  </li>
</template>
