<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRemoteConnection } from '../stores/useRemoteConnection'
import { useRemoteCatalog } from '../stores/useRemoteCatalog'
import BaseButton from './ui/BaseButton.vue'
import TextInput from './ui/TextInput.vue'
import InlineError from './ui/InlineError.vue'
import EmptyState from './ui/EmptyState.vue'
import RemoteNavigation from './RemoteNavigation.vue'
import RemoteTerminal from './RemoteTerminal.vue'

const props = defineProps<{ active: boolean }>()
const { endpoint, activeProfile, color } = useRemoteConnection()
const catalog = useRemoteCatalog()
const { workspace, chat, chatBusy, chatError, chats } = catalog
const name = ref('')
const selectedWorkspace = computed(() => catalog.workspaces.value.find((row) => row.dir === workspace.value))
const rows = computed(() => chats.value.filter((row) => row.workspace === workspace.value))
async function create(): Promise<void> {
  await catalog.openChat(name.value)
  if (!chatError.value) name.value = ''
}
</script>

<template>
  <div class="flex min-h-0 flex-1" data-testid="remote-chats">
    <aside class="flex w-64 shrink-0 flex-col overflow-y-auto border-r border-border">
      <BaseButton variant="ghost" data-testid="remote-chats-local" @click="catalog.selectWorkspace('')"
        >Local workspaces</BaseButton
      >
      <RemoteNavigation area="chats" />
      <div class="flex flex-col gap-1 p-2">
        <BaseButton
          v-for="row in rows"
          :key="row.id"
          variant="ghost"
          class="justify-start text-left"
          :style="{ color }"
          :disabled="chatBusy"
          :data-testid="`remote-chat-${row.id}`"
          @click="catalog.openChat('', row.id)"
          ><span class="truncate">{{ row.name }}</span></BaseButton
        >
      </div>
    </aside>
    <div class="flex min-h-0 min-w-0 flex-1 flex-col">
      <div class="flex flex-col gap-2 border-b border-border p-3">
        <p class="text-body font-semibold" :style="{ color }">
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
