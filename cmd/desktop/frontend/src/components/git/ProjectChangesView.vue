<script setup lang="ts">
import { computed, ref, type DeepReadonly } from 'vue'
import IconFolderGit from '~icons/lucide/folder-git-2'
import IconCircleAlert from '~icons/lucide/circle-alert'
import ChangesView from './ChangesView.vue'
import type { ProjectView } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

/**
 * A project's Changes: each repository's Changes view, stacked in one scroll
 * under a header that names the repository. Each view keeps its own target,
 * files and comments, and Send from any of them reaches the project's one
 * agent.
 */
const props = withDefaults(defineProps<{ project: DeepReadonly<ProjectView>; keysActive?: boolean }>(), {
  keysActive: true,
})

const repos = computed(() => props.project.repos ?? [])
// Only one view may answer the bare keys: the one the pointer or focus was
// last in.
const keyed = ref(0)
</script>

<template>
  <div class="flex h-full min-h-0 flex-1 flex-col overflow-y-auto" data-testid="project-changes-view">
    <section
      v-for="(repo, index) in repos"
      :key="repo.sessionId"
      class="flex h-full shrink-0 flex-col border-b border-strong last:border-b-0"
      data-testid="project-changes-repo"
      :data-dir="repo.dir"
      @pointerenter="keyed = index"
      @focusin="keyed = index"
    >
      <header
        class="sticky top-0 z-30 flex h-8 shrink-0 items-center gap-2 border-b border-border bg-sidebar px-3"
        data-testid="project-changes-repo-header"
      >
        <IconFolderGit class="size-3.5 shrink-0 text-accent" aria-hidden="true" />
        <span class="font-mono text-caption font-semibold text-text">{{ repo.dir }}</span>
        <span class="min-w-0 truncate font-mono text-caption text-text-4">{{ repo.remote }}</span>
        <span class="flex-1" />
        <span class="font-mono text-micro text-text-4">{{ repo.cloneStrategy }}</span>
        <span class="font-mono text-micro text-text-4">{{ index + 1 }}/{{ repos.length }}</span>
      </header>
      <div
        v-if="repo.missing"
        class="flex flex-1 items-center justify-center gap-2 text-caption text-text-3"
        data-testid="project-changes-repo-missing"
      >
        <IconCircleAlert class="size-3.5 text-severity-error" aria-hidden="true" />
        The session behind {{ repo.dir }} is gone.
      </div>
      <ChangesView
        v-else
        class="min-h-0 flex-1"
        :session-id="repo.sessionId"
        :keys-active="keysActive && keyed === index"
      />
    </section>
  </div>
</template>
