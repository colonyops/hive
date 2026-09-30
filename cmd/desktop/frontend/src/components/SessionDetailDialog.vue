<script setup lang="ts">
// Read-only view of one hive session. The session list carries a projection —
// enough to render and attach — so everything else about a session is read here
// on demand rather than shipped with every row.
import { computed } from 'vue'
import IconTerminal from '~icons/lucide/terminal'
import BaseButton from './BaseButton.vue'
import BaseModal from './BaseModal.vue'
import type { SessionDetail } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/dispatch/models'

const props = defineProps<{ detail: SessionDetail }>()
const emit = defineEmits<{ close: [] }>()

function timestamp(value: string): string {
  const at = new Date(value)
  return Number.isNaN(at.getTime()) ? value : at.toLocaleString()
}

const rows = computed(() => {
  const list: { label: string; value: string; mono?: boolean; testid: string }[] = [
    { label: 'State', value: props.detail.state, testid: 'state' },
    { label: 'Repository', value: props.detail.repo || '—', testid: 'repo' },
    { label: 'Path', value: props.detail.path, mono: true, testid: 'path' },
    { label: 'Terminal session', value: props.detail.slug, mono: true, testid: 'slug' },
    { label: 'Clone strategy', value: props.detail.cloneStrategy || 'full', testid: 'clone-strategy' },
  ]
  if (props.detail.worktreeBranch) {
    list.push({ label: 'Worktree branch', value: props.detail.worktreeBranch, mono: true, testid: 'worktree-branch' })
  }
  list.push(
    { label: 'Tags', value: props.detail.tags?.length ? props.detail.tags.join(', ') : '—', testid: 'tags' },
    { label: 'Created', value: timestamp(props.detail.createdAt), testid: 'created' },
    { label: 'Updated', value: timestamp(props.detail.updatedAt), testid: 'updated' },
    { label: 'Id', value: props.detail.id, mono: true, testid: 'id' },
  )
  return list
})
</script>

<template>
  <BaseModal :title="detail.name" :icon="IconTerminal" :width="480" testid="session-detail-dialog" @close="emit('close')">
    <dl class="flex flex-col gap-2.5 px-5 py-4">
      <div v-for="row in rows" :key="row.testid" class="flex items-baseline gap-3">
        <dt class="w-[124px] shrink-0 text-xs text-text-3">{{ row.label }}</dt>
        <dd
          class="min-w-0 flex-1 break-all text-[13px] text-text-2"
          :class="row.mono && 'font-mono text-[12.5px]'"
          :data-testid="`session-detail-${row.testid}`"
        >{{ row.value }}</dd>
      </div>
    </dl>
    <template #footer>
      <div class="flex-1" />
      <BaseButton variant="secondary" data-testid="session-detail-close" @click="emit('close')">Close</BaseButton>
    </template>
  </BaseModal>
</template>
