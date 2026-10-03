<script setup lang="ts">
import { computed, ref } from 'vue'
import IconBug from '~icons/lucide/bug'
import AppSelect, { type AppSelectOption } from '../ui/AppSelect.vue'
import BaseButton from '../ui/BaseButton.vue'
import TokenConnectDrawer, { type TokenProvider } from './TokenConnectDrawer.vue'
import {
  Connect,
  Disconnect,
  Projects,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/posthogservice'
import type { useTokenConnection } from '../../composables/useTokenConnection'
import type { Project } from '../../types/posthog'

type Run = ReturnType<typeof useTokenConnection>['run']

const emit = defineEmits<{ close: [] }>()

const provider: TokenProvider = {
  id: 'posthog',
  name: 'PostHog',
  icon: IconBug,
  noun: 'project',
  connectLabel: 'Connect a project',
  hint: 'The key is validated once and stored in your keychain; only the host and project id are written to disk.',
  docsUrl: 'https://posthog.com/docs/api/personal-api-keys',
  tokenPage: { path: '/settings/user-api-keys', label: 'API keys on your instance' },
  urlPlaceholder: 'https://us.posthog.com',
  tokenPlaceholder: 'phx_…',
  defaultUrl: 'https://us.posthog.com',
  disconnect: Disconnect,
}

// A personal API key spans projects, so connecting is two steps: list what the
// key can see, then bind the project the user picks.
const projects = ref<Project[]>([])
const selectedProject = ref<number | null>(null)

// AppSelect is string-valued and a project id is numeric, so the conversion
// happens here rather than leaking a stringly-typed id into the connect call.
const projectOptions = computed<AppSelectOption[]>(() =>
  projects.value.map((project) => ({ value: String(project.id), label: `${project.name} (#${project.id})` })),
)
const selectedProjectValue = computed(() => (selectedProject.value === null ? '' : String(selectedProject.value)))

function chooseProject(value: string) {
  const id = Number(value)
  selectedProject.value = Number.isFinite(id) ? id : null
}

async function onLoadProjects(run: Run, url: string, token: string) {
  const ok = await run(async () => {
    projects.value = (await Projects(url, token)) ?? []
  }, 'PostHog rejected the API key.')
  if (ok) selectedProject.value = projects.value[0]?.id ?? null
}

async function onConnect(run: Run, url: string, token: string) {
  const projectID = selectedProject.value
  if (projectID === null) return
  // The key stays so a second project can be connected without re-pasting it.
  if (await run(() => Connect(url, token, projectID), 'PostHog rejected the connection.')) startOver()
}

function startOver(clearError?: () => void) {
  selectedProject.value = null
  projects.value = []
  clearError?.()
}
</script>

<template>
  <TokenConnectDrawer :provider="provider" :locked="projects.length > 0" @close="emit('close')">
    Create a <span class="text-text-2">personal API key</span> with the <span class="text-text-2">project:read</span>,
    <span class="text-text-2">error_tracking:read</span> and <span class="text-text-2">alert:read</span> scopes, then
    paste it below. One key can connect several projects — connect each one separately to route them to different feeds.

    <template #extra-step="{ url, token, ready, busy, run, clearError }">
      <template v-if="projects.length > 0">
        <AppSelect
          :model-value="selectedProjectValue"
          :options="projectOptions"
          aria-label="PostHog project"
          testid="posthog-connect-project"
          @update:model-value="chooseProject"
        />
        <div class="flex gap-2">
          <BaseButton
            size="sm"
            :busy="busy"
            :disabled="selectedProject === null"
            data-testid="posthog-connect-submit"
            @click="onConnect(run, url, token)"
            >Connect</BaseButton
          >
          <BaseButton variant="secondary" size="sm" data-testid="posthog-connect-back" @click="startOver(clearError)"
            >Use another key</BaseButton
          >
        </div>
      </template>
      <div v-else>
        <BaseButton
          size="sm"
          :busy="busy"
          :disabled="!ready"
          data-testid="posthog-connect-load"
          @click="onLoadProjects(run, url, token)"
          >Find projects</BaseButton
        >
      </div>
    </template>
  </TokenConnectDrawer>
</template>
