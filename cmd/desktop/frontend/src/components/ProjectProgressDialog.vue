<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import IconCheck from '~icons/lucide/check'
import IconCircleDot from '~icons/lucide/circle-dot'
import IconLayers from '~icons/lucide/layers'
import IconRotateCcw from '~icons/lucide/rotate-ccw'
import IconX from '~icons/lucide/x'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import BaseModal from '@hive/ui/components/ui/BaseModal.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import { useProjectCreate } from '../stores/useProjectCreate'
import { StepState } from '../../bindings/github.com/colonyops/hive/internal/hive/project/models'

const emit = defineEmits<{ retry: [] }>()
const { progress, watching, error, succeeded, failed, close } = useProjectCreate()
const router = useRouter()

const steps = computed(() =>
  (progress.value?.steps ?? []).map((step) => ({
    ...step,
    label: step.dir || 'root',
    what: describe(step.dir, step.state, step.detail, step.remote),
  })),
)
const ready = computed(() => steps.value.filter((step) => step.dir && step.state === StepState.StepDone).length)
const total = computed(() => steps.value.filter((step) => step.dir).length)

function describe(dir: string, state: StepState, detail: string, remote: string): string {
  if (!dir) {
    if (state === StepState.StepRunning) return 'Linking repositories, writing AGENTS.md, starting the agent'
    if (state === StepState.StepDone) return 'Agent started'
    if (state === StepState.StepFailed) return detail
    return 'Link repositories, write AGENTS.md, start the agent'
  }
  switch (state) {
    case StepState.StepRunning:
      return `Checking out ${remote} (${detail})`
    case StepState.StepDone:
      return detail
    case StepState.StepFailed:
      return detail
    case StepState.StepRemoved:
      return 'Checked out, then removed'
    default:
      return `${remote} (${detail})`
  }
}

// A project that started opens in Code, as a new session does.
watch(
  () => succeeded.value,
  (ok) => {
    if (!ok || !watching.value) return
    const slug = progress.value?.slug
    close()
    if (slug) void router.push({ name: 'terminal', params: { slug } })
  },
)
</script>

<template>
  <BaseModal
    :title="failed ? `${progress?.name} was not created` : `Creating ${progress?.name}`"
    :icon="IconLayers"
    :width="560"
    :close-on-backdrop="false"
    testid="project-progress-dialog"
    @close="close"
  >
    <div class="flex flex-col gap-3 px-5 py-4">
      <InlineError v-if="failed" :message="progress?.error ?? ''" testid="project-progress-error" />
      <InlineError v-if="error" :message="error" />
      <div class="overflow-hidden rounded-lg border border-strong">
        <div
          v-for="step in steps"
          :key="step.label"
          class="flex items-center gap-2.5 border-b border-border bg-app px-3 py-2 last:border-b-0"
          data-testid="project-progress-step"
          :data-dir="step.dir"
          :data-state="step.state"
        >
          <Spinner v-if="step.state === 'running'" class="size-3 shrink-0" />
          <IconCheck v-else-if="step.state === 'done'" class="size-3.5 shrink-0 text-severity-success" />
          <IconX v-else-if="step.state === 'failed'" class="size-3.5 shrink-0 text-severity-error" />
          <IconRotateCcw v-else-if="step.state === 'removed'" class="size-3.5 shrink-0 text-text-3" />
          <IconCircleDot v-else class="size-3.5 shrink-0 text-text-4" />
          <span class="w-24 shrink-0 truncate font-mono text-caption">{{ step.label }}</span>
          <span class="min-w-0 flex-1 truncate text-caption text-text-3" :title="step.what">{{ step.what }}</span>
        </div>
      </div>
      <p class="text-caption text-text-3">
        <template v-if="failed">Hive removed the repositories it had already checked out.</template>
        <template v-else>{{ ready }} of {{ total }} repositories ready. If one fails, Hive removes the rest.</template>
      </p>
    </div>
    <template #footer>
      <BaseButton v-if="failed" class="flex-1" data-testid="project-progress-retry" @click="emit('retry')">
        Edit and try again
      </BaseButton>
      <BaseButton variant="secondary" data-testid="project-progress-close" @click="close">
        {{ failed ? 'Close' : 'Run in background' }}
      </BaseButton>
    </template>
  </BaseModal>
</template>
