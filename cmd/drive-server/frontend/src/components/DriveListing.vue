<script setup lang="ts">
import ConfirmationHost from '@hive/ui/components/ui/ConfirmationHost.vue'
import EmptyState from '@hive/ui/components/ui/EmptyState.vue'
import InlineError from '@hive/ui/components/ui/InlineError.vue'
import Spinner from '@hive/ui/components/ui/Spinner.vue'
import { shallowRef } from 'vue'
import { useRouter } from 'vue-router'
import { entryRoute, type ListEntry } from '../lib/entries'
import { useEntryActions } from '../lib/entryActions'
import { useViewMode } from '../lib/viewMode'
import SharePopover from './SharePopover.vue'
import Breadcrumb, { type Crumb } from './Breadcrumb.vue'
import EntryList from './EntryList.vue'
import ViewToggle from './ViewToggle.vue'

defineProps<{
  crumbs: Crumb[]
  /** Null while loading. */
  entries: ListEntry[] | null
  error?: string | null
  emptyMessage: string
}>()
const emit = defineEmits<{ changed: [] }>()

const router = useRouter()
const { mode, set } = useViewMode()
const actions = useEntryActions(() => emit('changed'))
const sharing = shallowRef<{ entry: ListEntry; anchor: HTMLElement } | null>(null)

function onAction(id: string, entry: ListEntry, anchor: HTMLElement): void {
  if (id === 'share') sharing.value = { entry, anchor }
  else void actions.run(id, entry)
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col">
    <header class="flex min-h-16 shrink-0 flex-wrap items-center gap-x-3 gap-y-1 border-b border-row px-6 py-3">
      <div class="min-w-0 flex-1"><Breadcrumb :items="crumbs" /></div>
      <slot name="toolbar" />
      <ViewToggle :model-value="mode" @update:model-value="set" />
    </header>
    <div class="min-h-0 flex-1 overflow-y-auto">
      <InlineError v-if="error" class="m-4" :message="error" testid="listing-error" />
      <Spinner v-else-if="!entries" class="m-4" />
      <EmptyState v-else-if="!entries.length" :message="emptyMessage" data-testid="listing-empty" />
      <EntryList v-else :entries="entries" :mode="mode" @open="router.push(entryRoute($event))" @action="onAction" />
    </div>
    <SharePopover
      v-if="sharing"
      :key="sharing.entry.key"
      :anchor="sharing.anchor"
      :prefix="sharing.entry.space.apiPrefix"
      :path="sharing.entry.path"
      :share="sharing.entry.doc?.shared ?? null"
      @changed="emit('changed')"
      @close="sharing = null"
    />
    <ConfirmationHost :confirmation="actions.confirmation" />
  </div>
</template>
