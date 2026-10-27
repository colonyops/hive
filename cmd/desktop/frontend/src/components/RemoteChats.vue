<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRemoteConnection } from '../stores/useRemoteConnection'
import { useRemoteCatalog } from '../stores/useRemoteCatalog'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import TextInput from '@hive/ui/components/ui/TextInput.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import RemoteTerminal from './RemoteTerminal.vue'

const props = defineProps<{ active: boolean }>()
const { endpoint, activeProfile } = useRemoteConnection()
const catalog = useRemoteCatalog()
const { workspace, chat, chatBusy, chatError } = catalog
const name = ref('')
const selectedWorkspace = computed(() => catalog.workspaces.value.find((row) => row.dir === workspace.value))
async function create(): Promise<void> {
  await catalog.openChat(name.value)
  if (!chatError.value) name.value = ''
}
</script>

<template>
  <div class="flex min-h-0 flex-1" data-testid="remote-chats">
    <div class="flex min-h-0 min-w-0 flex-1 flex-col">
      <div class="flex flex-col gap-2 border-b border-border p-3">
        <p class="text-body font-semibold">
          {{ selectedWorkspace?.name || workspace }} · Remote {{ activeProfile?.name }}
        </p>
        <p v-if="selectedWorkspace?.notice" class="text-small text-text-3">{{ selectedWorkspace.notice }}</p>
        <InlineError v-if="selectedWorkspace?.problem" :message="selectedWorkspace.problem" />
        <form class="flex items-center gap-2" @submit.prevent="create">
          <TextInput
            v-model="name"
            aria-label="New remote chat name"
            placeholder="New chat name"
            data-testid="remote-chat-name"
            class="min-w-0 flex-1"
            :disabled="chatBusy"
          />
          <BaseButton
            type="submit"
            :busy="chatBusy"
            :disabled="!selectedWorkspace || !!selectedWorkspace.problem"
            data-testid="remote-chat-new"
            class="shrink-0 whitespace-nowrap"
            >New chat</BaseButton
          >
        </form>
        <InlineError v-if="chatError" :message="chatError" testid="remote-chat-error" />
        <p v-if="chat?.notice" class="text-small text-text-3">{{ chat.notice }}</p>
      </div>
      <RemoteTerminal
        v-if="props.active && chat && endpoint"
        :key="`${endpoint.httpBaseURL}/${chat.id}/${chat.terminalId}`"
        :slug="chat.terminalId"
        :endpoint="endpoint"
      />
      <EmptyState v-else message="Start a new chat or select an existing remote chat." />
    </div>
  </div>
</template>
