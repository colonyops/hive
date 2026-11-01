<script setup lang="ts">
// The last stop before a stranger's code lands on this machine, so the warning leads, the
// default is to cancel, and importing takes a deliberate tick and a click. Nothing is on
// disk until then (ADR applets-are-web-pages-with-full-access-to-the-machine).
import { nextTick, onBeforeUnmount, onMounted, ref, watch, type ComponentPublicInstance } from 'vue'
import IconTriangleAlert from '~icons/lucide/triangle-alert'
import AppCheckbox from '@hive/ui/components/ui/AppCheckbox.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import BaseModal from '@hive/ui/components/ui/BaseModal.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import {
  CancelImport,
  ConfirmImport,
} from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/appletsservice'
import type { AppletImportPreview } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { useKeybindings } from '../../composables/useKeybindings'
import { useToasts } from '../../composables/useToasts'
import { formatBytes } from '../../lib/bytes'
import { PLACEABLE_SURFACES } from '../../stores/appletSurfaces'
import { useApplets } from '../../stores/useApplets'

const props = defineProps<{ preview: AppletImportPreview }>()
const emit = defineEmits<{ close: [] }>()

const WIDTH = 560

const applets = useApplets()
const keybindings = useKeybindings()
const { showToast } = useToasts()
const trusted = ref(false)
const id = ref('')
const busy = ref(false)
const error = ref('')
const cancelButton = ref<ComponentPublicInstance | null>(null)
// The id can be taken while this dialog is open, so a refused import also shows the field.
const conflict = ref(false)
let settled = false

watch(
  () => props.preview.suggestedId,
  (suggested) => {
    id.value = suggested
    conflict.value = suggested !== props.preview.id
  },
  { immediate: true },
)

onMounted(() => {
  void nextTick(() => (cancelButton.value?.$el as HTMLElement | undefined)?.focus())
})

function dropWaitingFile(): void {
  if (settled) return
  settled = true
  void CancelImport(props.preview.token)
}

function cancel(): void {
  dropWaitingFile()
  emit('close')
}

// A page can navigate away from Settings with this open, and the file must not stay in memory.
onBeforeUnmount(dropWaitingFile)

async function confirm(): Promise<void> {
  const target = id.value.trim()
  if (applets.applets.value.some((applet) => applet.id === target)) {
    conflict.value = true
    error.value = `An applet called "${target}" already exists. Choose another id.`
    return
  }
  busy.value = true
  error.value = ''
  try {
    await ConfirmImport(props.preview.token, target)
    settled = true
    await Promise.all([applets.reload(), applets.reloadPlacements(), keybindings.reload()])
    showToast(`Imported ${props.preview.title}`, { severity: 'success', body: 'Nothing is running until you open it.' })
    emit('close')
  } catch (e) {
    conflict.value = true
    error.value = e instanceof Error ? e.message : 'Could not import the applet.'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <BaseModal
    title="Only import applets from people you trust"
    :icon="IconTriangleAlert"
    tone="danger"
    aria-role="alertdialog"
    :width="WIDTH"
    :busy="busy"
    testid="applet-import"
    @close="cancel"
  >
    <div class="space-y-4 p-5 text-body">
      <div class="rounded-lg bg-severity-error-tint p-3.5" data-testid="applet-import-warning">
        <p class="font-semibold text-severity-error">An applet has full access to this machine, as you.</p>
        <p class="mt-1.5 text-text-2">Hive does not review or limit it. Once you open it, it can:</p>
        <ul class="mt-1.5 list-disc space-y-0.5 pl-5 text-text-2">
          <li>run any command or program, and keep processes running</li>
          <li>read, change and delete any of your files, including keys and passwords stored in them</li>
          <li>reach the network, and send what it reads anywhere</li>
          <li>use your clipboard, and ask for your camera and microphone</li>
          <li>run Hive's commands and move around the app</li>
        </ul>
        <p class="mt-1.5 text-text-2">Importing is the same as running a program someone sent you.</p>
      </div>

      <dl class="space-y-2.5">
        <div>
          <dt class="text-meta text-text-3">Name</dt>
          <dd class="font-medium" data-testid="applet-import-title">{{ preview.title }}</dd>
        </div>
        <div v-if="conflict" data-testid="applet-import-conflict">
          <dt class="text-meta text-text-3">Id</dt>
          <dd class="text-text-2">
            An applet called <code class="font-mono">{{ preview.id }}</code> may already exist, and will not be
            replaced. This one imports as:
          </dd>
          <input
            v-model="id"
            class="mt-1.5 h-7 w-full rounded-md border border-border bg-transparent px-2 font-mono text-meta"
            spellcheck="false"
            aria-label="Applet id"
            data-testid="applet-import-id"
          />
        </div>
        <div>
          <dt class="text-meta text-text-3">What runs</dt>
          <dd>
            <ul class="space-y-0.5 text-text-2" data-testid="applet-import-runs">
              <li>Nothing runs on import, and nothing starts by itself.</li>
              <li>
                <code class="font-mono">{{ preview.entry }}</code> runs when you open the applet.
              </li>
              <template v-for="{ surface, placement } in PLACEABLE_SURFACES" :key="surface">
                <li v-if="placement.offeredPage(preview)">
                  It offers <code class="font-mono">{{ placement.offeredPage(preview) }}</code> for
                  {{ placement.where }}. That stays off until you turn it on, and then runs whenever Hive is open.
                </li>
              </template>
              <li v-if="preview.commands?.length">
                It adds to the command palette, and running one opens its page:
                {{ preview.commands.map((command) => command.title).join(', ') }}.
              </li>
              <li v-if="preview.cwd">
                It runs commands in <code class="font-mono">{{ preview.cwd }}</code
                >.
              </li>
            </ul>
          </dd>
        </div>
        <div v-if="preview.warnings?.length">
          <dt class="text-meta text-text-3">What this Hive leaves out</dt>
          <dd class="text-text-2" data-testid="applet-import-ignored">
            <p>
              This file asks for things this version of Hive does not know. If any were meant to limit the applet, they
              will not.
            </p>
            <ul class="mt-1 list-disc pl-5">
              <li v-for="warning in preview.warnings" :key="warning">{{ warning }}</li>
            </ul>
          </dd>
        </div>
        <div>
          <dt class="text-meta text-text-3">
            {{ preview.files?.length }} files, {{ formatBytes(preview.totalBytes) }}
          </dt>
          <dd>
            <ul
              class="mt-1 max-h-36 overflow-y-auto rounded-md border border-border px-2 py-1 font-mono text-meta"
              data-testid="applet-import-files"
            >
              <li v-for="file in preview.files" :key="file.path" class="flex justify-between gap-3">
                <span class="min-w-0 truncate">{{ file.path }}</span>
                <span class="shrink-0 text-text-3">{{ formatBytes(file.bytes) }}</span>
              </li>
            </ul>
          </dd>
        </div>
      </dl>

      <AppCheckbox v-model="trusted" label="I trust who this came from" testid="applet-import-trust" />
      <InlineError v-if="error" :message="error" testid="applet-import-error" />
    </div>
    <template #footer>
      <div class="flex-1" />
      <BaseButton ref="cancelButton" variant="secondary" data-testid="applet-import-cancel" @click="cancel"
        >Cancel</BaseButton
      >
      <BaseButton
        variant="danger"
        :disabled="!trusted || !id.trim()"
        :busy="busy"
        data-testid="applet-import-confirm"
        @click="confirm"
        >Import applet</BaseButton
      >
    </template>
  </BaseModal>
</template>
