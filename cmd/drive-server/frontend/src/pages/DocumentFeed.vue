<script setup lang="ts">
import { computed } from 'vue'
import { api } from '../api'
import DriveListing from '../components/DriveListing.vue'
import { useDriveData } from '../lib/driveData'
import { documentEntries } from '../lib/entries'
import { useLoad } from '../lib/load'

const props = defineProps<{ feed: 'recent' | 'links' }>()

const data = useDriveData()
const {
  data: documents,
  error,
  reload: load,
} = useLoad(() => (props.feed === 'recent' ? api.recent() : api.sharedLinks()), 'Could not load documents.')

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
