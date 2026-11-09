<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import RepositorySelect, { type RepositoryPreset } from './RepositorySelect.vue'
import {
  DefaultCloneStrategy,
  ProjectDefinitions,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type {
  ProjectDefinitionView,
  ProjectRepoRequest,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import type { SessionLaunchRepository } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'

/**
 * The repositories of a New project form. A project from config fills the
 * set with its folders and checkouts; a repository picked here gets its
 * folder from its name and its checkout from the repository's rules.
 */
const props = withDefaults(
  defineProps<{
    repositories: SessionLaunchRepository[] | null | undefined
    testid: string
    id?: string
    /** The server the project is created on, '' for this machine. */
    server?: string
  }>(),
  { id: undefined, server: '' },
)
const definition = defineModel<string>('definition', { required: true })
const repos = defineModel<ProjectRepoRequest[]>('repos', { required: true })
const emit = defineEmits<{ preset: [name: string] }>()

const definitions = ref<ProjectDefinitionView[]>([])

onMounted(async () => {
  try {
    definitions.value = (await ProjectDefinitions(props.server)) ?? []
  } catch {
    definitions.value = []
  }
})

const presets = computed<RepositoryPreset[]>(() =>
  definitions.value.map((def) => ({ name: def.name, remotes: (def.repos ?? []).map((repo) => repo.remote) })),
)
const selected = computed(() => repos.value.map((repo) => repo.remote))

function folderFor(remote: string, taken: Set<string>): string {
  const base = (remote.replace(/\/+$/, '').split(/[/:]/).pop() ?? '').replace(/\.git$/, '') || 'repo'
  let dir = base
  for (let n = 2; taken.has(dir); n++) dir = `${base}-${n}`
  return dir
}

async function strategyFor(remote: string): Promise<string> {
  try {
    return (await DefaultCloneStrategy(props.server, remote)) || 'full'
  } catch {
    // The rules' default is a convenience; the create resolves it again.
    return 'full'
  }
}

async function select(remotes: string[]): Promise<void> {
  const kept = repos.value.filter((repo) => remotes.includes(repo.remote))
  const taken = new Set(kept.map((repo) => repo.dir))
  const added = remotes
    .filter((remote) => !kept.some((repo) => repo.remote === remote))
    .map((remote) => {
      const dir = folderFor(remote, taken)
      taken.add(dir)
      return { remote, dir, cloneStrategy: 'full' }
    })
  repos.value = [...kept, ...added]
  if (!repos.value.length) definition.value = ''
  for (const repo of added) {
    const strategy = await strategyFor(repo.remote)
    repos.value = repos.value.map((item) => (item.remote === repo.remote ? { ...item, cloneStrategy: strategy } : item))
  }
}

function applyPreset(name: string): void {
  const def = definitions.value.find((candidate) => candidate.name === name)
  if (!def) return
  definition.value = def.name
  repos.value = (def.repos ?? []).map((repo) => ({ ...repo }))
  emit('preset', def.name)
}
</script>

<template>
  <RepositorySelect
    :id="id"
    multiple
    :selected="selected"
    :presets="presets"
    :repositories="repositories"
    :server="server"
    :testid="`${testid}-project-repos`"
    @update:selected="select"
    @preset="applyPreset"
  />
</template>
