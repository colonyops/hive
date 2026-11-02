<script setup lang="ts">
import { computed, type DeepReadonly } from 'vue'
import IconFolder from '~icons/lucide/folder'
import IconPlay from '~icons/lucide/play'
import AppSwitch from '@hive/ui/components/ui/AppSwitch.vue'
import BaseButton from '@hive/ui/components/ui/BaseButton.vue'
import BaseIconBadge from '@hive/ui/components/ui/BaseIconBadge.vue'
import DrawerSheet from '@hive/ui/components/ui/DrawerSheet.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Kbd from '@hive/ui/components/ui/Kbd.vue'
import SettingsHeading from '../settings/SettingsHeading.vue'
import type { Applet, AppletProcess } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { formatCombo, useKeybindings } from '../../composables/useKeybindings'
import { useSystemSettings } from '../../composables/useSystemSettings'
import { appletCommandID } from '../../keybindings/catalog'
import { requestedEditorFilter } from '../../keybindings/keymapRows'
import { launcherIconComponent } from '../../lib/launcherIcons'
import { router } from '../../router'
import { DECLARED_SURFACES, type AppletSurface } from '../../stores/appletSurfaces'

const props = defineProps<{
  applet: DeepReadonly<Applet>
  processes: readonly AppletProcess[]
  dir: string
  error: string | null
}>()
const emit = defineEmits<{
  close: []
  open: []
  export: []
  delete: []
  enabled: [value: boolean]
  surface: [surface: AppletSurface, on: boolean]
  stop: [process: AppletProcess]
}>()

const keybindings = useKeybindings()
const { revealPath } = useSystemSettings()

const icon = computed(() => launcherIconComponent(props.applet.icon))
const opens = computed(() => !props.applet.runnable || props.applet.surfaces.tab || !!props.applet.surfaces.popup)
const folder = computed(() => `${props.dir}/${props.applet.id}`)

const commands = computed(() =>
  [
    ...(opens.value ? [{ id: appletCommandID(props.applet.id), title: `Open ${props.applet.title}` }] : []),
    ...(props.applet.runnable ? (props.applet.commands ?? []) : []).map((command) => ({
      id: appletCommandID(props.applet.id, command.id),
      title: command.title,
    })),
  ].map((command) => ({ ...command, keys: keybindings.combosFor(command.id).map((combo) => formatCombo(combo)) })),
)

const canExport = computed(() => !props.applet.builtin && props.applet.runnable)
const hasActions = computed(() => !props.applet.builtin || opens.value)

function openShortcuts(): void {
  requestedEditorFilter.value = props.applet.title
  void router.push({ name: 'application-settings', params: { section: 'keybindings' } })
}
</script>

<template>
  <DrawerSheet
    :title="applet.title"
    :subtitle="applet.id"
    header-size="lg"
    closable
    testid="settings-applet-details"
    @close="emit('close')"
  >
    <template #icon>
      <BaseIconBadge :size="38" rounded="rounded-xl" class="bg-chip text-text-2">
        <component :is="icon" class="size-[18px]" />
      </BaseIconBadge>
    </template>
    <template #header-actions>
      <AppSwitch
        :aria-label="`Turn ${applet.title} on`"
        :model-value="applet.enabled"
        testid="settings-applet-details-enabled"
        @update:model-value="emit('enabled', $event)"
      />
    </template>

    <div class="flex flex-col gap-5">
      <InlineError v-if="error" :message="error" testid="settings-applet-details-error" />
      <InlineError v-if="!applet.runnable" :message="applet.error" testid="settings-applet-details-issue" />
      <div
        v-else-if="applet.error"
        class="rounded-md border border-severity-warning-border bg-severity-warning-tint px-3 py-2 text-xs leading-relaxed text-severity-warning"
        data-testid="settings-applet-details-issue"
      >
        Its file has an error, so Hive keeps running the last good version:
        <span class="font-mono">{{ applet.error }}</span>
      </div>
      <ul
        v-if="applet.warnings?.length"
        class="rounded-md border border-severity-warning-border bg-severity-warning-tint px-3 py-2 text-xs leading-relaxed text-severity-warning"
        data-testid="settings-applet-details-warnings"
      >
        <li v-for="warning in applet.warnings" :key="warning">{{ warning }}</li>
      </ul>

      <section v-if="applet.runnable">
        <SettingsHeading level="group" title="Where it shows" />
        <div class="mt-1 divide-y divide-border">
          <div
            v-for="{ surface, declared } in DECLARED_SURFACES"
            :key="surface"
            class="flex items-center gap-2.5 py-2"
            :data-testid="`settings-applet-${applet.id}-surface-${surface}`"
          >
            <component
              :is="declared.icon"
              class="size-3.5 shrink-0"
              :class="declared.available(applet) ? 'text-text-3' : 'text-text-4'"
              aria-hidden="true"
            />
            <div class="min-w-0 flex-1">
              <div class="text-small font-medium" :class="declared.available(applet) ? 'text-text' : 'text-text-4'">
                {{ declared.label }}
              </div>
              <p v-if="declared.available(applet)" class="mt-0.5 text-caption leading-relaxed text-text-3">
                {{ declared.description }}
              </p>
              <p v-else class="mt-0.5 text-caption leading-relaxed text-text-4">
                This applet has no page for here. Add <span class="font-mono">surfaces.{{ surface }}</span> to
                applet.yaml.
              </p>
            </div>
            <AppSwitch
              :aria-label="`${declared.label}: ${applet.title}`"
              :model-value="!!declared.page(applet)"
              :disabled="applet.builtin || !declared.available(applet)"
              :testid="`settings-applet-${applet.id}-${surface}`"
              @update:model-value="emit('surface', surface, $event)"
            />
          </div>
        </div>
        <p class="mt-1 text-caption text-text-3">
          <template v-if="applet.builtin">This applet ships with Hive, so where it shows is fixed.</template>
          <template v-else>Saved in its <span class="font-mono">applet.yaml</span>.</template>
        </p>
      </section>

      <section>
        <SettingsHeading level="group" title="Running" />
        <p v-if="!processes.length" class="py-2 text-caption text-text-3">Nothing running.</p>
        <div v-else class="mt-1 divide-y divide-border">
          <div v-for="process in processes" :key="process.handle" class="flex items-center gap-2.5 py-2">
            <div class="min-w-0 flex-1">
              <div class="font-mono text-caption break-all text-text">{{ process.command }}</div>
              <p class="mt-0.5 text-caption text-text-3">
                Since {{ new Date(process.startedMillis).toLocaleTimeString() }}
              </p>
            </div>
            <BaseButton
              variant="secondary"
              size="xs"
              :data-testid="`settings-applet-details-process-${process.handle}-stop`"
              @click="emit('stop', process)"
              >Stop</BaseButton
            >
          </div>
        </div>
      </section>

      <section>
        <div class="flex items-center gap-2.5">
          <SettingsHeading level="group" title="Commands" class="flex-1" />
          <button
            type="button"
            class="cursor-pointer text-caption text-text-3 hover:text-text hover:underline"
            data-testid="settings-applet-details-shortcuts"
            @click="openShortcuts"
          >
            Keyboard shortcuts
          </button>
        </div>
        <div class="mt-1 divide-y divide-border">
          <div v-for="command in commands" :key="command.id" class="flex min-h-9 items-center gap-2.5 py-1.5">
            <div class="min-w-0 flex-1 truncate text-small text-text">{{ command.title }}</div>
            <Kbd v-for="key in command.keys" :key="key" variant="boxed">{{ key }}</Kbd>
            <span v-if="!command.keys.length" class="shrink-0 text-caption text-text-3">No shortcut</span>
          </div>
        </div>
      </section>

      <section>
        <SettingsHeading level="group" title="Files" />
        <div class="mt-1 flex items-center gap-2.5 py-2">
          <IconFolder class="size-3.5 shrink-0 text-text-3" aria-hidden="true" />
          <div class="min-w-0 flex-1 font-mono text-caption break-all text-text-3">
            {{ applet.builtin ? 'Ships with Hive' : folder }}
          </div>
          <BaseButton
            v-if="!applet.builtin"
            variant="secondary"
            size="xs"
            data-testid="settings-applet-details-reveal"
            @click="revealPath(folder)"
            >Reveal</BaseButton
          >
        </div>
      </section>
    </div>

    <template v-if="hasActions" #footer>
      <div class="flex items-center gap-2.5">
        <button
          v-if="!applet.builtin"
          type="button"
          class="cursor-pointer text-small text-text-3 hover:text-severity-error"
          :data-testid="`settings-applet-${applet.id}-delete`"
          @click="emit('delete')"
        >
          Delete applet
        </button>
        <div class="flex-1" />
        <BaseButton
          v-if="canExport"
          variant="secondary"
          size="sm"
          :data-testid="`settings-applet-${applet.id}-export`"
          @click="emit('export')"
          >Export</BaseButton
        >
        <BaseButton
          v-if="opens"
          size="sm"
          :disabled="!applet.enabled"
          :data-testid="`settings-applet-${applet.id}-open`"
          @click="emit('open')"
        >
          <IconPlay class="size-3" aria-hidden="true" />Open
        </BaseButton>
      </div>
    </template>
  </DrawerSheet>
</template>
