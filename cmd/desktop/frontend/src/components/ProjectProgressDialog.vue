<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import IconCheck from '~icons/lucide/check'
import IconCircleAlert from '~icons/lucide/circle-alert'
import IconGitBranch from '~icons/lucide/git-branch'
import IconLayers from '~icons/lucide/layers'
import IconUndo from '~icons/lucide/undo-2'
import IconX from '~icons/lucide/x'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import BaseModal from '@hive/ui/components/ui/BaseModal.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import { useProjectCreate } from '../stores/useProjectCreate'
import { repoDisplayName } from '../lib/repositories'
import { StepState } from '../../bindings/github.com/colonyops/hive/internal/hive/project/models'

const emit = defineEmits<{ retry: [] }>()
const { progress, request, watching, error, succeeded, failed, close } = useProjectCreate()
const router = useRouter()

const steps = computed(() =>
  (progress.value?.steps ?? []).map((step) => ({
    ...step,
    key: step.dir || ':root',
    label: step.dir ? repoDisplayName(step.remote) || step.dir : 'Agent',
    hint: hint(step.dir, step.state),
  })),
)
const repoSteps = computed(() => steps.value.filter((step) => step.dir))
const ready = computed(() => repoSteps.value.filter((step) => step.state === StepState.StepDone).length)
const server = computed(() => request.value?.server ?? '')

function hint(dir: string, state: StepState): string {
  switch (state) {
    case StepState.StepRunning:
      return dir ? 'Checking out…' : 'Starting…'
    case StepState.StepDone:
      return dir ? 'Ready' : 'Started'
    case StepState.StepFailed:
      return 'Failed'
    case StepState.StepRemoved:
      return 'Removed'
    default:
      return 'Waiting'
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
    :width="480"
    :close-on-backdrop="false"
    testid="project-progress-dialog"
    @close="close"
  >
    <section
      v-if="failed"
      class="mx-5 mt-4 rounded-lg border border-severity-error-border bg-severity-error-tint px-3.5 py-3"
      data-testid="project-progress-failure"
    >
      <div class="flex items-start gap-2">
        <IconCircleAlert class="mt-px size-3.75 shrink-0 text-severity-error" />
        <div class="min-w-0 flex-1">
          <div class="text-small font-semibold text-severity-error">This project was not created</div>
          <p class="mt-0.5 break-words text-small leading-[1.45] text-text-2" data-testid="project-progress-error">
            {{ progress?.error }}
          </p>
          <p class="mt-2 text-caption leading-[1.45] text-text-3">
            Hive removed the repositories it had already checked out.
          </p>
        </div>
      </div>
    </section>
    <div class="flex flex-col gap-3 px-5 py-4">
      <InlineError v-if="error" variant="line" :message="error" />
      <div v-if="!failed" class="flex items-center justify-between text-caption text-text-3">
        <span>{{ ready }} of {{ repoSteps.length }} repositories ready</span>
        <span v-if="server" class="text-text-4">on {{ server }}</span>
      </div>
      <ul class="flex flex-col gap-0.5" data-testid="project-progress-steps">
        <li
          v-for="step in steps"
          :key="step.key"
          class="flex items-center gap-2.5 rounded-md px-2.25 py-1.75"
          :class="step.state === StepState.StepRunning ? 'bg-hover' : ''"
          data-testid="project-progress-step"
          :data-dir="step.dir"
          :data-state="step.state"
          :title="step.detail || step.remote"
        >
          <span class="flex size-4 shrink-0 items-center justify-center">
            <Spinner v-if="step.state === StepState.StepRunning" class="size-3.5" />
            <IconCheck v-else-if="step.state === StepState.StepDone" class="size-3.5 text-accent" :stroke-width="3" />
            <IconX v-else-if="step.state === StepState.StepFailed" class="size-3.5 text-severity-error" />
            <IconUndo v-else-if="step.state === StepState.StepRemoved" class="size-3.5 text-text-4" />
            <component :is="step.dir ? IconGitBranch : IconLayers" v-else class="size-3.5 text-text-4" />
          </span>
          <span
            class="min-w-0 flex-1 truncate text-body"
            :class="
              step.state === StepState.StepPending || step.state === StepState.StepRemoved ? 'text-text-3' : 'text-text'
            "
            >{{ step.label }}</span
          >
          <span
            class="shrink-0 text-caption"
            :class="step.state === StepState.StepFailed ? 'text-severity-error' : 'text-text-4'"
            >{{ step.hint }}</span
          >
        </li>
      </ul>
    </div>
    <template #footer>
      <BaseButton v-if="failed" class="flex-1" data-testid="project-progress-retry" @click="emit('retry')">
        Edit and try again
      </BaseButton>
      <div v-else class="flex-1" />
      <BaseButton variant="secondary" data-testid="project-progress-close" @click="close">
        {{ failed ? 'Close' : 'Run in background' }}
      </BaseButton>
    </template>
  </BaseModal>
</template>
