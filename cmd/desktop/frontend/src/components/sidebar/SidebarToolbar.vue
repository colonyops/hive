<script setup lang="ts">
// The bar over a tree sidebar (Code and Chats): filter, reload, new, and the
// list menu. A host can leave reload out (Code keeps it in the list menu) and
// put controls of its own before new through the default slot. The filter sits flush rather than boxed: the sidebar resizes down
// narrow, and a bordered field beside three controls leaves the bar looking
// like nothing but chrome. Attributes and listeners go to the filter's input.
import { ref, shallowRef } from 'vue'
import IconEllipsisVertical from '~icons/lucide/ellipsis-vertical'
import IconPlus from '~icons/lucide/plus'
import IconRotateCw from '~icons/lucide/rotate-cw'
import AppMenu from '@hive/ui/components/ui/AppMenu.vue'
import IconButton from '@hive/ui/components/ui/IconButton.vue'
import SearchField from '@hive/ui/components/ui/SearchField.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import type { MenuEntry } from '@hive/ui/types/menu'

defineOptions({ inheritAttrs: false })

defineProps<{
  filterLabel: string
  /** Leave out to drop the reload button; a spinner then stands in for it while loading. */
  reloadLabel?: string
  newLabel: string
  menuLabel: string
  menuEntries: MenuEntry[]
  /** The tree renders from its last-good rows, so the spin is what says a re-read is in flight. */
  loading?: boolean
  creating?: boolean
  /** Names the filter (`${testid}-filter`) and the menu (`${testid}-menu`, `${testid}-menu-toggle`). */
  testid: string
  reloadTestid?: string
  newTestid: string
}>()

const filter = defineModel<string>({ required: true })
const emit = defineEmits<{ reload: []; new: []; select: [id: string] }>()

const field = ref<{ focus: () => void; select: () => void } | null>(null)
const menuOpen = ref(false)
const menuWrap = shallowRef<HTMLElement | null>(null)

function onSelect(id: string): void {
  menuOpen.value = false
  emit('select', id)
}

defineExpose({
  focus: () => field.value?.focus(),
  select: () => field.value?.select(),
})
</script>

<template>
  <div class="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3">
    <SearchField
      ref="field"
      v-bind="$attrs"
      v-model="filter"
      variant="bar"
      :aria-label="filterLabel"
      :testid="`${testid}-filter`"
    />
    <IconButton
      v-if="reloadLabel"
      :label="reloadLabel"
      :icon="IconRotateCw"
      :busy="loading"
      :data-testid="reloadTestid"
      @click="emit('reload')"
    />
    <!-- The tree renders from its last-good rows, so without the reload button's spin something still has to say a re-read is in flight. -->
    <span
      v-else-if="loading"
      class="flex text-text-4"
      role="status"
      aria-label="Reloading"
      :data-testid="`${testid}-reloading`"
    >
      <Spinner />
    </span>
    <slot />
    <IconButton :label="newLabel" :icon="IconPlus" :busy="creating" :data-testid="newTestid" @click="emit('new')" />
    <div ref="menuWrap" class="relative flex">
      <IconButton
        :label="menuLabel"
        :icon="IconEllipsisVertical"
        :data-testid="`${testid}-menu-toggle`"
        aria-haspopup="menu"
        :aria-expanded="menuOpen"
        @click="menuOpen = !menuOpen"
      />
      <AppMenu
        v-if="menuOpen"
        :entries="menuEntries"
        :ignore="[menuWrap]"
        :testid="`${testid}-menu`"
        @close="menuOpen = false"
        @select="onSelect"
      />
    </div>
  </div>
</template>
