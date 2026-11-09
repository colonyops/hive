<script setup lang="ts">
import { computed, type DeepReadonly } from 'vue'
import IconLayers from '~icons/lucide/layers'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import BaseModal from '@hive/ui/components/ui/BaseModal.vue'
import ProjectRepoRow from './terminal/ProjectRepoRow.vue'
import type { ProjectView } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'

const props = defineProps<{ project: DeepReadonly<ProjectView> }>()
const emit = defineEmits<{ close: [] }>()

const rows = computed(() => {
  const at = new Date(props.project.createdAt)
  return [
    ...(props.project.server ? [{ label: 'Server', value: props.project.server, testid: 'server' }] : []),
    ...(props.project.project ? [{ label: 'From config', value: props.project.project, testid: 'definition' }] : []),
    { label: 'Folder', value: props.project.root, mono: true, testid: 'root' },
    { label: 'Terminal session', value: props.project.slug, mono: true, testid: 'slug' },
    {
      label: 'Created',
      value: Number.isNaN(at.getTime()) ? props.project.createdAt : at.toLocaleString(),
      testid: 'created',
    },
  ]
})
</script>

<template>
  <BaseModal
    :title="project.name"
    :icon="IconLayers"
    :width="480"
    testid="project-detail-dialog"
    @close="emit('close')"
  >
    <dl class="flex flex-col gap-2.5 px-5 pt-4">
      <div v-for="row in rows" :key="row.testid" class="flex items-baseline gap-3">
        <dt class="w-31 shrink-0 text-xs text-text-3">{{ row.label }}</dt>
        <dd
          class="min-w-0 flex-1 break-all text-body text-text-2"
          :class="row.mono && 'font-mono text-small'"
          :data-testid="`project-detail-${row.testid}`"
        >
          {{ row.value }}
        </dd>
      </div>
    </dl>
    <div class="flex flex-col gap-1.5 px-5 pt-4 pb-4">
      <div class="text-xs text-text-3">Repositories</div>
      <ul class="-mx-2.25 flex flex-col gap-0.5" data-testid="project-detail-repos">
        <ProjectRepoRow v-for="repo in project.repos ?? []" :key="repo.sessionId" :repo="repo" />
      </ul>
    </div>
    <template #footer>
      <div class="flex-1" />
      <BaseButton variant="secondary" data-testid="project-detail-close" @click="emit('close')">Close</BaseButton>
    </template>
  </BaseModal>
</template>
