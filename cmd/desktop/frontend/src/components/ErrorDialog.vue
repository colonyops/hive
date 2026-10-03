<script setup lang="ts">
// The app-wide answer to "an operation failed and the user needs the details".
// A caller raises one through useErrorDialog(); this renders it with the two
// affordances that make a failure actionable: the full text on the clipboard,
// and the report dialog one click away.
//
// The error text is not prefilled into the issue: it names flows, nodes and
// repositories, so pasting it is the user's own call.
//
// The backdrop does not dismiss it: a failure the app decided to interrupt for
// should not close on a stray click before it has been read.
import { computed } from 'vue'
import IconCheck from '~icons/lucide/check'
import IconCircleAlert from '~icons/lucide/circle-alert'
import IconCopy from '~icons/lucide/copy'
import IconButton from './ui/IconButton.vue'
import BaseButton from './ui/BaseButton.vue'
import BaseModal from './ui/BaseModal.vue'
import { useClipboard } from '../composables/useClipboard'
import { useReportDialog } from '../composables/useReportDialog'
import { errorDetailsText, type ErrorDetails } from '../composables/useErrorDialog'

const props = defineProps<{ error: ErrorDetails }>()
const emit = defineEmits<{ close: [] }>()

const { copy, status: copyStatus } = useClipboard()
const { reportProblem: openIssue } = useReportDialog()

const text = computed(() => errorDetailsText(props.error))
const detail = computed(() => props.error.detail.trim() || 'No further detail was reported.')
const contextEntries = computed(() => Object.entries(props.error.context ?? {}))

const copyLabel = computed(() => {
  if (copyStatus.value === 'success') return 'Copied'
  if (copyStatus.value === 'error') return 'Copy failed'
  return 'Copy error details'
})

function reportProblem(): void {
  emit('close')
  void openIssue()
}
</script>

<template>
  <BaseModal
    :title="error.title"
    :icon="IconCircleAlert"
    tone="danger"
    aria-role="alertdialog"
    :width="560"
    :close-on-backdrop="false"
    testid="error-dialog"
    @close="emit('close')"
  >
    <div class="flex flex-col gap-3 px-5 py-4">
      <p v-if="error.summary" class="text-body leading-relaxed text-text-2" data-testid="error-dialog-summary">
        {{ error.summary }}
      </p>

      <dl
        v-if="contextEntries.length"
        class="flex flex-wrap gap-x-4 gap-y-1 text-small"
        data-testid="error-dialog-context"
      >
        <div v-for="[key, value] in contextEntries" :key="key" class="flex gap-1.5">
          <dt class="text-text-3">{{ key }}</dt>
          <dd class="font-mono text-text-2">{{ value }}</dd>
        </div>
      </dl>

      <!-- Error-toned, not a neutral code block: this is the failure itself,
           and Copy belongs beside it rather than in the footer among the
           dialog's own actions. -->
      <!-- The glyph and the button are h-5 boxes against the pre's leading-5,
           so both centre on the first line however many lines follow. The
           button's own box is larger than that line and gives the margin back,
           keeping a 28px target without pushing the row taller. -->
      <div
        class="flex items-start gap-2.5 rounded-lg border border-severity-error-border bg-severity-error-tint px-3 py-2.5"
        data-testid="error-dialog-message"
      >
        <span class="flex h-5 shrink-0 items-center"><IconCircleAlert class="size-4 text-severity-error" /></span>
        <pre
          class="hive-scroll max-h-[240px] min-w-0 flex-1 select-text overflow-auto whitespace-pre-wrap break-words font-mono text-small leading-5 text-severity-error"
          data-testid="error-dialog-detail"
          >{{ detail }}</pre>
        <IconButton
          :label="copyLabel"
          :icon="copyStatus === 'success' ? IconCheck : IconCopy"
          size="lg"
          tone="danger"
          class="-my-1 -mr-1"
          data-testid="error-dialog-copy"
          @click="copy(text)"
        />
      </div>

      <p class="text-caption text-text-3">
        Report a problem opens a GitHub issue. Copy this error first; it belongs in the issue body.
      </p>
    </div>

    <template #footer>
      <BaseButton class="flex-1" data-testid="error-dialog-close" @click="emit('close')">Close</BaseButton>
      <BaseButton variant="secondary" data-testid="error-dialog-report" @click="reportProblem"
        >Report a problem</BaseButton
      >
    </template>
  </BaseModal>
</template>
