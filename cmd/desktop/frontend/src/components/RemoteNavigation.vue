<script setup lang="ts">
import { computed } from 'vue'
import { useIntervalFn } from '@vueuse/core'
import { useRouter } from 'vue-router'
import { useRemoteConnection } from '../stores/useRemoteConnection'
import { useRemoteCatalog } from '../stores/useRemoteCatalog'
import { useCodeInstallation } from '../stores/useCodeInstallation'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import IconMessagesSquare from '~icons/lucide/messages-square'

const props = defineProps<{ area: 'inbox' | 'code' | 'chats'; compact?: boolean }>()
const { endpoint, activeProfile, color, sessions, reload } = useRemoteConnection()
const catalog = useRemoteCatalog()
const installation = useCodeInstallation()
const router = useRouter()
useIntervalFn(() => {
  if (props.area === 'code' && endpoint.value) void reload()
}, 10000)
const rows = computed(() => {
  if (props.area === 'inbox') return catalog.inboxes.value
  if (props.area === 'chats') return catalog.workspaces.value.map((row) => ({ id: row.dir, name: row.name }))
  return sessions.value.filter((row) => row.state === 'active')
})
const error = computed(() =>
  props.area === 'inbox' ? catalog.inboxError.value : props.area === 'chats' ? catalog.workspaceError.value : null,
)
const selected = computed(() =>
  props.area === 'inbox'
    ? catalog.inboxId.value
    : props.area === 'chats'
      ? catalog.workspace.value
      : installation.installation.value === 'remote'
        ? catalog.codeId.value
        : '',
)
function select(id: string): void {
  if (props.area === 'inbox') {
    catalog.selectInbox(id)
    void router.push({ name: 'feed' })
  } else if (props.area === 'chats') {
    catalog.selectWorkspace(id)
    void router.push({ name: 'agents' })
  } else {
    catalog.selectCode(id)
    installation.select('remote')
    void router.push({ name: 'terminal' })
  }
}
function selectChat(workspace: string, id: string): void {
  if (catalog.workspace.value !== workspace) select(workspace)
  void catalog.openChat('', id)
}
</script>

<template>
  <div
    v-if="endpoint"
    class="flex shrink-0 flex-col"
    :class="compact ? 'gap-2.5' : 'remote-list'"
    :data-testid="`remote-${area}-menu`"
  >
    <template v-for="row in rows" :key="row.id">
      <BaseButton
        variant="ghost"
        size="sm"
        :class="[
          compact
            ? 'size-[38px] justify-center rounded-xl border border-card bg-chip font-mono'
            : area === 'chats'
              ? 'remote-row remote-workspace'
              : 'remote-row',
          { 'bg-hover': selected === row.id },
        ]"
        :style="{ borderLeft: `3px solid ${color}` }"
        :aria-pressed="selected === row.id"
        :aria-label="`${row.name} · Remote ${activeProfile?.name}`"
        :title="`${row.name} · Remote ${activeProfile?.name}`"
        :data-testid="`remote-${area}-item-${row.id}`"
        @click="select(row.id)"
      >
        <span class="truncate">{{ compact ? row.name.slice(0, 1) : row.name }}</span>
      </BaseButton>
      <div
        v-if="area === 'chats' && catalog.chats.value.some((session) => session.workspace === row.id)"
        class="remote-chat-list"
      >
        <BaseButton
          v-for="session in catalog.chats.value.filter((session) => session.workspace === row.id)"
          :key="session.id"
          variant="ghost"
          size="sm"
          class="remote-row remote-chat"
          :class="{ 'bg-hover': catalog.chat.value?.id === session.id && selected === row.id }"
          :style="{ borderLeft: `3px solid ${color}` }"
          :title="`${session.name} · Remote ${activeProfile?.name}`"
          :disabled="catalog.chatBusy.value"
          :data-testid="`remote-chat-${session.id}`"
          @click="selectChat(row.id, session.id)"
          ><IconMessagesSquare class="size-3.5 shrink-0 text-text-4" /><span class="truncate">{{
            session.name
          }}</span></BaseButton
        >
      </div>
    </template>
    <InlineError v-if="error" :message="error" :testid="`remote-${area}-menu-error`" />
  </div>
</template>

<style scoped>
.remote-row {
  justify-content: flex-start;
  height: 34px;
  padding: 0 12px;
  border-radius: 0;
  text-align: left;
}
.remote-row:hover {
  background: var(--color-chip);
}
.remote-workspace {
  height: 40px;
  border-top: 1px solid var(--color-border);
  color: var(--color-text);
}
.remote-chat-list {
  display: flex;
  flex-direction: column;
  border-top: 1px solid var(--color-border);
  background: var(--color-app);
  padding: 6px 0;
}
.remote-chat {
  gap: 8px;
  font-weight: 400;
}
</style>
