<script setup lang="ts">
// One applet in Settings. A row keeps one menu for its places and one on/off switch however
// many surfaces there are; everything else is in AppletDetailsDrawer.
import { computed, ref, type DeepReadonly } from 'vue'
import IconChevronDown from '~icons/lucide/chevron-down'
import IconChevronRight from '~icons/lucide/chevron-right'
import IconPlus from '~icons/lucide/plus'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import AppSwitch from '@hive/ui/components/ui/AppSwitch.vue'
import BaseBadge from '@hive/ui/components/ui/BaseBadge.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import type { MenuEntry } from '@hive/ui/types/menu'
import type { Applet } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { launcherIconComponent } from '../../lib/launcherIcons'
import { PLACEABLE_SURFACES, type AppletSurface } from '../../stores/appletSurfaces'
import { useApplets } from '../../stores/useApplets'

const props = defineProps<{ applet: DeepReadonly<Applet>; running: number }>()
const emit = defineEmits<{ details: []; enabled: [value: boolean] }>()

const applets = useApplets()
const icon = computed(() => launcherIconComponent(props.applet.icon))
const usable = computed(() => props.applet.runnable && props.applet.enabled)

const status = computed<{ tone: 'danger' | 'accent' | 'neutral'; label: string } | null>(() => {
  if (!props.applet.runnable) return { tone: 'danger', label: 'Invalid' }
  if (props.applet.error) return { tone: 'accent', label: 'Running last good file' }
  if (!props.applet.enabled) return { tone: 'neutral', label: 'Off' }
  return null
})

const warningCount = computed(() => props.applet.warnings?.length ?? 0)

const offersAPlace = computed(() => PLACEABLE_SURFACES.some(({ placement }) => placement.offeredPage(props.applet)))
const placed = computed(() =>
  PLACEABLE_SURFACES.filter(({ surface }) => applets.isPlaced(surface, props.applet.id)).map(
    ({ placement }) => placement,
  ),
)
const placedLabel = computed(() => placed.value.map((placement) => placement.label).join(', '))

const row = ref<HTMLElement | null>(null)
const menuToggle = ref<HTMLElement | null>(null)
const menuOpen = ref(false)

const menuEntries = computed<MenuEntry[]>(() =>
  PLACEABLE_SURFACES.map(({ surface, placement }) => {
    const offered = Boolean(placement.offeredPage(props.applet))
    return {
      kind: 'action',
      id: surface,
      label: placement.label,
      checked: applets.isPlaced(surface, props.applet.id),
      disabled: !offered,
      hint: offered ? undefined : 'No page for this place',
      testid: `settings-applet-${props.applet.id}-places-${surface}`,
    }
  }),
)

// The menu stays open, so someone can change several places in one go.
function togglePlace(surface: string): void {
  const id = surface as AppletSurface
  applets.setPlaced(id, props.applet.id, !applets.isPlaced(id, props.applet.id))
}
</script>

<template>
  <div
    ref="row"
    class="flex min-h-13 items-center gap-1 pr-3 hover:bg-row-hover"
    :data-testid="`settings-applet-${applet.id}`"
  >
    <button
      type="button"
      class="flex min-w-0 flex-1 cursor-pointer items-center gap-3 py-2.5 pl-4 pr-2 text-left outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-accent"
      :data-testid="`settings-applet-${applet.id}-details`"
      @click="emit('details')"
    >
      <component
        :is="icon"
        class="size-4 shrink-0"
        :class="usable ? 'text-text-2' : 'text-text-4'"
        aria-hidden="true"
      />
      <span class="min-w-0 flex-1">
        <span class="flex min-w-0 items-center gap-2">
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
        </span>
        <span class="mt-0.5 flex min-w-0 items-center gap-2 text-small text-text-3">
          <span
            v-if="applet.error"
            class="truncate"
            :class="applet.runnable ? 'text-severity-warning' : 'text-severity-error'"
            :title="applet.error"
            :data-testid="`settings-applet-${applet.id}-error`"
            >{{ applet.error }}</span
          >
          <span
            v-else-if="warningCount"
            class="truncate text-severity-warning"
            :data-testid="`settings-applet-${applet.id}-warnings`"
            >{{ warningCount === 1 ? '1 warning' : `${warningCount} warnings` }}</span
          >
          <span v-else class="truncate font-mono">{{ applet.id }}</span>
          <span v-if="applet.builtin" class="shrink-0">Ships with Hive</span>
          <span v-if="running" class="shrink-0" :data-testid="`settings-applet-${applet.id}-running`"
            >{{ running }} running</span
          >
        </span>
      </span>
    </button>
    <div class="flex shrink-0 items-center gap-2">
      <div class="flex justify-end @[600px]/pane:w-40">
        <span v-if="!offersAPlace" class="whitespace-nowrap text-small text-text-3">Panel only</span>
        <div v-else ref="menuToggle">
          <button
            type="button"
            class="flex h-6 cursor-pointer items-center gap-1 rounded-md border px-1.5 text-small whitespace-nowrap text-text-2 outline-none hover:border-strong hover:bg-chip focus-visible:ring-2 focus-visible:ring-accent disabled:cursor-default disabled:opacity-50 disabled:hover:border-border disabled:hover:bg-transparent"
            :class="placed.length ? 'border-border' : 'border-dashed border-strong'"
            aria-haspopup="menu"
            :aria-expanded="menuOpen"
            :aria-label="`Places for ${applet.title}: ${placedLabel || 'none'}`"
            :disabled="!usable"
            :data-testid="`settings-applet-${applet.id}-places`"
            @click="menuOpen = !menuOpen"
          >
            <component :is="placed[0]?.icon ?? IconPlus" class="size-3 shrink-0" aria-hidden="true" />
            <span class="hidden max-w-28 truncate @[600px]/pane:inline">{{ placedLabel || 'Add to a place' }}</span>
            <IconChevronDown class="size-3 shrink-0" aria-hidden="true" />
          </button>
        </div>
        <!-- Anchored to the row: the menu takes its width from the anchor, and this button is
             narrower than the menu's entries. -->
        <AppMenu
          v-if="menuOpen"
          :anchor="row"
          :ignore="[menuToggle]"
          :entries="menuEntries"
          :testid="`settings-applet-${applet.id}-places-menu`"
          @select="togglePlace"
          @close="menuOpen = false"
        />
      </div>
      <AppSwitch
        :aria-label="`Turn ${applet.title} on`"
        :model-value="applet.enabled"
        :testid="`settings-applet-${applet.id}-enabled`"
        @update:model-value="emit('enabled', $event)"
      />
      <IconButton
        :icon="IconChevronRight"
        :label="`Details for ${applet.title}`"
        size="md"
        tabindex="-1"
        @click="emit('details')"
      />
    </div>
  </div>
</template>
