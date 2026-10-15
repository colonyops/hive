<script setup lang="ts">
import { computed, onMounted } from 'vue'
import DriveListing from '../components/DriveListing.vue'
import { useDriveData } from '../lib/driveData'
import { spaceEntries } from '../lib/entries'

const data = useDriveData()
onMounted(() => {
  if (!data.spaces.value) void data.loadSpaces()
})

const entries = computed(() => (data.spaces.value ? spaceEntries(data.spaces.value) : null))
</script>

<template>
  <DriveListing
    :crumbs="[{ label: 'All spaces', to: '/' }]"
    :entries="entries"
    :error="data.spacesError.value"
    empty-message="No documents yet. Push one with hive drive push."
    data-testid="spaces-list"
  />
</template>
