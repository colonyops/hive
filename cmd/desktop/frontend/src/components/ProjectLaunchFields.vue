<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import IconPlus from '~icons/lucide/plus'
import IconX from '~icons/lucide/x'
import AppSelect, { type AppSelectOption } from '@hive/ui/components/ui/AppSelect.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import SegmentedControl, { type SegmentedControlOption } from '@hive/ui/components/ui/SegmentedControl.vue'
import TextInput from '@hive/ui/components/ui/TextInput.vue'
import RepositorySelect from './RepositorySelect.vue'
import {
  DefaultCloneStrategy,
  ProjectDefinitions,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type {
  ProjectDefinitionView,
  ProjectRepoRequest,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import type { SessionLaunchRepository } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'
import { repoDisplayName } from '../lib/repositories'

/**
 * The repositories of a New project form: a project from config, or repos
 * picked here. Either way the list is editable, so a folder name or a clone
 * strategy can change for this one project.
 */
const props = defineProps<{ repositories: SessionLaunchRepository[] | null | undefined; testid: string }>()
const definition = defineModel<string>('definition', { required: true })
const repos = defineModel<ProjectRepoRequest[]>('repos', { required: true })

type Source = 'definition' | 'pick'
const definitions = ref<ProjectDefinitionView[]>([])
const source = ref<Source>('pick')
const adding = ref('')

const sourceOptions = computed<SegmentedControlOption<Source>[]>(() => [
  ...(definitions.value.length ? [{ value: 'definition' as const, label: 'From config' }] : []),
  { value: 'pick', label: 'Pick repos' },
])
const definitionOptions = computed<AppSelectOption[]>(() =>
  definitions.value.map((def) => ({
    value: def.name,
    label: def.name,
    hint: (def.repos ?? []).map((repo) => repo.dir).join(', '),
  })),
)
const strategyOptions: AppSelectOption[] = [
  { value: 'worktree', label: 'Worktree' },
  { value: 'full', label: 'Full clone' },
]

onMounted(async () => {
  try {
    definitions.value = (await ProjectDefinitions()) ?? []
  } catch {
    definitions.value = []
  }
  // A retried form arrives with its repositories; only an empty one takes the
  // first definition.
  if (definition.value) source.value = 'definition'
  else if (definitions.value.length && !repos.value.length) {
    source.value = 'definition'
    definition.value = definitions.value[0].name
  }
})

watch(source, (next) => {
  if (next === 'pick') definition.value = ''
  else if (!definition.value && definitions.value.length) definition.value = definitions.value[0].name
})

watch(definition, (name) => {
  const def = definitions.value.find((candidate) => candidate.name === name)
  if (def) repos.value = (def.repos ?? []).map((repo) => ({ ...repo }))
})

function folderFor(remote: string): string {
  const name = remote.replace(/\/+$/, '').split(/[/:]/).pop() ?? ''
  return name.replace(/\.git$/, '')
}

async function add(remote: string): Promise<void> {
  if (!remote || repos.value.some((repo) => repo.remote === remote)) return
  let strategy = 'full'
  try {
    strategy = (await DefaultCloneStrategy(remote)) || 'full'
  } catch {
    // The rules' default is a convenience; the user picks it in the row anyway.
  }
  repos.value = [...repos.value, { remote, dir: folderFor(remote), cloneStrategy: strategy }]
  adding.value = ''
}

watch(adding, (remote) => void add(remote))

function update(index: number, patch: Partial<ProjectRepoRequest>): void {
  repos.value = repos.value.map((repo, at) => (at === index ? { ...repo, ...patch } : repo))
}

function remove(index: number): void {
  repos.value = repos.value.filter((_, at) => at !== index)
}

const unpicked = computed(() =>
  (props.repositories ?? []).filter((option) => !repos.value.some((repo) => repo.remote === option.repository)),
)
</script>

<template>
  <div class="flex flex-col gap-2" :data-testid="`${testid}-project`">
    <div class="flex items-center gap-2">
      <SegmentedControl
        v-if="sourceOptions.length > 1"
        v-model="source"
        variant="compact"
        size="sm"
        :columns="sourceOptions.length"
        :options="sourceOptions"
        aria-label="Where the repositories come from"
        :testid="`${testid}-project-source`"
      />
      <AppSelect
        v-if="source === 'definition'"
        v-model="definition"
        class="min-w-0 flex-1"
        :options="definitionOptions"
        aria-label="Project"
        :testid="`${testid}-project-definition`"
      />
    </div>
    <div class="overflow-hidden rounded-lg border border-strong">
      <div
        class="grid grid-cols-[minmax(0,1.3fr)_minmax(0,0.9fr)_112px_24px] items-center gap-2 border-b border-border bg-sidebar px-2.5 py-1.5 font-mono text-micro tracking-wide text-text-4 uppercase"
      >
        <span>Repository</span><span>Folder</span><span>Checkout</span><span />
      </div>
      <div
        v-for="(repo, index) in repos"
        :key="repo.remote"
        class="grid grid-cols-[minmax(0,1.3fr)_minmax(0,0.9fr)_112px_24px] items-center gap-2 border-b border-border bg-app px-2.5 py-1.5"
        :data-testid="`${testid}-project-repo`"
        :data-remote="repo.remote"
      >
        <span class="min-w-0 truncate font-mono text-caption" :title="repo.remote">{{
          repoDisplayName(repo.remote) || repo.remote
        }}</span>
        <TextInput
          :model-value="repo.dir"
          class="font-mono"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
          :aria-label="`Folder for ${repo.remote}`"
          :data-testid="`${testid}-project-repo-dir`"
          @update:model-value="update(index, { dir: String($event) })"
        />
        <AppSelect
          :model-value="repo.cloneStrategy"
          :options="strategyOptions"
          :aria-label="`Checkout for ${repo.remote}`"
          :testid="`${testid}-project-repo-strategy`"
          @update:model-value="update(index, { cloneStrategy: String($event) })"
        />
        <IconButton
          :label="`Remove ${repo.remote}`"
          :icon="IconX"
          size="sm"
          :data-testid="`${testid}-project-repo-remove`"
          @click="remove(index)"
        />
      </div>
      <div class="flex items-center gap-2 bg-sidebar px-2.5 py-1.5">
        <IconPlus class="size-3 shrink-0 text-text-4" aria-hidden="true" />
        <RepositorySelect
          v-model="adding"
          class="min-w-0 flex-1"
          :repositories="unpicked"
          :testid="`${testid}-project-add`"
        />
      </div>
    </div>
    <p class="text-caption text-text-4">
      One agent opens in a folder that links every repository. The checkout defaults come from each repository's rules.
    </p>
  </div>
</template>
