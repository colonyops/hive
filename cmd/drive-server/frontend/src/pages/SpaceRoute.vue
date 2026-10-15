<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import type { Crumb } from '../components/Breadcrumb.vue'
import DriveListing from '../components/DriveListing.vue'
import { useDriveData } from '../lib/driveData'
import { documentEntries, folderContents } from '../lib/entries'
import { useSearchQuery } from '../lib/search'
import { documentRoute, fromRouteParams } from '../lib/space'
import { buildTree, matchingDocuments } from '../lib/tree'
import DocumentView from './DocumentView.vue'

const route = useRoute()
const data = useDriveData()
const query = useSearchQuery()

const target = computed(() => fromRouteParams(route.params))
const space = computed(() => target.value.space)
// A path that names a document opens it; any other path is a folder.
const path = computed(() => target.value.path.replace(/\/+$/, ''))
const documents = computed(() => data.documents[space.value.key] ?? null)
const isDocument = computed(() => !!path.value && !!documents.value?.some((d) => d.path === path.value))

watch(
  () => space.value.key,
  (key) => void data.load(key),
  { immediate: true },
)

const filtering = computed(() => query.value.trim() !== '')
const contents = computed(() => {
  const docs = documents.value
  if (!docs) return null
  if (filtering.value) return documentEntries(matchingDocuments(buildTree(docs), query.value), false)
  return folderContents(docs, space.value, path.value)
})

const error = computed(() => {
  const failed = data.errors[space.value.key]
  if (failed) return failed
  return documents.value && !contents.value ? 'This folder does not exist.' : null
})

const crumbs = computed<Crumb[]>(() => {
  const segments = path.value.split('/').filter(Boolean)
  return [
    { label: space.value.title, to: space.value.routePrefix },
    ...segments.map((name, i) => ({
      label: name,
      to: documentRoute(space.value, segments.slice(0, i + 1).join('/')),
    })),
  ]
})

const emptyMessage = computed(() => {
  if (filtering.value) return `No documents match "${query.value.trim()}".`
  return path.value ? 'This folder is empty.' : 'This space has no documents.'
})

function reload(): void {
  void data.load(space.value.key, true)
  void data.loadSpaces()
}
</script>

<template>
  <DocumentView
    v-if="isDocument"
    :key="route.fullPath"
    :space="space"
    :path="path"
    :documents="documents"
    :load-error="null"
    @changed="reload"
  />
  <DriveListing
    v-else
    :crumbs="crumbs"
    :entries="contents"
    :error="error"
    :empty-message="emptyMessage"
    @changed="reload"
  />
</template>
