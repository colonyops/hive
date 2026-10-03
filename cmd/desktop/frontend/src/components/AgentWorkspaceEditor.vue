<script setup lang="ts">
// Create/edit editor for an agent workspace's manifest. It writes the fields
// it shows (name, command, the mcps, skills and schedules lists, and the
// directory name at creation); hand-written comments in the YAML survive the
// write. Deleting the workspace also lives here, because the editor is the
// workspace's whole management surface and its sidebar row has no menu.
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import IconArrowLeft from '~icons/lucide/arrow-left'
import IconExternalLink from '~icons/lucide/external-link'
import IconFolderCog from '~icons/lucide/folder-cog'
import IconFolderOpen from '~icons/lucide/folder-open'
import IconX from '~icons/lucide/x'
import BaseButton from './ui/BaseButton.vue'
import DrawerSheet from './ui/DrawerSheet.vue'
import FormField from './ui/FormField.vue'
import InlineConfirm from './ui/InlineConfirm.vue'
import InlineError from './ui/InlineError.vue'
import TextInput from './ui/TextInput.vue'
import CommandField from './workspace/CommandField.vue'
import McpServerList from './workspace/McpServerList.vue'
import ScheduleEditor from './workspace/ScheduleEditor.vue'
import ScheduleList from './workspace/ScheduleList.vue'
import SkillPackageList from './workspace/SkillPackageList.vue'
import { TextField } from '../pipeline/fields'
import { useScheduleDraft } from '../composables/useScheduleDraft'
import { useAgentWorkspaces } from '../stores/useAgentWorkspaces'
import type { AgentWorkspace, WorkspaceEditRequest } from '../lib/agentWorkspacesClient'
import { seedRef } from '../lib/seedRef'

const props = defineProps<{
  /** The workspace being edited, or null to create one. */
  workspace?: AgentWorkspace | null
  busy?: boolean
  error?: string | null
}>()
const emit = defineEmits<{
  close: []
  save: [request: WorkspaceEditRequest]
  delete: [dir: string]
}>()

const { root, editor, presets, openWorkspaceInEditor, revealWorkspace } = useAgentWorkspaces()

const creating = computed(() => !props.workspace)

// The confirm strip names the folder it is about to delete, in full: the
// directory name alone reads as a label in the app, not as a path on disk.
const deletedPath = computed(() =>
  root.value ? `${root.value}/${props.workspace?.dir ?? ''}` : (props.workspace?.dir ?? ''),
)

// A manifest the loader could not read leaves every list on this form empty,
// and Save reconciles the file to what the form holds, so it would rewrite
// mcps:, skills: and schedules: to nothing. The Go side refuses such an update
// (KindInvalid); the form refuses it first, and says what to fix instead.
const manifestProblem = computed(() => (creating.value ? '' : (props.workspace?.problem ?? '')))

const dir = seedRef(() => props.workspace?.dir ?? '')
const name = seedRef(() => props.workspace?.name ?? '')
const command = seedRef(() => props.workspace?.command ?? '')
const selectedMCPs = seedRef<string[]>(() => props.workspace?.mcps ?? [])
const selectedSkills = seedRef<string[]>(() => props.workspace?.skills ?? [])

const sheet = ref<{ body: HTMLElement | null } | null>(null)
const schedules = reactive(
  useScheduleDraft({
    schedules: () => props.workspace?.schedules ?? [],
    workspace: () => props.workspace?.dir ?? '',
    scroller: () => sheet.value?.body,
  }),
)

// What Save would write, in one comparable shape. A schedule whose next run
// time moved while the sheet was open changed the card, not the manifest, and
// must not make the form dirty.
function formState(): string {
  return JSON.stringify({
    dir: dir.value.trim(),
    name: name.value.trim(),
    command: command.value.trim(),
    mcps: [...selectedMCPs.value].sort(),
    skills: [...selectedSkills.value].sort(),
    schedules: schedules.edits(),
  })
}

const baseline = ref(formState())
const dirty = computed(() => formState() !== baseline.value)

// A watch, not an initializer: presets arrive with the area's overview, which
// can land after this sheet is mounted.
watch(
  presets,
  (rows) => {
    if (!creating.value || command.value || !rows.length) return
    command.value = rows[0].command
    // A default the form fills in for itself is not an edit the user has to be
    // asked about on the way out.
    baseline.value = formState()
  },
  { immediate: true },
)

const headerTitle = computed(() => {
  if (schedules.draft) return schedules.draft.key === null ? 'New schedule' : 'Edit schedule'
  return creating.value ? 'New workspace' : 'Edit workspace'
})

const headerSubtitle = computed(() => {
  const draft = schedules.draft
  if (draft)
    return schedules.draftId(draft) || `Starts a chat in ${name.value.trim() || 'this workspace'} on its own timetable`
  return creating.value ? 'A directory an agent works in' : props.workspace!.dir
})

const actionError = ref('')

async function openOutside(open: (dir: string) => Promise<void>, fallback: string): Promise<void> {
  actionError.value = ''
  try {
    await open(props.workspace!.dir)
  } catch (failure) {
    actionError.value = failure instanceof Error ? failure.message : fallback
  }
}

const valid = computed(
  () =>
    !!name.value.trim() && !!command.value.trim() && (!creating.value || !!dir.value.trim()) && !manifestProblem.value,
)

const confirming = ref(false)
const discarding = ref(false)

function submit(): void {
  if (props.busy || confirming.value || schedules.draft || !valid.value) return
  emit('save', {
    dir: creating.value ? dir.value.trim() : props.workspace!.dir,
    name: name.value.trim(),
    command: command.value.trim(),
    mcps: selectedMCPs.value,
    skills: selectedSkills.value,
    schedules: schedules.edits(),
  })
}

// Escape and the backdrop step back one surface: off the schedule page while
// one is open, and out of the sheet otherwise.
function cancel(): void {
  if (props.busy || confirming.value || discarding.value) return
  if (schedules.draft) schedules.close()
  else closeSheet()
}

// Every gesture that leaves asks before it throws an edit away. The question
// is a footer strip rather than a second dialog: a modal over the sheet doubles
// the backdrop and both would answer the same Escape.
function closeSheet(): void {
  if (props.busy || confirming.value) return
  if (dirty.value) discarding.value = true
  else emit('close')
}

const nameInput = ref<{ focus: () => void } | null>(null)
const dirInput = ref<{ focus: () => void } | null>(null)
onMounted(async () => {
  await nextTick()
  ;(creating.value ? dirInput.value : nameInput.value)?.focus()
})
</script>

<template>
  <DrawerSheet
    ref="sheet"
    :ariaLabel="creating ? 'New workspace' : 'Edit workspace'"
    :title="headerTitle"
    :subtitle="headerSubtitle"
    :icon="IconFolderCog"
    header-size="lg"
    testid="agent-workspace-editor"
    :default-size="520"
    :close-on-escape="!confirming && !discarding && !schedules.draft?.removing"
    :close-on-backdrop="!confirming && !discarding"
    @close="cancel"
  >
    <template v-if="schedules.draft" #icon>
      <button
        type="button"
        class="flex size-[38px] shrink-0 cursor-pointer items-center justify-center rounded-xl border border-card text-text-2 hover:border-strong hover:text-text"
        aria-label="Back to the workspace"
        data-testid="agent-workspace-editor-schedule-back"
        @click="schedules.close()"
      >
        <IconArrowLeft class="size-[18px]" />
      </button>
    </template>
    <template #header-actions>
      <button
        class="text-text-3 hover:text-text disabled:opacity-50"
        aria-label="Close"
        data-testid="agent-workspace-editor-close"
        :disabled="busy || confirming || discarding"
        @click="closeSheet"
      >
        <IconX class="size-4" />
      </button>
    </template>

    <!-- The schedule page takes the body over, the way the confirm strip takes
         the footer: one surface at a time, so Escape has one thing to answer. -->
    <ScheduleEditor
      v-if="schedules.draft"
      v-model:draft="schedules.draft"
      :busy="busy"
      :issue="schedules.issue"
      :previewable="!!workspace"
    />

    <!-- While a delete is pending the form recedes: dimmed and inert, so the
         two states can't be misread for each other. -->
    <div
      v-else
      class="flex flex-col gap-7 transition-opacity"
      :class="{ 'pointer-events-none opacity-45': confirming || discarding }"
    >
      <!-- Above the open/reveal actions on purpose: the fix is in the file,
           and those two buttons are what reach it. -->
      <InlineError v-if="manifestProblem" class="flex flex-col gap-1" testid="agent-workspace-editor-problem">
        <p>{{ manifestProblem }}</p>
        <p>
          Fix agent-workspace.yaml before saving from here. This form could not read the file, so saving would rewrite
          its lists as empty. The buttons below open it.
        </p>
      </InlineError>

      <div v-if="!creating" class="flex flex-col gap-1.5">
        <div class="flex items-center gap-2">
          <BaseButton
            v-if="editor.command"
            variant="secondary"
            size="xs"
            class="shrink-0"
            :disabled="busy"
            data-testid="agent-workspace-editor-open-editor"
            @click="openOutside(openWorkspaceInEditor, 'The editor could not be opened.')"
          >
            <template #icon><IconExternalLink class="size-3.5" /></template>Open in {{ editor.title }}
          </BaseButton>
          <BaseButton
            variant="secondary"
            size="xs"
            class="shrink-0"
            :disabled="busy"
            data-testid="agent-workspace-editor-reveal"
            @click="openOutside(revealWorkspace, 'The directory could not be opened.')"
          >
            <template #icon><IconFolderOpen class="size-3.5" /></template>Show in Finder
          </BaseButton>
        </div>
        <span v-if="!editor.command" class="text-xs text-text-4"
          >Pick a default editor in Settings › General to open this directory in it.</span
        >
        <InlineError
          v-if="actionError"
          testid="agent-workspace-editor-action-error"
          variant="line"
          :message="actionError"
        />
      </div>

      <div class="flex flex-col gap-3">
        <FormField
          v-if="creating"
          v-slot="{ id }"
          label="Directory name"
          hint="A new directory under the workspace root, seeded with an AGENTS.md to shape."
        >
          <TextInput
            :id="id"
            ref="dirInput"
            v-model="dir"
            placeholder="my-project"
            autocapitalize="off"
            autocorrect="off"
            spellcheck="false"
            aria-label="Directory name"
            :disabled="busy"
            data-testid="agent-workspace-editor-dir"
            @keydown.enter="submit"
            monospace
          />
        </FormField>
        <TextField
          ref="nameInput"
          v-model="name"
          label="Name"
          :disabled="busy"
          testid="agent-workspace-editor-name"
          @keydown.enter="submit"
        />
      </div>

      <CommandField v-model="command" :busy="busy" />
      <McpServerList v-model="selectedMCPs" :busy="busy" />
      <SkillPackageList v-model="selectedSkills" :busy="busy" />
      <ScheduleList v-model:cards="schedules.cards" :busy="busy" @open="schedules.open" @run="schedules.runNow" />

      <p v-if="!creating" class="border-t border-border pt-4 text-xs leading-relaxed text-text-4">
        Saving rewrites these fields in agent-workspace.yaml and re-syncs the workspace's generated files. Comments and
        anything else in the file stay as written — edit the file for those.
      </p>
      <InlineError v-if="error && !confirming" :message="error" testid="agent-workspace-editor-error" />
    </div>

    <template #footer>
      <!-- The strip escapes the footer's own padding so it reads as the
           sheet's bottom edge, the way InlineConfirm is designed to sit. -->
      <InlineConfirm
        v-if="schedules.draft?.removing"
        class="-mx-[18px] -my-[13px]"
        title="Remove this schedule?"
        description="It leaves agent-workspace.yaml when you save the workspace."
        confirm-label="Remove"
        testid="agent-workspace-editor-schedule-remove-confirm"
        @confirm="schedules.remove"
        @cancel="schedules.draft.removing = false"
      />
      <div v-else-if="schedules.draft" class="flex items-center gap-2.5">
        <button
          v-if="schedules.draft.key !== null"
          type="button"
          class="cursor-pointer text-small text-text-3 hover:text-severity-error disabled:opacity-50"
          :disabled="busy"
          data-testid="agent-workspace-editor-schedule-remove"
          @click="schedules.draft.removing = true"
        >
          Remove schedule
        </button>
        <div class="flex-1" />
        <BaseButton
          variant="secondary"
          size="sm"
          :disabled="busy"
          data-testid="agent-workspace-editor-schedule-cancel"
          @click="schedules.close()"
          >Cancel</BaseButton
        >
        <BaseButton
          size="sm"
          :disabled="busy"
          data-testid="agent-workspace-editor-schedule-keep"
          @click="schedules.keep"
        >
          {{ schedules.draft.key === null ? 'Add schedule' : 'Done' }}
        </BaseButton>
      </div>
      <InlineConfirm
        v-else-if="confirming"
        class="-mx-[18px] -my-[13px]"
        title="Delete this workspace?"
        :description="`Every live chat in ${workspace!.name || workspace!.dir} closes, its chat history is removed, and ${deletedPath} is deleted from disk with everything in it — AGENTS.md, docs, canvases, and anything else written there. This cannot be undone.`"
        confirm-label="Delete"
        :busy="busy"
        :error="error"
        testid="agent-workspace-editor-delete-confirm"
        @confirm="emit('delete', workspace!.dir)"
        @cancel="confirming = false"
      />
      <InlineConfirm
        v-else-if="discarding"
        class="-mx-[18px] -my-[13px]"
        title="Discard your changes?"
        description="Nothing here is written until you save. Closing now leaves agent-workspace.yaml exactly as it was."
        confirm-label="Discard"
        cancel-label="Keep editing"
        testid="agent-workspace-editor-discard-confirm"
        @confirm="emit('close')"
        @cancel="discarding = false"
      />
      <div v-else class="flex items-center gap-2.5">
        <button
          v-if="!creating"
          type="button"
          class="cursor-pointer text-small text-text-3 hover:text-severity-error disabled:opacity-50"
          :disabled="busy"
          data-testid="agent-workspace-editor-delete"
          @click="confirming = true"
        >
          Delete workspace
        </button>
        <div class="flex-1" />
        <BaseButton
          variant="secondary"
          size="sm"
          :disabled="busy"
          data-testid="agent-workspace-editor-cancel"
          @click="cancel"
          >Cancel</BaseButton
        >
        <BaseButton size="sm" :busy="busy" :disabled="!valid" data-testid="agent-workspace-editor-save" @click="submit">
          {{ creating ? 'Create workspace' : 'Save' }}
        </BaseButton>
      </div>
    </template>
  </DrawerSheet>
</template>
