<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useRemoteConnection } from '../stores/useRemoteConnection'
import { useRemoteCatalog } from '../stores/useRemoteCatalog'
import { useCodeInstallation } from '../stores/useCodeInstallation'
import BaseButton from './ui/BaseButton.vue'
import InlineError from './ui/InlineError.vue'

const props = defineProps<{ area: 'inbox' | 'code' | 'chats'; compact?: boolean }>()
const { endpoint, activeProfile, color, sessions } = useRemoteConnection()
const catalog = useRemoteCatalog()
const installation = useCodeInstallation()
const router = useRouter()
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
      : catalog.codeId.value,
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
</script>

<template>
  <div
    v-if="endpoint"
    class="flex shrink-0 flex-col gap-1 border-t border-border p-2"
    :data-testid="`remote-${area}-menu`"
  >
    <p v-if="!compact" class="px-2 py-1 text-caption" :style="{ color }">Remote · {{ activeProfile?.name }}</p>
    <BaseButton
      v-for="row in rows"
      :key="row.id"
      variant="ghost"
      size="sm"
      class="justify-start gap-2"
      :style="{ color, borderLeft: selected === row.id ? `3px solid ${color}` : '3px solid transparent' }"
      :aria-label="`${row.name} · Remote ${activeProfile?.name}`"
      :title="`${row.name} · Remote ${activeProfile?.name}`"
      :data-testid="`remote-${area}-item-${row.id}`"
      @click="select(row.id)"
    >
      <span class="size-2 shrink-0 rounded-full" :style="{ backgroundColor: color }" aria-hidden="true" />
      <span class="truncate">{{ compact ? row.name.slice(0, 1) : row.name }}</span>
    </BaseButton>
    <InlineError v-if="error" :message="error" :testid="`remote-${area}-menu-error`" />
  </div>
</template>
