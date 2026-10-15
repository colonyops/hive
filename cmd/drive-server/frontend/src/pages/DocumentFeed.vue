<script setup lang="ts">
// Recent and Shared links: documents gathered from every space.
import { computed, onMounted, ref } from 'vue'
import DriveListing from '../components/DriveListing.vue'
import { useDriveData } from '../lib/driveData'
import { documentEntries, recentDocuments } from '../lib/entries'
import type { DriveDocument } from '../types'

const props = defineProps<{ feed: 'recent' | 'links' }>()

const data = useDriveData()
const loaded = ref(false)

async function load(): Promise<void> {
  await data.loadAll()
  loaded.value = true
}
onMounted(load)

async function reload(): Promise<void> {
  await data.loadSpaces()
  await Promise.all(Object.keys(data.documents).map((key) => data.load(key, true)))
}

const all = computed<DriveDocument[]>(() => Object.values(data.documents).flatMap((docs) => docs ?? []))
const entries = computed(() => {
  if (!loaded.value) return null
  const docs = props.feed === 'recent' ? recentDocuments(all.value) : recentDocuments(all.value.filter((d) => d.shared))
  return documentEntries(docs)
})
const title = computed(() => (props.feed === 'recent' ? 'Recent' : 'Shared links'))
const emptyMessage = computed(() =>
  props.feed === 'recent'
    ? 'No documents yet. Push one with hive drive push.'
    : 'No shared links. Share a document to see it here.',
)
</script>

<template>
  <DriveListing
    :crumbs="[{ label: title, to: feed === 'recent' ? '/recent' : '/links' }]"
    :entries="entries"
    :error="data.spacesError.value"
    :empty-message="emptyMessage"
    @changed="reload"
  />
</template>
