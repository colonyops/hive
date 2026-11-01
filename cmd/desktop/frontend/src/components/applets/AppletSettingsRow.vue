<script setup lang="ts">
// One applet in Settings. The controls sit in fixed columns, so a long list reads down the
// page, and a status shows only when there is something to say.
import { computed, type DeepReadonly } from 'vue'
import IconPlay from '~icons/lucide/play'
import IconTrash from '~icons/lucide/trash-2'
import IconUpload from '~icons/lucide/upload'
import AppSwitch from '@hive/ui/components/ui/AppSwitch.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import type { Applet } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { launcherIconComponent } from '../../lib/launcherIcons'
import { PLACEABLE_SURFACES } from '../../stores/appletSurfaces'
import { useApplets } from '../../stores/useApplets'

const props = defineProps<{ applet: DeepReadonly<Applet>; running: number }>()
const emit = defineEmits<{ open: []; export: []; delete: []; enabled: [value: boolean] }>()

const applets = useApplets()
const icon = computed(() => launcherIconComponent(props.applet.icon))
const usable = computed(() => props.applet.runnable && props.applet.enabled)

const status = computed<{ tone: 'danger' | 'accent' | 'neutral'; label: string } | null>(() => {
  if (!props.applet.runnable) return { tone: 'danger', label: 'Invalid' }
  if (props.applet.error) return { tone: 'accent', label: 'Running last good file' }
  if (!props.applet.enabled) return { tone: 'neutral', label: 'Off' }
  return null
})

// The label names which applet, because the switch has no words of its own beside it.
function placementLabel(label: string): string {
  return `${label}: ${props.applet.title}`
}
</script>

<template>
  <div
    class="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-3 gap-y-2 px-4 py-2.5 @[600px]/pane:grid-cols-[auto_minmax(0,1fr)_auto]"
    :data-testid="`settings-applet-${applet.id}`"
  >
    <component :is="icon" class="size-4 shrink-0" :class="usable ? 'text-text-2' : 'text-text-4'" aria-hidden="true" />
    <div class="min-w-0">
      <div class="flex min-w-0 items-center gap-2">
        <span class="truncate text-body font-medium" :class="usable ? 'text-text' : 'text-text-3'">{{
          applet.title
        }}</span>
        <BaseBadge
          v-if="status"
          :tone="status.tone"
          variant="pill"
          class="shrink-0 px-2 py-px text-caption font-medium"
          :data-testid="`settings-applet-${applet.id}-state`"
          >{{ status.label }}</BaseBadge
        >
      </div>
      <div class="mt-0.5 flex min-w-0 items-center gap-2 text-small text-text-3">
        <span class="truncate font-mono">{{ applet.id }}</span>
        <span v-if="applet.builtin" class="shrink-0">Ships with Hive</span>
        <span v-if="running" class="shrink-0" :data-testid="`settings-applet-${applet.id}-running`"
          >{{ running }} running</span
        >
      </div>
      <p
        v-if="applet.error"
        class="mt-1 text-small leading-relaxed text-severity-error"
        :data-testid="`settings-applet-${applet.id}-error`"
      >
        {{ applet.error }}
      </p>
      <ul
        v-if="applet.warnings?.length"
        class="mt-1 text-small leading-relaxed text-severity-warning"
        :data-testid="`settings-applet-${applet.id}-warnings`"
      >
        <li v-for="warning in applet.warnings" :key="warning">{{ warning }}</li>
      </ul>
    </div>
    <div class="col-span-2 flex items-center justify-end gap-3 @[600px]/pane:col-span-1">
      <div
        v-for="{ surface, placement } in PLACEABLE_SURFACES"
        :key="surface"
        class="flex w-24 justify-end text-meta text-text-3"
      >
        <AppSwitch
          v-if="placement.offeredPage(applet)"
          size="sm"
          :label="placement.label"
          :aria-label="placementLabel(placement.label)"
          :model-value="applets.isPlaced(surface, applet.id)"
          :disabled="!usable"
          :testid="`settings-applet-${applet.id}-${surface}`"
          @update:model-value="applets.setPlaced(surface, applet.id, $event)"
        />
      </div>
      <div class="grid w-16 grid-cols-3 justify-items-end gap-0.5">
        <IconButton
          v-if="!applet.builtin"
          class="col-start-1"
          :icon="IconTrash"
          :label="`Delete ${applet.title}`"
          tooltip="Delete"
          tone="danger"
          size="sm"
          :data-testid="`settings-applet-${applet.id}-delete`"
          @click="emit('delete')"
        />
        <IconButton
          v-if="!applet.builtin && applet.runnable"
          class="col-start-2"
          :icon="IconUpload"
          :label="`Export ${applet.title}`"
          tooltip="Export"
          size="sm"
          :data-testid="`settings-applet-${applet.id}-export`"
          @click="emit('export')"
        />
        <IconButton
          class="col-start-3"
          :icon="IconPlay"
          :label="`Open ${applet.title}`"
          tooltip="Open"
          size="sm"
          :disabled="!applet.enabled"
          :data-testid="`settings-applet-${applet.id}-open`"
          @click="emit('open')"
        />
      </div>
      <AppSwitch
        :aria-label="`Turn ${applet.title} on`"
        :model-value="applet.enabled"
        :testid="`settings-applet-${applet.id}-enabled`"
        @update:model-value="emit('enabled', $event)"
      />
    </div>
  </div>
</template>
