<script setup lang="ts">
import IconCheck from '~icons/lucide/check'
import IconCircleAlert from '~icons/lucide/circle-alert'
import IconClock3 from '~icons/lucide/clock-3'
import IconExternalLink from '~icons/lucide/external-link'
import IconLoader from '~icons/lucide/loader'
import type { Job } from '../../bindings/github.com/colonyops/hive/internal/desktop/jobs/models'

defineProps<{ jobs: Job[] }>()
const emit = defineEmits<{ 'open-run': [commandId: number] }>()

function statusClasses(status: string): string {
  if (status === 'failed') return 'border-severity-error-border bg-severity-error-tint text-severity-error'
  if (status === 'done') return 'border-severity-success-border bg-severity-success-tint text-severity-success'
  return 'border-border bg-chip text-text-2'
}
</script>

<template>
  <section
    class="absolute right-0 top-[calc(100%+6px)] z-50 w-[340px] overflow-hidden rounded-lg border border-border bg-raised shadow-xl"
    style="--wails-draggable: no-drag"
    role="dialog"
    aria-label="Action jobs"
    data-testid="jobs-popover"
  >
    <header class="border-b border-border px-3.5 py-2.5">
      <div class="text-xs font-semibold text-text">Action jobs</div>
      <div class="mt-0.5 text-[10.5px] text-text-3">Running and recently completed work</div>
    </header>
    <ul class="max-h-80 divide-y divide-border overflow-y-auto">
      <li v-for="job in jobs" :key="job.id" class="flex items-start gap-2.5 px-3.5 py-3" :data-testid="`job-row-${job.id}`">
        <span class="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full border" :class="statusClasses(job.status)">
          <IconLoader v-if="job.status === 'running'" class="size-3.5 animate-spin" />
          <IconClock3 v-else-if="job.status === 'queued'" class="size-3.5" />
          <IconCheck v-else-if="job.status === 'done'" class="size-3.5" />
          <IconCircleAlert v-else class="size-3.5" />
        </span>
        <div class="min-w-0 flex-1">
          <div class="truncate text-[12px] font-medium text-text">{{ job.label || job.actionId }}</div>
          <div class="mt-0.5 flex items-center gap-1.5 text-[10.5px]">
            <span :class="job.status === 'failed' ? 'text-severity-error' : 'text-text-3'">{{ job.step }}</span>
            <span v-if="job.target" class="truncate text-text-4">· {{ job.target }}</span>
          </div>
          <div v-if="job.error" class="mt-1 line-clamp-2 text-[10.5px] text-severity-error">{{ job.error }}</div>
        </div>
        <button
          v-if="job.commandId"
          type="button"
          class="mt-0.5 flex size-6 shrink-0 cursor-pointer items-center justify-center rounded text-text-3 hover:bg-chip hover:text-text"
          :aria-label="`Open action run for ${job.label || job.actionId}`"
          title="Open action run"
          :data-testid="`job-open-run-${job.id}`"
          @click="emit('open-run', job.commandId)"
        ><IconExternalLink class="size-3.5" /></button>
      </li>
    </ul>
  </section>
</template>
