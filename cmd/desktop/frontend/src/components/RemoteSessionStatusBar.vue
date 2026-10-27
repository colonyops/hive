<script setup lang="ts">
import { toRef } from 'vue'
import { useRouter } from 'vue-router'
import IconSettings from '~icons/lucide/settings'
import { useSessionStatus } from '../composables/useSessionStatus'
import { createRemoteCodeClient } from '../lib/remoteCodeClient'
import type { TerminalEndpoint } from '../lib/terminalClient'
import { useRemoteConnection } from '../stores/useRemoteConnection'
import PaneStatusBar from './PaneStatusBar.vue'
import SessionStatusChips from './SessionStatusChips.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'

const props = defineProps<{ endpoint: TerminalEndpoint; sessionId: string }>()
const router = useRouter()
const { activeProfile, color } = useRemoteConnection()
// The parent keys the bar by endpoint and session.
// eslint-disable-next-line vue/no-setup-props-reactivity-loss
const client = createRemoteCodeClient(props.endpoint)
const { git, pullRequest, pullRequestError, refresh } = useSessionStatus(toRef(props, 'sessionId'), client)
</script>

<template>
  <PaneStatusBar testid="remote-pane-statusbar" :revealable="false">
    <SessionStatusChips
      :git="git"
      :pull-request="pullRequest"
      :pull-request-error="pullRequestError"
      @refresh-pull-request="refresh({ refreshPullRequest: true })"
    />
    <template #actions>
      <span class="flex h-6 items-center gap-1.5 px-1.5 text-caption text-text-3" :title="activeProfile?.host">
        <span class="size-2 rounded-full" :style="{ backgroundColor: color }" aria-hidden="true" />{{
          activeProfile?.name
        }}
      </span>
      <IconButton
        label="Remote settings"
        :icon="IconSettings"
        data-testid="remote-open-settings"
        @click="router.push({ name: 'application-settings', params: { section: 'remote' } })"
      />
    </template>
  </PaneStatusBar>
</template>
