<script setup lang="ts">
// Recent and Shared links: documents gathered from every space by one request each.
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import DriveListing from '../components/DriveListing.vue'
import { useDriveData } from '../lib/driveData'
import { documentEntries } from '../lib/entries'
import type { DriveDocument } from '../types'

const props = defineProps<{ feed: 'recent' | 'links' }>()

const data = useDriveData()
const documents = ref<DriveDocument[] | null>(null)
const error = ref<string | null>(null)

async function load(): Promise<void> {
  try {
    documents.value = await (props.feed === 'recent' ? api.recent() : api.sharedLinks())
    error.value = null
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Could not load documents.'
  }
}
onMounted(load)

// A share or delete here also changes the space lists the nav and folders cache.
async function reload(): Promise<void> {
  await Promise.all([load(), data.loadSpaces(), ...Object.keys(data.documents).map((key) => data.load(key, true))])
}

const entries = computed(() => (documents.value ? documentEntries(documents.value) : null))
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
    :error="error ?? data.spacesError.value"
    :empty-message="emptyMessage"
    @changed="reload"
  />
</template>
