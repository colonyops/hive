<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useIntervalFn } from '@vueuse/core'
import { useRoute, useRouter } from 'vue-router'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconPlus from '~icons/lucide/plus'
import IconPlay from '~icons/lucide/play'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import NewWindowMenu from '../NewWindowMenu.vue'
import { useRemoteConnection } from '../../stores/useRemoteConnection'
import { useRemoteCode } from '../../stores/useRemoteCode'
import { useRemoteCatalog } from '../../stores/useRemoteCatalog'
import { useCodeInstallation } from '../../stores/useCodeInstallation'
import { useCommands, type Command } from '../../composables/useCommands'
import { useNewSession } from '../../composables/useNewSession'
import { repoDisplayName } from '../../lib/repositories'
import { activityIndicator } from '../../lib/agentActivity'
import { errorText } from '../../lib/appError'
import type { RemoteSession } from '../../lib/remoteConnection'

const props = defineProps<{ query?: string; runningOnly?: boolean }>()
const remote = useRemoteConnection()
const code = useRemoteCode()
const catalog = useRemoteCatalog()
const installation = useCodeInstallation()
const router = useRouter()
const route = useRoute()
const { openBlank } = useNewSession()
const collapsed = ref(new Set<string>())
const menu = ref('')
const busy = ref(false)
const actionError = ref<string | null>(null)
const groups = computed(() => {
  const repos = new Map<string, { repo: string; name: string; sessions: RemoteSession[] }>()
  for (const choice of code.options.value?.repositories ?? [])
    repos.set(choice.repository, { repo: choice.repository, name: repoDisplayName(choice.repository), sessions: [] })
  for (const row of remote.sessions.value.filter((row) => row.state === 'active')) {
    if (!repos.has(row.repo))
      repos.set(row.repo, { repo: row.repo, name: repoDisplayName(row.repo) || '(no remote)', sessions: [] })
    repos.get(row.repo)!.sessions.push(row)
  }
  const needle = props.query?.trim().toLowerCase() ?? ''
  return [...repos.values()]
    .map((group) => ({
      ...group,
      sessions: group.sessions
        .filter(
          (row) =>
            (!props.runningOnly || !!code.windows.value[row.slug]?.length) &&
            (!needle || group.name.toLowerCase().includes(needle) || row.name.toLowerCase().includes(needle)),
        )
        .sort((a, b) => a.name.localeCompare(b.name)),
    }))
    .filter(
      (group) =>
        group.sessions.length || (!props.runningOnly && (!needle || group.name.toLowerCase().includes(needle))),
    )
    .sort((a, b) => a.name.localeCompare(b.name))
})
function toggle(repo: string): void {
  if (collapsed.value.has(repo)) collapsed.value.delete(repo)
  else collapsed.value.add(repo)
}
function selected(row: RemoteSession): boolean {
  return installation.installation.value === 'remote' && catalog.codeId.value === row.id
}
function select(row: RemoteSession, window = ''): void {
  catalog.selectCode(row.id)
  code.selectWindow(window)
  installation.select('remote')
  void router.push({ name: 'terminal' })
}
function indicator(row: RemoteSession, window: string) {
  const state = code.statuses.value
    .find((item) => item.sessionId === row.id)
    ?.windows?.find((item) => item.windowId === window)
  return state ? activityIndicator(state.status, state.tool) : null
}
async function addWindow(row: RemoteSession, agent?: string): Promise<void> {
  const client = code.client.value
  const endpoint = remote.endpoint.value
  if (!client || busy.value) return
  busy.value = true
  actionError.value = null
  try {
    if (!code.windows.value[row.slug]?.length) {
      await client.start(row.slug)
      await code.reload()
      if (remote.endpoint.value === endpoint) select(row)
    } else {
      const created = agent ? await client.newAgentWindow(row.slug, agent) : await client.newWindow(row.slug)
      await code.reload()
      if (remote.endpoint.value === endpoint) select(row, created.windowId)
    }
  } catch (error) {
    actionError.value = errorText(error, 'Could not open the remote window.')
  } finally {
    busy.value = false
  }
}
watch(remote.sessions, () => void code.reload(), { immediate: true })
useIntervalFn(() => {
  if (route.name === 'terminal') void code.reload()
}, 5000)
useCommands((): Command[] => {
  if (route.name !== 'terminal' || !remote.endpoint.value) return []
  const profile = remote.activeProfile.value
  const commands: Command[] = groups.value
    .filter((group) => !!group.repo)
    .map((group) => ({
      id: `remote:${profile?.id}:repo:${group.repo}:new`,
      title: 'New session',
      group: `${group.name} · ${profile?.name}`,
      scope: 'actions',
      run: () => void openBlank(group.repo, 'repository', undefined, profile?.id),
    }))
  const row = remote.sessions.value.find((row) => selected(row))
  if (row) {
    commands.push({
      id: `remote:${profile?.id}:${row.id}:new-terminal`,
      title: 'New terminal',
      group: row.name,
      scope: 'actions',
      run: () => void addWindow(row),
    })
    if (code.windows.value[row.slug]?.length)
      for (const agent of code.options.value?.agents ?? [])
        commands.push({
          id: `remote:${profile?.id}:${row.id}:agent:${agent}`,
          title: `New ${agent} agent`,
          group: row.name,
          scope: 'actions',
          run: () => void addWindow(row, agent),
        })
  }
  return commands
})
</script>

<template>
  <div v-if="remote.endpoint.value" data-testid="remote-code-menu">
    <InlineError
      v-if="actionError || code.error.value || code.optionsError.value"
      :message="actionError || code.error.value || code.optionsError.value || ''"
      testid="remote-code-tree-error"
    />
    <div
      v-for="group in groups"
      :key="group.repo"
      class="border-t border-border"
      :style="{ borderLeft: `3px solid ${remote.color.value}` }"
    >
      <div
        class="repo-group"
        role="button"
        tabindex="0"
        :aria-expanded="!collapsed.has(group.repo)"
        :title="`${group.name} · ${remote.activeProfile.value?.name}`"
        data-testid="remote-code-repo"
        :data-repo="group.repo"
        @click="toggle(group.repo)"
        @keydown.enter.self.prevent="toggle(group.repo)"
        @keydown.space.self.prevent="toggle(group.repo)"
      >
        <span class="min-w-0 flex-1 truncate text-body text-text">{{ group.name }}</span>
        <span class="truncate text-caption text-text-4">{{ remote.activeProfile.value?.name }}</span>
        <IconButton
          v-if="group.repo"
          :icon="IconPlus"
          :label="`New session in ${group.name}`"
          size="sm"
          data-testid="remote-repo-new-session"
          @click.stop="openBlank(group.repo, 'repository', undefined, remote.activeProfile.value?.id)"
        />
        <component
          :is="collapsed.has(group.repo) ? IconChevronRight : IconChevronDown"
          class="size-3 shrink-0 text-text-4"
        />
      </div>
      <div v-if="!collapsed.has(group.repo)" class="border-t border-border bg-app py-1">
        <div v-for="row in group.sessions" :key="row.id">
          <div
            class="session-row"
            :class="{ 'session-row-attached': selected(row), 'menu-open': menu === row.id }"
            role="button"
            tabindex="0"
            :data-testid="`remote-code-item-${row.id}`"
            :aria-label="`${row.name} · Remote ${remote.activeProfile.value?.name}`"
            :aria-selected="selected(row)"
            @click="select(row)"
            @keydown.enter.self.prevent="select(row)"
            @keydown.space.self.prevent="select(row)"
          >
            <span class="min-w-0 flex-1 truncate text-body">{{ row.name }}</span>
            <span
              v-if="code.windows.value[row.slug]?.length"
              class="size-2 rounded-full bg-severity-success"
              title="Terminal running"
            />
            <IconButton
              :icon="code.windows.value[row.slug]?.length ? IconPlus : IconPlay"
              label="New window"
              size="sm"
              :disabled="busy"
              data-testid="remote-new-window"
              @click.stop="menu = menu === row.id ? '' : row.id"
            />
            <NewWindowMenu
              v-if="menu === row.id"
              :running="!!code.windows.value[row.slug]?.length"
              :flip="false"
              :ignore="[]"
              :agents="code.options.value?.agents ?? []"
              :default-agent="code.options.value?.defaultAgent ?? ''"
              :loading="code.loadingOptions.value"
              :failed="!!code.optionsError.value"
              @close="menu = ''"
              @terminal="addWindow(row)"
              @agent="addWindow(row, $event)"
              @retry="code.reloadOptions"
            />
          </div>
          <div
            v-for="(window, index) in code.windows.value[row.slug] ?? []"
            :key="window.windowId"
            class="window-row"
            :class="{
              'window-row-last': index === (code.windows.value[row.slug]?.length ?? 0) - 1,
              'window-row-active': selected(row) && code.selectedWindow.value === window.windowId,
            }"
            role="button"
            tabindex="0"
            data-testid="remote-code-window"
            :data-window-id="window.windowId"
            @click="select(row, window.windowId)"
            @keydown.enter.prevent="select(row, window.windowId)"
            @keydown.space.prevent="select(row, window.windowId)"
          >
            <span class="min-w-0 flex-1 truncate font-mono text-small">{{ window.name }}</span>
            <component
              :is="indicator(row, window.windowId)?.icon"
              v-if="indicator(row, window.windowId)"
              class="size-3.5"
              :class="indicator(row, window.windowId)?.color"
              :title="indicator(row, window.windowId)?.label"
            />
          </div>
        </div>
        <p v-if="!group.sessions.length" class="px-5 py-2 text-caption text-text-4">No sessions</p>
      </div>
    </div>
  </div>
</template>

<style scoped src="./terminalSidebar.css"></style>
